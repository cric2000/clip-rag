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

func openTest(t *testing.T) *Store {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "data", "history.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestStorageWorkflow(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	cfg := config.Default()
	now := time.Now().UnixMilli()
	for i, text := range []string{"git status", "docker ps", "SELECT * FROM users;", "git status"} {
		category := "Git"
		if i == 1 {
			category = "Docker"
		}
		if e := s.Capture(ctx, text, category, now+int64(i), cfg); e != nil {
			t.Fatal(e)
		}
	}
	p, e := s.List(ctx, history.Query{})
	if e != nil || p.Total != 3 {
		t.Fatalf("list: %+v %v", p, e)
	}
	first := p.Entries[0]
	if first.Content != "git status" || first.CopyCount != 2 {
		t.Fatal(first)
	}
	if e = s.Restore(ctx, first.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.Pin(ctx, first.ID, true); e != nil {
		t.Fatal(e)
	}
	p, e = s.List(ctx, history.Query{Search: "GIT", Category: "Git", Pinned: true})
	if e != nil || p.Total != 1 || p.Entries[0].RestoreCount != 1 || p.Entries[0].CopyCount != 2 {
		t.Fatalf("filter %+v %v", p, e)
	}
	p, _ = s.List(ctx, history.Query{Sort: "used"})
	if p.Entries[0].ID != first.ID {
		t.Fatal("usage sort")
	}
	p, _ = s.List(ctx, history.Query{Sort: "alpha"})
	if p.Entries[0].Content != "docker ps" {
		t.Fatal("alphabetical sort")
	}
	p, _ = s.List(ctx, history.Query{Limit: 1, Offset: 1})
	if len(p.Entries) != 1 || p.Total != 3 {
		t.Fatal("pagination")
	}
	if e = s.Pin(ctx, first.ID, false); e != nil {
		t.Fatal(e)
	}
	if e = s.Delete(ctx, first.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Get(ctx, first.ID); e != sql.ErrNoRows {
		t.Fatal(e)
	}
	if s.Clear(ctx, "") == nil {
		t.Fatal("unconfirmed clear")
	}
	if e = s.Clear(ctx, "CLEAR"); e != nil {
		t.Fatal(e)
	}
	if e = s.Integrity(ctx); e != nil {
		t.Fatal(e)
	}
}
func TestRetentionAndStableOrdering(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	cfg := config.Default()
	now := time.Now().UnixMilli()
	for _, v := range []string{"a", "b", "c"} {
		if e := s.Capture(ctx, v, "Text", now, cfg); e != nil {
			t.Fatal(e)
		}
	}
	p, _ := s.List(ctx, history.Query{})
	if p.Entries[0].Content != "c" || p.Entries[2].Content != "a" {
		t.Fatal("tie ordering")
	}
	id := p.Entries[2].ID
	s.Pin(ctx, id, true)
	cfg.MaxEntries = 1
	if e := s.Cleanup(ctx, cfg, now); e != nil {
		t.Fatal(e)
	}
	p, _ = s.List(ctx, history.Query{})
	if p.Total != 2 {
		t.Fatal("limit should exempt pin")
	}
	if e := s.Cleanup(ctx, cfg, now+31*24*60*60*1000); e != nil {
		t.Fatal(e)
	}
	p, _ = s.List(ctx, history.Query{})
	if p.Total != 1 || p.Entries[0].ID != id {
		t.Fatal("retention removed pin")
	}
}
func TestMigrationPersistencePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "history.db")
	ctx := context.Background()
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	cfg := config.Default()
	cfg.MaxEntries = 22
	if e = s.SaveSettings(ctx, cfg); e != nil {
		t.Fatal(e)
	}
	if e = s.Capture(ctx, " x\n y ", "Text", time.Now().UnixMilli(), cfg); e != nil {
		t.Fatal(e)
	}
	var version int
	s.db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 2 {
		t.Fatal(version)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	got, _ := s.Settings(ctx)
	if got != cfg {
		t.Fatal("settings persistence")
	}
	p, _ := s.List(ctx, history.Query{})
	if p.Entries[0].Content != " x\n y " {
		t.Fatal("formatting")
	}
	for _, v := range []struct {
		path string
		mode os.FileMode
	}{{path, 0600}, {filepath.Dir(path), 0700}} {
		st, e := os.Stat(v.path)
		if e != nil || st.Mode().Perm() != v.mode {
			t.Fatalf("permissions: %v %v", st, e)
		}
	}
}
func TestTransactionsAndConcurrentDedup(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	cfg := config.Default()
	now := time.Now().UnixMilli()
	if e := s.Capture(ctx, "first", "Text", now, cfg); e != nil {
		t.Fatal(e)
	}
	tx, e := s.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec("UPDATE entries SET content='should rollback'"); e != nil {
		t.Fatal(e)
	}
	tx.Rollback()
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if e := s.Capture(ctx, "first", "Text", now, cfg); e != nil {
				t.Error(e)
			}
		})
	}
	wg.Wait()
	p, e := s.List(ctx, history.Query{})
	if e != nil || p.Total != 1 || p.Entries[0].CopyCount != 21 || p.Entries[0].Content != "first" {
		t.Fatalf("dedup %+v %v", p, e)
	}
	p, e = s.List(ctx, history.Query{Search: "%"})
	if e != nil || p.Total != 0 {
		t.Fatal("search must be literal")
	}
	if _, e = s.List(ctx, history.Query{Sort: "; DROP TABLE entries"}); e == nil {
		t.Fatal("unsafe sort")
	}
}
func TestRejectNewerSchema(t *testing.T) {
	s := openTest(t)
	s.db.Exec("PRAGMA user_version=3")
	if s.migrate() == nil {
		t.Fatal("accepted incompatible schema")
	}
}

func TestUnicodeSearch(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if e := s.Capture(ctx, "Café Москва", "Text", time.Now().UnixMilli(), config.Default()); e != nil {
		t.Fatal(e)
	}
	p, e := s.List(ctx, history.Query{Search: "CAFÉ МОСКВА"})
	if e != nil || p.Total != 1 {
		t.Fatalf("unicode search %+v %v", p, e)
	}
}
