package shellcomplete

import (
	shellassets "cliprag/integrations/shell"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EnsureBashRuntime stages the embedded, pinned line editor once per user.
// Atomic publication allows several new terminals to start concurrently.
func EnsureBashRuntime(cache string) (string, error) {
	if !filepath.IsAbs(cache) {
		return "", errors.New("shell cache directory must be absolute")
	}
	parent := filepath.Join(cache, "cliprag", "shell")
	destination := filepath.Join(parent, "ble-"+shellassets.BashRuntimeVersion)
	ready := func() bool {
		info, err := os.Lstat(destination)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		marker, err := os.ReadFile(filepath.Join(destination, "VERSION"))
		if err != nil || string(marker) != shellassets.BashRuntimeVersion {
			return false
		}
		for _, required := range []string{"ble.sh", "lib/core-complete.sh", "contrib/scheme/default.bash"} {
			info, err = os.Lstat(filepath.Join(destination, required))
			if err != nil || !info.Mode().IsRegular() {
				return false
			}
		}
		return true
	}
	if ready() {
		return filepath.Join(destination, "ble.sh"), nil
	}
	if _, err := os.Lstat(destination); err == nil {
		return "", errors.New("shell runtime cache is incomplete; remove its version directory and retry")
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(parent, ".ble-stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	const prefix = "vendor/blesh"
	err = fs.WalkDir(shellassets.BashRuntime, prefix, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(path, prefix), "/")
		target := filepath.Join(stage, filepath.FromSlash(rel))
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		content, e := shellassets.BashRuntime.ReadFile(path)
		if e != nil {
			return e
		}
		return os.WriteFile(target, content, 0600)
	})
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(stage, "VERSION"), []byte(shellassets.BashRuntimeVersion), 0600); err != nil {
		return "", err
	}
	if err = os.Rename(stage, destination); err != nil && !ready() {
		return "", err
	}
	return filepath.Join(destination, "ble.sh"), nil
}
