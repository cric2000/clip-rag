package storage

import (
	"cliprag/internal/history"
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
)

// RecordAutocompleteUse only updates a surviving, eligible clip in an existing
// current database. It never captures text, creates history, or runs migrations.
// Acceptance counts are separate from clipboard copies and command execution.
func RecordAutocompleteUse(ctx context.Context, path, content string) error {
	if len(content) > MaxSuggestionBytes || history.Eligible(content, false, MaxSuggestionBytes) != nil {
		return errors.New("ineligible command")
	}
	db, err := openUsageDatabase(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	hash := history.Identity(content)
	var category string
	if err = db.QueryRowContext(ctx, "SELECT category FROM entries WHERE hash=? AND content=?", hash, content).Scan(&category); err != nil {
		return err
	}
	if !SuggestibleCommand(content, category) {
		return errors.New("ineligible command")
	}
	result, err := db.ExecContext(ctx, "UPDATE entries SET autocomplete_count=autocomplete_count+1 WHERE hash=? AND content=? AND category=?", hash, content, category)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DataVersion changes when another SQLite connection commits. The desktop
// checks this small metadata value to refresh counters changed by shell helpers.
func (s *Store) DataVersion(ctx context.Context) (int64, error) {
	var version int64
	err := s.db.QueryRowContext(ctx, "PRAGMA data_version").Scan(&version)
	return version, err
}

// Both integrations use an existing database without creating or migrating it.
func openUsageDatabase(ctx context.Context, path string) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("history database must be a regular file")
	}
	dsn := (&url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=rw&_pragma=busy_timeout(40)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var version int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version != 2 {
		db.Close()
		return nil, errors.New("open the updated ClipRAG app to enable usage counts")
	}
	return db, nil
}

func RecordBrowserAutocompleteUse(ctx context.Context, path string, id int64, hash string) error {
	db, err := openUsageDatabase(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	var text string
	if err = db.QueryRowContext(ctx, "SELECT content FROM entries WHERE id=? AND hash=?", id, hash).Scan(&text); err != nil {
		return err
	}
	if !BrowserTextEligible(text, true) {
		return errors.New("ineligible clip")
	}
	result, err := db.ExecContext(ctx, "UPDATE entries SET autocomplete_count=autocomplete_count+1 WHERE id=? AND hash=?", id, hash)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return err
}
