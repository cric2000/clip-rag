package browsercomplete

import (
	browserassets "cliprag/integrations/browser"
	platform "cliprag/internal/platform/linux"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

var BrowserDirectories = map[string]string{
	"chrome": "google-chrome", "chromium": "chromium",
	"brave": "BraveSoftware/Brave-Browser", "edge": "microsoft-edge",
}

type nativeManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

func Origin() string { return "chrome-extension://" + browserassets.ExtensionID() + "/" }

func writePrivate(path string, value any) error {
	if existing, err := os.Lstat(path); err == nil && !existing.Mode().IsRegular() {
		return errors.New("configuration must be a regular file")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".cliprag-browser-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func Install(manifestDir, binary, dataDir, configRoot string, remove bool) error {
	if !filepath.IsAbs(manifestDir) || !filepath.IsAbs(binary) || !filepath.IsAbs(dataDir) || !filepath.IsAbs(configRoot) {
		return errors.New("browser setup paths must be absolute")
	}
	if info, err := os.Lstat(manifestDir); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("native messaging directory must be a regular directory")
	}
	path := filepath.Join(manifestDir, browserassets.HostName+".json")
	data, err := os.ReadFile(path)
	if err == nil {
		info, e := os.Lstat(path)
		if e != nil || !info.Mode().IsRegular() {
			return errors.New("native manifest must be a regular file")
		}
		var existing nativeManifest
		if json.Unmarshal(data, &existing) != nil || existing.Name != browserassets.HostName || existing.Type != "stdio" || len(existing.AllowedOrigins) != 1 || existing.AllowedOrigins[0] != Origin() {
			return errors.New("existing native manifest belongs to another integration")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if remove {
		if os.IsNotExist(err) {
			return nil
		}
		return os.Remove(path)
	}
	if err = writePrivate(filepath.Join(configRoot, "cliprag", "browser.json"), map[string]string{"dataDir": dataDir}); err != nil {
		return err
	}
	return writePrivate(path, nativeManifest{browserassets.HostName, "Local ClipRAG clipboard suggestions", binary, "stdio", []string{Origin()}})
}

func DatabasePath() (string, error) {
	dir, err := platform.DataDir()
	if err != nil {
		return "", err
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(configRoot, "cliprag", "browser.json")
	if info, e := os.Lstat(path); e == nil {
		if !info.Mode().IsRegular() || info.Size() > 4096 {
			return "", errors.New("invalid browser data configuration")
		}
		raw, e := os.ReadFile(path)
		if e != nil {
			return "", e
		}
		var cfg struct {
			DataDir string `json:"dataDir"`
		}
		if json.Unmarshal(raw, &cfg) != nil || !filepath.IsAbs(cfg.DataDir) {
			return "", errors.New("invalid browser data directory")
		}
		dir = cfg.DataDir
	} else if !os.IsNotExist(e) {
		return "", e
	}
	return filepath.Join(dir, "history.db"), nil
}
