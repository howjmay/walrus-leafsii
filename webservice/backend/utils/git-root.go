package utils

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func GitRoot(startDir string) (string, error) {
	// 1) normalize startDir
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	startDir = filepath.Clean(startDir)

	// 2) Try git CLI if available: git -C <startDir> rev-parse --show-toplevel
	if gitPath, err := exec.LookPath("git"); err == nil && gitPath != "" {
		cmd := exec.Command(gitPath, "-C", startDir, "rev-parse", "--show-toplevel")
		out, err := cmd.Output()
		if err == nil {
			root := strings.TrimSpace(string(bytes.TrimSpace(out)))
			if root != "" {
				return root, nil
			}
		}
		// if git failed (not a repo) fall through to fallback
	}

	// 3) Fallback: walk upward from startDir and look for .git
	for dir := startDir; ; dir = filepath.Dir(dir) {
		if dir == filepath.Dir(dir) { // reached filesystem root
			break
		}
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			// .git exists
			if info.IsDir() {
				// normal repo
				return dir, nil
			}
			// .git is a file (worktree / submodule) — treat containing directory as repo root
			// optionally we can parse the file to follow gitdir, but root is 'dir' anyway
			// read file if you want to inspect gitdir:
			// data, _ := ioutil.ReadFile(gitPath)
			return dir, nil
		}
		// if not found, keep walking up
	}

	return "", errors.New("not inside a git repository")
}
