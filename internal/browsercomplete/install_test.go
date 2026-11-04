package browsercomplete

import (
	browserassets "cliprag/integrations/browser"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeInstallationIsScopedIdempotentAndRejectsForeignFiles(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "chrome", "NativeMessagingHosts")
	binary := filepath.Join(root, "tools ' $(literal)", "cliprag-browser")
	for range 2 {
		if err := Install(directory, binary, filepath.Join(root, "data"), root, false); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(directory, browserassets.HostName+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest nativeManifest
	if err = json.Unmarshal(raw, &manifest); err != nil || manifest.Path != binary || len(manifest.AllowedOrigins) != 1 || manifest.AllowedOrigins[0] != Origin() {
		t.Fatal(manifest, err)
	}
	t.Setenv("XDG_CONFIG_HOME", root)
	database, err := DatabasePath()
	if err != nil || database != filepath.Join(root, "data", "history.db") {
		t.Fatal(database, err)
	}
	if err = Install(directory, binary, filepath.Join(root, "data"), root, true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("uninstall did not remove own manifest")
	}
	if err = os.WriteFile(path, []byte(`{"name":"another.application"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if Install(directory, binary, filepath.Join(root, "data"), root, false) == nil {
		t.Fatal("foreign manifest overwritten")
	}
	if Install(directory, binary, filepath.Join(root, "data"), root, true) == nil {
		t.Fatal("foreign manifest deleted")
	}
}
