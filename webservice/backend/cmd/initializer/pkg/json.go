package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pattonkan/sui-go/sui"
)

type InitConfig struct {
	ProtocolId        *sui.Address   `json:"protocol_id"`
	PoolId            *sui.Address   `json:"pool_id"`
	AdminCapId        *sui.ObjectId  `json:"admin_cap_id"`
	FtokenPackageId   *sui.Address   `json:"ftoken_package_id"`
	XtokenPackageId   *sui.Address   `json:"xtoken_package_id"`
	BrowserWalletAddr *sui.Address   `json:"browser_wallet_addr"`
	LeafsiiPackageId  *sui.PackageId `json:"leafsii_package_id"`
}

// ReadConfig reads JSON at path into Config.
// Returns os.ErrNotExist if the file doesn't exist.
// Returns nil with zero-value Config if the file is empty.
func ReadConfig(path string) (InitConfig, error) {
	var cfg InitConfig

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, err
		}
		return cfg, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return cfg, fmt.Errorf("stat: %w", err)
	}
	if st.Size() == 0 {
		// Empty file -> zero cfg, no error.
		return cfg, nil
	}

	dec := json.NewDecoder(f)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("decode: %w", err)
	}
	return cfg, nil
}

// WriteConfig marshals cfg as pretty JSON and writes it to path atomically,
// preserving existing file permissions (defaults to 0644 if file doesn't exist).
func WriteConfig(path string, cfg InitConfig) error {
	mode := fs.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode()
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat: %w", err)
	}

	// Marshal pretty with trailing newline.
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')

	if err := writeFileAtomic(path, data, mode); err != nil {
		return fmt.Errorf("atomic write: %w", err)
	}
	return nil
}

func writeFileAtomic(path string, content []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		_ = os.Remove(tmpName)
	}

	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("fsync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", err)
	}

	// Atomic replace (POSIX). Fallback remove+rename if needed.
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmpName, path); err2 != nil {
			_ = os.Remove(tmpName)
			return fmt.Errorf("rename: %w (after remove: %v)", err, err2)
		}
	}

	// Best-effort fsync the directory for durability on crashes.
	_ = fsyncDir(dir)
	return nil
}

func fsyncDir(dir string) error {
	df, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer df.Close()
	return df.Sync()
}
