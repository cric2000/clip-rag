package browsercomplete

import (
	"bytes"
	"cliprag/internal/config"
	"cliprag/internal/history"
	"cliprag/internal/storage"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Host, *storage.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	now := time.Now().UnixMilli()
	for _, item := range []struct{ text, category string }{
		{"https://example.test/guide?x=%25_value", "URLs"},
		{"Hello from my clipboard", "Text"}, {"Hello <img src=x onerror=alert(1)>", "Code"},
		{"SELECT id FROM users;", "SQL"}, {"{\"name\":\"Anmol\"}", "JSON"},
		{"Hello first line\nsecond line\tindented", "Text"},
		{"Hello 👩‍💻 developer", "Text"},
		{"Hello password=supersecret", "Text"}, {"Hello escape\x1b[2J", "Text"},
		{"git checkout main", "Git"},
	} {
		if err := db.Capture(context.Background(), item.text, item.category, now, config.Default()); err != nil {
			t.Fatal(err)
		}
	}
	return &Host{Database: path}, db
}

func TestBrowserMatchesAllTextCategoriesWithoutChangingHistory(t *testing.T) {
	host, _ := fixture(t)
	before, _ := os.ReadFile(host.Database)
	for _, prefix := range []string{"https://example.test/guide?x=%25_", "hello", "select", "{\"n", "git ch"} {
		response := host.Handle(Request{Type: "suggest", Prefix: prefix, Multiline: true})
		if !response.OK || len(response.Matches) == 0 {
			t.Fatal(prefix, response)
		}
		for _, match := range response.Matches {
			if match.Token == "" || strings.Contains(match.Text, "password=") || strings.ContainsRune(match.Text, 27) {
				t.Fatal("unsafe or untagged result", match)
			}
		}
	}
	response := host.Handle(Request{Type: "suggest", Prefix: "hello", Multiline: false})
	for _, match := range response.Matches {
		if strings.ContainsAny(match.Text, "\n\t") {
			t.Fatal("multiline clip offered to single-line field")
		}
	}
	response = host.Handle(Request{Type: "suggest", Prefix: "hello", URLsOnly: true})
	if len(response.Matches) != 0 {
		t.Fatal("non-URL offered for address bar")
	}
	after, _ := os.ReadFile(host.Database)
	if !bytes.Equal(before, after) {
		t.Fatal("lookup changed history")
	}
}

func TestBrowserCountsOnlyIssuedAcceptedTokensOnce(t *testing.T) {
	host, db := fixture(t)
	response := host.Handle(Request{Type: "suggest", Prefix: "select", Multiline: true})
	match := response.Matches[0]
	page, _ := db.List(context.Background(), history.Query{Search: "SELECT"})
	before := page.Entries[0]
	accepted := host.Handle(Request{Type: "accept", Token: match.Token})
	if !accepted.OK {
		t.Fatal(accepted)
	}
	after, err := db.Get(context.Background(), before.ID)
	if err != nil || after.AutocompleteCount != 1 {
		t.Fatal(after, err)
	}
	after.AutocompleteCount = 0
	if before != after {
		t.Fatal("acceptance changed unrelated fields")
	}
	if host.Handle(Request{Type: "accept", Token: match.Token}).OK {
		t.Fatal("replayed token counted")
	}
	if host.Handle(Request{Type: "accept", Token: strings.Repeat("a", 48)}).OK {
		t.Fatal("forged token counted")
	}
	response = host.Handle(Request{Type: "suggest", Prefix: "select", Multiline: true})
	token := response.Matches[0].Token
	value := host.tokens[token]
	value.expires = time.Now().Add(-time.Second)
	host.tokens[token] = value
	if host.Handle(Request{Type: "accept", Token: token}).OK {
		t.Fatal("expired token counted")
	}
	response = host.Handle(Request{Type: "suggest", Prefix: "select", Multiline: true})
	if err = db.Delete(context.Background(), before.ID); err != nil {
		t.Fatal(err)
	}
	if host.Handle(Request{Type: "accept", Token: response.Matches[0].Token}).OK {
		t.Fatal("deleted clip recreated")
	}
}

func frame(payload []byte) []byte {
	var header [4]byte
	binary.NativeEndian.PutUint32(header[:], uint32(len(payload)))
	return append(header[:], payload...)
}
func TestNativeFramingAndBounds(t *testing.T) {
	host, _ := fixture(t)
	input := append(frame([]byte(`{"requestId":"one","type":"suggest","prefix":"hello","multiline":true}`)), frame([]byte(`{"requestId":"two","type":"status"}`))...)
	var output bytes.Buffer
	if err := Serve(bytes.NewReader(input), &output, host.Database); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		header := output.Next(4)
		if len(header) != 4 {
			t.Fatal("missing header")
		}
		var response Response
		if err := json.Unmarshal(output.Next(int(binary.NativeEndian.Uint32(header))), &response); err != nil || !response.OK || response.RequestID != id {
			t.Fatal(response, err)
		}
	}
	var oversized [4]byte
	binary.NativeEndian.PutUint32(oversized[:], MaxMessageBytes+1)
	if Serve(bytes.NewReader(oversized[:]), &output, host.Database) == nil {
		t.Fatal("oversized request accepted")
	}
	if Serve(bytes.NewReader([]byte{1, 2}), &output, host.Database) == nil {
		t.Fatal("truncated header accepted")
	}
	missing := &Host{Database: filepath.Join(t.TempDir(), "absent", "history.db")}
	if !missing.Handle(Request{Type: "suggest", Prefix: "hello"}).OK {
		t.Fatal("missing history should be empty")
	}
	if _, err := os.Stat(filepath.Dir(missing.Database)); !os.IsNotExist(err) {
		t.Fatal("browser created missing history")
	}
}

func TestBrowserAddressBarMatchesDomainAndExactSavedURL(t *testing.T) {
	host, db := fixture(t)
	ctx := context.Background()
	for _, text := range []string{"http://example.test/other", "https://www.example.test/path"} {
		if err := db.Capture(ctx, text, "URLs", time.Now().UnixMilli(), config.Default()); err != nil {
			t.Fatal(err)
		}
	}
	for _, prefix := range []string{"example.test", "EXAMPLE.TEST", "https://example.test/guide?x=%25_value"} {
		result := host.Handle(Request{Type: "suggest", Prefix: prefix, URLsOnly: true})
		if !result.OK || len(result.Matches) == 0 {
			t.Fatal(prefix, result)
		}
		for _, match := range result.Matches {
			if !storage.BrowserURL(match.Text) {
				t.Fatal("non-URL offered", match)
			}
		}
	}
	result := host.Handle(Request{Type: "suggest", Prefix: "example.test/guide?x=%25_", URLsOnly: true})
	if !result.OK || len(result.Matches) != 1 || result.Matches[0].Text != "https://example.test/guide?x=%25_value" {
		t.Fatal("literal URL path prefix", result)
	}
}
