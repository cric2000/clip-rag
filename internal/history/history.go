package history

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Entry struct {
	ID           int64  `json:"id"`
	Content      string `json:"content"`
	Hash         string `json:"hash"`
	Category     string `json:"category"`
	FirstCopied  int64  `json:"firstCopied"`
	LastCopied   int64  `json:"lastCopied"`
	CopyCount    int64  `json:"copyCount"`
	RestoreCount int64  `json:"restoreCount"`
	Pinned       bool   `json:"pinned"`
}
type Query struct {
	Search   string `json:"search"`
	Category string `json:"category"`
	Pinned   bool   `json:"pinned"`
	Sort     string `json:"sort"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
}
type Page struct {
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"`
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN (?:[A-Z0-9 ]*PRIVATE KEY|OPENSSH PRIVATE KEY)-----`),
	regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),
	regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-(?:proj-)?[A-Za-z0-9_-]{20,}|xox[baprs]-[A-Za-z0-9-]{15,})\b`),
	regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret|password|passwd|access[_-]?token|auth[_-]?token)["']?\s*[=:]\s*["']?[^\s"']{6,}`),
	regexp.MustCompile(`(?i)\bauthorization\s*:\s*bearer\s+\S+`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`),
	regexp.MustCompile(`(?i)(?:https?|postgres(?:ql)?|mysql|redis|mongodb(?:\+srv)?)://[^\s/:]+:[^\s/@]+@`),
}
var ErrExcluded = errors.New("clipboard content excluded by privacy or size policy")

func Eligible(s string, sensitive bool, max int) error {
	if sensitive || len(s) > max || !utf8.ValidString(s) || strings.TrimSpace(s) == "" || strings.ContainsRune(s, 0) {
		return ErrExcluded
	}
	for _, p := range secretPatterns {
		if p.MatchString(s) {
			return ErrExcluded
		}
	}
	return nil
}
func Identity(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
