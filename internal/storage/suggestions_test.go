package storage

import (
	"cliprag/internal/config"
	"cliprag/internal/history"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSuggestionsRankAndMatchLiteralPrefix(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	cfg := config.Default()
	now := time.Now().UnixMilli()
	for i, text := range []string{"git checkout main", "git cherry-pick abc", "git checkout feature", "git checkout feature", "git status", "git CHERRY-PICK wrong-case", "echo '100%_done'", "echo '100ABdone'"} {
		if err := s.Capture(ctx, text, "Git", now+int64(i), cfg); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := s.List(ctx, history.Query{Search: "cherry-pick abc"})
	if err := s.Pin(ctx, p.Entries[0].ID, true); err != nil {
		t.Fatal(err)
	}
	got, err := s.Suggestions(ctx, "git ch", 5)
	want := []string{"git cherry-pick abc", "git checkout feature", "git checkout main"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q (%v), want %q", got, err, want)
	}
	got, err = s.Suggestions(ctx, "echo '100%_", 5)
	if err != nil || len(got) != 1 || got[0] != "echo '100%_done'" {
		t.Fatalf("wildcards not literal: %q %v", got, err)
	}
	got, _ = s.Suggestions(ctx, "git checkout feature", 5)
	if len(got) != 0 {
		t.Fatal("exact command must not suggest itself")
	}
	got, _ = s.Suggestions(ctx, "git ch", 1)
	if len(got) != 1 {
		t.Fatal("limit ignored")
	}
}

func TestSuggestionsExcludeUnsafeAndNonCommandClips(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	cfg := config.Default()
	now := time.Now().UnixMilli()
	fixtures := []struct{ text, category string }{
		{"echo safe", "Text"}, {"echo café", "Text"}, {"npm run dev", "Text"},
		{"echo multiline\nwhoami", "Linux"}, {"echo carriage\rwhoami", "Linux"},
		{"echo escape\x1b[2J", "Linux"}, {"echo tab\tvalue", "Linux"},
		{"echo bidi\u202Evalue", "Linux"}, {"echo split\u2028value", "Linux"},
		{"echo password=not-a-real-password", "Linux"}, {"echo " + strings.Repeat("x", 5000), "Linux"},
		{"echo ordinary prose", "SQL"}, {"echo link", "URLs"}, {"echo json", "JSON"},
		{"echo unknown", "unknown"}, {"echo fenced code", "Code"},
	}
	for i, v := range fixtures {
		if err := s.Capture(ctx, v.text, v.category, now+int64(i), cfg); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Suggestions(ctx, "echo ", 20)
	if err != nil || !reflect.DeepEqual(got, []string{"echo café", "echo safe"}) {
		t.Fatalf("unexpected candidates: %q %v", got, err)
	}
	got, _ = s.Suggestions(ctx, "npm", 5)
	if len(got) != 1 || got[0] != "npm run dev" {
		t.Fatal(got)
	}
	for _, prefix := range []string{"", "gi", "\x1b[2J", "echo\n", "echo\r", "echo\u202E", strings.Repeat("x", MaxSuggestionBytes+1), string([]byte{0xff, 0xfe, 0xfd})} {
		got, err = s.Suggestions(ctx, prefix, 5)
		if err != nil || len(got) != 0 {
			t.Fatalf("invalid prefix returned results: %v", err)
		}
	}
}

func TestReadOnlyHistoryDoesNotCreateOrModify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-created", "history.db")
	if _, err := OpenReadOnly(path); !os.IsNotExist(err) {
		t.Fatalf("missing DB: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("lookup created a directory")
	}
	writer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	ctx := context.Background()
	cfg := config.Default()
	now := time.Now().UnixMilli()
	if err = writer.Capture(ctx, "git status", "Git", now, cfg); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	reader, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got, err := reader.Suggestions(ctx, "git", 5)
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	if err = reader.Capture(ctx, "git log", "Git", now, cfg); err == nil {
		t.Fatal("read-only connection accepted a write")
	}
	after, _ := os.Stat(path)
	if before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("lookup modified database")
	}
	if err = writer.Capture(ctx, "git log", "Git", now+1, cfg); err != nil {
		t.Fatal(err)
	}
	got, err = reader.Suggestions(ctx, "git", 5)
	if err != nil || len(got) != 2 {
		t.Fatalf("new capture not visible: %q %v", got, err)
	}
	link := filepath.Join(filepath.Dir(path), "link.db")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if db, e := OpenReadOnly(link); e == nil {
		db.Close()
		t.Fatal("symlink accepted")
	}
}
