package storage

import (
	"cliprag/internal/config"
	"cliprag/internal/history"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("cliprag_fold", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if value, ok := args[0].(string); ok {
			return strings.ToLower(value), nil
		}
		return "", nil
	})
}

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	path = absolute
	dir := filepath.Dir(path)
	if st, e := os.Lstat(dir); e == nil && st.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("database directory must not be a symlink")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	if st, e := os.Lstat(path); e == nil && st.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("database must not be a symlink")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()
	dsn := (&url.URL{Scheme: "file", Path: path}).String() + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(DELETE)&_pragma=secure_delete(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db}
	if err = s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) migrate() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 2 {
		return errors.New("database schema newer than this ClipRAG version")
	}
	if version == 0 {
		if _, err = tx.Exec(schemaV1); err != nil {
			return err
		}
		version = 1
	}
	if version == 1 {
		if _, err = tx.Exec(schemaV2); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) Close() error { return s.db.Close() }

const columns = "id,content,hash,category,first_copied,last_copied,copy_count,restore_count,pinned,autocomplete_count"

func scan(row interface{ Scan(...any) error }) (history.Entry, error) {
	var e history.Entry
	err := row.Scan(&e.ID, &e.Content, &e.Hash, &e.Category, &e.FirstCopied, &e.LastCopied, &e.CopyCount, &e.RestoreCount, &e.Pinned, &e.AutocompleteCount)
	return e, err
}
func cleanup(ctx context.Context, tx *sql.Tx, cfg config.Settings, now int64) error {
	if _, e := tx.ExecContext(ctx, "DELETE FROM entries WHERE pinned=0 AND last_copied < ?", now-int64(cfg.RetentionDays)*int64(24*time.Hour/time.Millisecond)); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `DELETE FROM entries WHERE pinned=0 AND id IN (SELECT id FROM entries WHERE pinned=0 ORDER BY last_copied DESC,id DESC LIMIT -1 OFFSET ?)`, cfg.MaxEntries)
	return e
}
func (s *Store) Capture(ctx context.Context, text, category string, now int64, cfg config.Settings) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO entries(content,hash,category,first_copied,last_copied) VALUES(?,?,?,?,?) ON CONFLICT(hash) DO UPDATE SET last_copied=excluded.last_copied, copy_count=entries.copy_count+1`, text, history.Identity(text), category, now, now)
	if err != nil {
		return err
	}
	if err = cleanup(ctx, tx, cfg, now); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Cleanup(ctx context.Context, cfg config.Settings, now int64) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = cleanup(ctx, tx, cfg, now); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Get(ctx context.Context, id int64) (history.Entry, error) {
	return scan(s.db.QueryRowContext(ctx, "SELECT "+columns+" FROM entries WHERE id=?", id))
}
func (s *Store) List(ctx context.Context, q history.Query) (history.Page, error) {
	page := history.Page{Entries: []history.Entry{}}
	where := " WHERE 1=1"
	args := []any{}
	if q.Search != "" {
		where += " AND instr(cliprag_fold(content),cliprag_fold(?))>0"
		args = append(args, q.Search)
	}
	if q.Category != "" {
		where += " AND category=?"
		args = append(args, q.Category)
	}
	if q.Pinned {
		where += " AND pinned=1"
	}
	order := "last_copied DESC,id DESC"
	switch q.Sort {
	case "used":
		order = "copy_count DESC,last_copied DESC,id DESC"
	case "alpha":
		order = "content COLLATE NOCASE ASC,id DESC"
	case "", "newest":
	default:
		return page, errors.New("invalid sort")
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		return page, errors.New("invalid offset")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return page, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM entries"+where, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	args = append(args, q.Limit, q.Offset)
	rows, err := tx.QueryContext(ctx, "SELECT "+columns+" FROM entries"+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", args...)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		e, er := scan(rows)
		if er != nil {
			rows.Close()
			return page, er
		}
		page.Entries = append(page.Entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return page, err
	}
	return page, tx.Commit()
}
func (s *Store) Counts(ctx context.Context) (map[string]int, error) {
	out := map[string]int{"All History": 0, "Most Used": 0, "Pinned": 0}
	rows, e := s.db.QueryContext(ctx, "SELECT category,count(*),sum(pinned) FROM entries GROUP BY category")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		var n, p int
		if e = rows.Scan(&c, &n, &p); e != nil {
			return nil, e
		}
		out[c] = n
		out["All History"] += n
		out["Most Used"] += n
		out["Pinned"] += p
	}
	return out, rows.Err()
}
func (s *Store) modify(ctx context.Context, query string, args ...any) error {
	r, e := s.db.ExecContext(ctx, query, args...)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e == nil && n == 0 {
		return sql.ErrNoRows
	}
	return e
}
func (s *Store) Pin(ctx context.Context, id int64, p bool) error {
	return s.modify(ctx, "UPDATE entries SET pinned=? WHERE id=?", p, id)
}
func (s *Store) Restore(ctx context.Context, id int64) error {
	return s.modify(ctx, "UPDATE entries SET restore_count=restore_count+1 WHERE id=?", id)
}
func (s *Store) Delete(ctx context.Context, id int64) error {
	return s.modify(ctx, "DELETE FROM entries WHERE id=?", id)
}
func (s *Store) Clear(ctx context.Context, confirmation string) error {
	if confirmation != "CLEAR" {
		return errors.New("confirmation required")
	}
	_, e := s.db.ExecContext(ctx, "DELETE FROM entries")
	return e
}
func (s *Store) Settings(ctx context.Context) (config.Settings, error) {
	c := config.Default()
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE id=1").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal([]byte(raw), &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}
func (s *Store) SaveSettings(ctx context.Context, c config.Settings) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO settings(id,value) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET value=excluded.value", string(b)); err != nil {
		return err
	}
	if err = cleanup(ctx, tx, c, time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Integrity(ctx context.Context) error {
	var result string
	if e := s.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); e != nil {
		return e
	}
	if !strings.EqualFold(result, "ok") {
		return fmt.Errorf("database integrity check failed")
	}
	return nil
}
