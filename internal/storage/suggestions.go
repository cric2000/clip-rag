package storage

import (
	"cliprag/internal/history"
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// OpenReadOnly never creates a database, changes permissions, or runs migrations.
// It can coexist with the desktop app and also works while the app is closed.
func OpenReadOnly(path string) (*Store, error) {
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
	dsn := (&url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(40)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err == nil && (version < 1 || version > 2) {
		err = errors.New("unsupported history schema")
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

const MaxSuggestionBytes = 4096

// SuggestibleCommand rejects terminal controls, multiline clips, detected secrets,
// and non-command categories. No shell parser or evaluator ever sees this text.
func SuggestibleCommand(content, category string) bool {
	if strings.TrimSpace(content) != content || history.Eligible(content, false, MaxSuggestionBytes) != nil {
		return false
	}
	for _, r := range content {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	switch category {
	case "Git", "Linux", "Docker", "Kubernetes":
		return true
	case "Text":
		// Common developer commands which intentionally remain Text in the app.
		// Ordinary prose, standalone URLs, JSON and SQL are not shell completions.
		fields := strings.Fields(content)
		if len(fields) < 2 {
			return false
		}
		if fields[0] == "sudo" {
			fields = fields[1:]
		}
		if len(fields) == 0 {
			return false
		}
		command := fields[0]
		if strings.HasPrefix(command, "./") || strings.HasPrefix(command, "/") || strings.HasPrefix(command, "~/") {
			return true
		}
		switch command {
		case "go", "npm", "npx", "pnpm", "yarn", "bun", "node", "python", "python3", "pip", "pip3", "uv", "poetry", "cargo", "rustc", "make", "cmake", "gcc", "g++", "dotnet", "java", "javac", "mvn", "gradle", "ruby", "bundle", "php", "composer", "terraform", "ansible", "ansible-playbook", "aws", "gcloud", "az", "cd", "ls", "pwd", "cat", "find", "echo", "printf", "export", "source", "bash", "zsh", "sh", "env", "rsync", "scp", "sftp", "ffmpeg", "jq", "yq", "head", "tail", "less", "sort", "uniq", "wc", "xargs", "touch", "mkdir", "cp", "mv", "rm", "rmdir", "ln", "man", "which", "type", "test":
			return true
		}
	}
	return false
}

func ValidSuggestionPrefix(prefix string) bool {
	if !utf8.ValidString(prefix) || utf8.RuneCountInString(prefix) < 3 || len(prefix) > MaxSuggestionBytes || strings.TrimSpace(prefix) == "" {
		return false
	}
	for _, r := range prefix {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return history.Eligible(prefix, false, MaxSuggestionBytes) == nil
}

// Suggestions matches a literal, case-sensitive prefix of the whole command.
// It only reads history: accepting a completion does not claim command execution
// or alter the clipboard/restore counts. Work and output are bounded for typing.
func (s *Store) Suggestions(ctx context.Context, prefix string, limit int) ([]string, error) {
	out := []string{}
	if !ValidSuggestionPrefix(prefix) {
		return out, nil
	}
	if limit < 1 || limit > 20 {
		limit = 5
	}
	rows, err := s.db.QueryContext(ctx, `SELECT content,category FROM entries
 WHERE substr(content,1,length(?))=? AND content<>? AND length(CAST(content AS BLOB))<=?
 ORDER BY pinned DESC,copy_count DESC,last_copied DESC,id DESC LIMIT 200`, prefix, prefix, prefix, MaxSuggestionBytes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var content, category string
		if err = rows.Scan(&content, &category); err != nil {
			return nil, err
		}
		if SuggestibleCommand(content, category) {
			out = append(out, content)
			if len(out) == limit {
				break
			}
		}
	}
	return out, rows.Err()
}
