package shellcomplete

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallIsReversibleAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rc")
	original := "# existing setup\nexport MY_PROMPT=kept\n"
	if err := os.WriteFile(path, []byte(original), 0640); err != nil {
		t.Fatal(err)
	}
	binary := "/tmp/Clip RAG's app/cliprag-shell"
	changed, err := UpdateRC(path, "bash", binary, true)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	installed, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(installed), original) || strings.Count(string(installed), blockStart) != 1 {
		t.Fatal("existing config replaced")
	}
	changed, err = UpdateRC(path, "bash", binary, true)
	if err != nil || changed {
		t.Fatal("install not idempotent", err)
	}
	backup, _ := os.ReadFile(path + ".cliprag-backup")
	if string(backup) != original {
		t.Fatal("original backup lost")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0640 {
		t.Fatal("permissions changed")
	}
	changed, err = UpdateRC(path, "bash", binary, false)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	restored, _ := os.ReadFile(path)
	if string(restored) != original {
		t.Fatal("uninstall changed unrelated setup")
	}
	changed, err = UpdateRC(path, "bash", binary, false)
	if err != nil || changed {
		t.Fatal("uninstall not idempotent")
	}
}
func TestInstallRefusesAmbiguousConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rc")
	for _, bad := range []string{blockStart + "\n", blockEnd + "\n", blockEnd + "\n" + blockStart, blockStart + "\n" + blockEnd + "\n" + blockStart + "\n" + blockEnd} {
		os.WriteFile(path, []byte(bad), 0600)
		if _, err := UpdateRC(path, "zsh", "/usr/bin/cliprag-shell", true); err == nil {
			t.Fatal("malformed block accepted")
		}
		after, _ := os.ReadFile(path)
		if string(after) != bad {
			t.Fatal("malformed file modified")
		}
	}
	link := path + "-link"
	os.Symlink(path, link)
	if _, err := UpdateRC(link, "bash", "/usr/bin/cliprag-shell", true); err == nil {
		t.Fatal("symlink silently overwritten")
	}
	if _, err := UpdateRC("relative", "bash", "/usr/bin/cliprag-shell", true); err == nil {
		t.Fatal("relative file accepted")
	}
}
