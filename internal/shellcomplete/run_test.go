package shellcomplete

import (
	"bytes"
	"cliprag/internal/config"
	"cliprag/internal/storage"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIReadsExactPrefixWithoutChangingHistory(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	path := filepath.Join(base, "cliprag", "history.db")
	writer, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	command := "git checkout 'a $(touch /tmp/cliprag-not-executed)'"
	if err = writer.Capture(context.Background(), command, "Git", time.Now().UnixMilli(), config.Default()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	var out, stderr bytes.Buffer
	code := Run([]string{"suggest", "--json"}, strings.NewReader("git ch"), &out, &stderr, "/usr/bin/cliprag-shell")
	if code != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), command) {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), stderr.String())
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("lookup altered history")
	}
}
func TestCLIEmptyHistoryAndBounds(t *testing.T) {
	base := filepath.Join(t.TempDir(), "not-created")
	t.Setenv("XDG_DATA_HOME", base)
	var out, stderr bytes.Buffer
	if code := Run([]string{"suggest", "--json"}, strings.NewReader("git ch"), &out, &stderr, "/usr/bin/cliprag-shell"); code != 0 || out.String() != "[]\n" || stderr.Len() != 0 {
		t.Fatal(code, out.String(), stderr.String())
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatal("CLI created missing history directory")
	}
	out.Reset()
	stderr.Reset()
	if code := Run([]string{"suggest", "--limit", "0"}, strings.NewReader("git ch"), &out, &stderr, "/usr/bin/cliprag-shell"); code == 0 || out.Len() != 0 {
		t.Fatal("invalid limit accepted")
	}
	out.Reset()
	stderr.Reset()
	if code := Run([]string{"suggest", "--json"}, strings.NewReader(strings.Repeat("x", 5000)), &out, &stderr, "/usr/bin/cliprag-shell"); code != 0 || out.String() != "[]\n" {
		t.Fatal("oversize prefix not excluded")
	}
}
func TestInitOnlySupportsNamedShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		s, err := Init(shell, "/tmp/a ' $(not-evaluated)/cliprag-shell")
		if err != nil || !strings.Contains(s, "_CLIPRAG_COMPLETION_BIN='/tmp/a '\\'' $(not-evaluated)/cliprag-shell'") {
			t.Fatal(shell, err)
		}
	}
	if _, err := Init("fish", "/usr/bin/cliprag-shell"); err == nil {
		t.Fatal("unsupported shell accepted")
	}
}
