package shellcomplete

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestBashRuntimeConcurrentInstallation(t *testing.T) {
	cache := t.TempDir()
	const terminals = 8
	paths := make([]string, terminals)
	errors := make([]error, terminals)
	var wg sync.WaitGroup
	for i := range paths {
		wg.Go(func() { paths[i], errors[i] = EnsureBashRuntime(cache) })
	}
	wg.Wait()
	for i, path := range paths {
		if errors[i] != nil || path != paths[0] {
			t.Fatalf("terminal %d: path=%q error=%v", i, path, errors[i])
		}
	}
	info, err := os.Stat(paths[0])
	if err != nil || info.Size() < 100000 || info.Mode().Perm() != 0600 {
		t.Fatalf("invalid staged runtime: %v, %v", info, err)
	}
	// Later terminal startup reuses the complete cache without rewriting it.
	path, err := EnsureBashRuntime(cache)
	after, _ := os.Stat(path)
	if err != nil || !after.ModTime().Equal(info.ModTime()) {
		t.Fatal("runtime was not reused", err)
	}
	stages, _ := filepath.Glob(filepath.Join(cache, "cliprag", "shell", ".ble-stage-*"))
	if len(stages) != 0 {
		t.Fatal("temporary staging directories remain", stages)
	}
}

func TestBashRuntimeRejectsIncompleteOrSymlinkedCache(t *testing.T) {
	if _, err := EnsureBashRuntime("relative"); err == nil {
		t.Fatal("relative cache accepted")
	}
	cache := t.TempDir()
	path, err := EnsureBashRuntime(cache)
	if err != nil {
		t.Fatal(err)
	}
	core := filepath.Join(filepath.Dir(path), "lib", "core-complete.sh")
	if err = os.Remove(core); err != nil {
		t.Fatal(err)
	}
	if _, err = EnsureBashRuntime(cache); err == nil {
		t.Fatal("incomplete runtime accepted")
	}
	if _, err = os.Stat(core); !os.IsNotExist(err) {
		t.Fatal("incomplete directory was overwritten")
	}
	destination := filepath.Dir(path)
	if err = os.Rename(destination, destination+"-saved"); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(destination+"-saved", destination); err != nil {
		t.Fatal(err)
	}
	if _, err = EnsureBashRuntime(cache); err == nil {
		t.Fatal("symlinked runtime accepted")
	}
}
