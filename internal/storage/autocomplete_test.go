package storage

import (
	"cliprag/internal/config"
	"cliprag/internal/history"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAutocompleteUsageConcurrentAndIndependent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	command := "git checkout 'café $(touch /tmp/never-execute)'"
	if err = s.Capture(ctx, command, "Git", time.Now().UnixMilli(), config.Default()); err != nil {
		t.Fatal(err)
	}
	p, _ := s.List(ctx, history.Query{})
	before := p.Entries[0]
	version, _ := s.DataVersion(ctx)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := RecordAutocompleteUse(ctx, path, command); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	after, err := s.Get(ctx, before.ID)
	if err != nil || after.AutocompleteCount != 8 {
		t.Fatal(after, err)
	}
	after.AutocompleteCount = 0
	if after != before {
		t.Fatal("acceptance changed unrelated clip data", before, after)
	}
	updated, _ := s.DataVersion(ctx)
	if updated == version {
		t.Fatal("external usage update not visible through data_version")
	}
	if err = RecordAutocompleteUse(ctx, path, "git missing"); err == nil {
		t.Fatal("unknown clip counted")
	}
	if err = s.Delete(ctx, before.ID); err != nil {
		t.Fatal(err)
	}
	if err = RecordAutocompleteUse(ctx, path, command); err == nil {
		t.Fatal("deleted suggestion recreated")
	}
	link := filepath.Join(filepath.Dir(path), "link.db")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err = RecordAutocompleteUse(ctx, link, command); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestAutocompleteUsageDoesNotCreateMigrateOrCountUnsafeClips(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "missing", "history.db")
	if err := RecordAutocompleteUse(ctx, path, "git status"); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("usage created directory")
	}
	legacy := filepath.Join(t.TempDir(), "history.db")
	db, err := sql.Open("sqlite", legacy)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(schemaV1); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO entries(content,hash,category,first_copied,last_copied,copy_count,restore_count,pinned) VALUES(?,?,?,?,?,?,?,?)", "git status", history.Identity("git status"), "Git", 10, 20, 4, 2, 1); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(legacy)
	reader, err := OpenReadOnly(legacy)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := reader.Suggestions(ctx, "git", 1)
	reader.Close()
	if err != nil || len(matches) != 1 {
		t.Fatal(matches, err)
	}
	if err = RecordAutocompleteUse(ctx, legacy, "git status"); err == nil {
		t.Fatal("legacy schema accepted for writes")
	}
	after, _ := os.ReadFile(legacy)
	if string(before) != string(after) {
		t.Fatal("helper migrated legacy DB")
	}
	// The desktop migration preserves all existing data and is safe to repeat.
	s, err := Open(legacy)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.migrate(); err != nil {
		t.Fatal(err)
	}
	p, err := s.List(ctx, history.Query{})
	if err != nil || p.Total != 1 || p.Entries[0].CopyCount != 4 || p.Entries[0].RestoreCount != 2 || !p.Entries[0].Pinned || p.Entries[0].LastCopied != 20 || p.Entries[0].AutocompleteCount != 0 {
		t.Fatal(p, err)
	}
	for _, clip := range []struct{ text, category string }{{"git unsafe\x1b[31m", "Git"}, {"git multiline\nwhoami", "Git"}, {"echo normal prose", "SQL"}} {
		if err = s.Capture(ctx, clip.text, clip.category, time.Now().UnixMilli(), config.Default()); err != nil {
			t.Fatal(err)
		}
		if err = RecordAutocompleteUse(ctx, legacy, clip.text); err == nil {
			t.Fatal("unsafe or non-command clip counted")
		}
	}
}

func TestAutocompleteUsageLockTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err = s.Capture(ctx, "git status", "Git", time.Now().UnixMilli(), config.Default()); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE entries SET pinned=1"); err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 180*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err = RecordAutocompleteUse(bounded, path, "git status"); err == nil {
		t.Fatal("write bypassed lock")
	}
	if time.Since(start) > time.Second {
		t.Fatal("busy history stalled completion")
	}
}
