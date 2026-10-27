//go:build linux

package linux

import (
	"errors"
	"os"
	"path/filepath"
)

func DataDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		base = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(base) {
		return "", errors.New("XDG_DATA_HOME must be absolute")
	}
	return filepath.Join(base, "cliprag"), nil
}
