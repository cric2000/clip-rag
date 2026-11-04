package storage

import (
	"cliprag/internal/history"
	"context"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxBrowserClipBytes = 100 * 1024

type BrowserMatch struct {
	ID       int64  `json:"-"`
	Hash     string `json:"-"`
	Text     string `json:"text"`
	Category string `json:"category"`
	Token    string `json:"token"`
}

func ValidBrowserPrefix(prefix string) bool {
	return utf8.RuneCountInString(prefix) >= 3 && len(prefix) <= 2048 && BrowserTextEligible(prefix, true)
}

// Browser suggestions accept every text category, with the same secret policy
// as clipboard capture. Only tabs and newlines are allowed as control characters.
func BrowserTextEligible(text string, multiline bool) bool {
	if history.Eligible(text, false, MaxBrowserClipBytes) != nil {
		return false
	}
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' {
			if !multiline {
				return false
			}
			continue
		}
		if unicode.IsControl(r) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return false
		}
	}
	return true
}

func BrowserURL(text string) bool {
	u, err := url.Parse(text)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil && !strings.ContainsAny(text, "\r\n\t ")
}

func (s *Store) BrowserSuggestions(ctx context.Context, prefix string, multiline, urlsOnly bool) ([]BrowserMatch, error) {
	result := []BrowserMatch{}
	if !ValidBrowserPrefix(prefix) {
		return result, nil
	}
	prefixes := []string{prefix}
	if urlsOnly && !strings.Contains(prefix, "://") {
		prefixes = append(prefixes, "https://"+prefix, "http://"+prefix, "https://www."+prefix, "http://www."+prefix)
	}
	var conditions []string
	var args []any
	for _, value := range prefixes {
		conditions = append(conditions, "substr(cliprag_fold(content),1,length(cliprag_fold(?)))=cliprag_fold(?)")
		args = append(args, value, value)
	}
	args = append(args, urlsOnly, prefix, MaxBrowserClipBytes)
	rows, err := s.db.QueryContext(ctx, `SELECT id,hash,content,category FROM entries
 WHERE (`+strings.Join(conditions, " OR ")+`)
 AND (? OR content<>?) AND length(CAST(content AS BLOB))<=?
 ORDER BY pinned DESC,copy_count DESC,last_copied DESC,id DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Keep the JSON native-message response comfortably below Chrome's 1 MiB cap,
	// including worst-case escaping, while preserving the entire accepted clip.
	remaining := 128 * 1024
	for rows.Next() {
		var match BrowserMatch
		if err := rows.Scan(&match.ID, &match.Hash, &match.Text, &match.Category); err != nil {
			return nil, err
		}
		if !BrowserTextEligible(match.Text, multiline) || (urlsOnly && !BrowserURL(match.Text)) || len(match.Text) > remaining {
			continue
		}
		remaining -= len(match.Text)
		result = append(result, match)
		if len(result) == 5 {
			break
		}
	}
	return result, rows.Err()
}
