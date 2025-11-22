package movebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pattonkan/sui-go/utils"
)

// Build compiles the Move package located at contractPath using the Sui CLI.
// It works from a temporary copy of the source so read-only checkouts don't
// break the build and uses offline-friendly flags to avoid network fetches.
func Build(ctx context.Context, contractPath string) (*utils.CompiledMoveModules, error) {
	if contractPath == "" {
		return nil, fmt.Errorf("contract path is empty")
	}

	abs, err := filepath.Abs(contractPath)
	if err != nil {
		return nil, fmt.Errorf("resolve contract path: %w", err)
	}

	tmpSrc, cleanup, err := mirrorToTemp(abs)
	if err != nil {
		return nil, fmt.Errorf("prepare temp copy of Move package: %w", err)
	}
	defer cleanup()

	modules, err := runSuiBuild(ctx, tmpSrc)
	if err != nil {
		return nil, err
	}
	return modules, nil
}

func runSuiBuild(ctx context.Context, dir string) (*utils.CompiledMoveModules, error) {
	installDir, err := os.MkdirTemp("", "walrus-move-install-*")
	if err != nil {
		return nil, fmt.Errorf("create temp install dir: %w", err)
	}
	defer os.RemoveAll(installDir)

	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(
		ctx,
		"sui",
		"move",
		"build",
		"--dump-bytecode-as-base64",
		"--skip-fetch-latest-git-deps",
		"--ignore-chain",
		"--install-dir", installDir,
	)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), fmt.Sprintf("RUST_BACKTRACE=%d", 1)) // keep failure output useful

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf(
			"sui move build failed: %w: %s",
			err,
			compactOutput(stdout.String(), stderr.String()),
		)
	}

	var modules utils.CompiledMoveModules
	if err := json.Unmarshal(stdout.Bytes(), &modules); err != nil {
		return nil, fmt.Errorf("parse move build output: %w", err)
	}
	return &modules, nil
}

func mirrorToTemp(src string) (string, func(), error) {
	info, err := os.Stat(src)
	if err != nil {
		return "", func() {}, fmt.Errorf("stat source: %w", err)
	}
	if !info.IsDir() {
		return "", func() {}, fmt.Errorf("contract path %s is not a directory", src)
	}

	tmpRoot, err := os.MkdirTemp("", "walrus-move-src-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp dir: %w", err)
	}

	// ensure clean-up survives being called multiple times
	cleanup := func() { _ = os.RemoveAll(tmpRoot) }

	dst := filepath.Join(tmpRoot, filepath.Base(src))
	if err := copyDir(src, dst); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return dst, cleanup, nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, info.Mode())
		}

		// Drop heavy or writable-only directories from the temp build.
		if info.IsDir() && (info.Name() == ".git" || info.Name() == "build") {
			return filepath.SkipDir
		}

		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()

		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()

		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return nil
	})
}

func compactOutput(stdout, stderr string) string {
	parts := []string{
		strings.TrimSpace(stdout),
		strings.TrimSpace(stderr),
	}

	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) == 0 {
		return "no output from sui"
	}

	// keep the first two chunks to avoid flooding logs with lengthy bytecode dumps
	if len(nonEmpty) > 2 {
		nonEmpty = nonEmpty[:2]
		nonEmpty = append(nonEmpty, fmt.Sprintf("...truncated at %s", time.Now().Format(time.RFC3339)))
	}
	return strings.Join(nonEmpty, "\n")
}
