package history

import (
	"strings"
	"testing"
)

func TestPrivacy(t *testing.T) {
	for _, v := range []struct {
		text      string
		sensitive bool
		max       int
		valid     bool
	}{
		{"git status", false, 1024, true}, {"  \n\t", false, 1024, false}, {"abcd", false, 3, false}, {"value", true, 1024, false}, {"-----BEGIN RSA PRIVATE KEY-----", false, 1024, false}, {"AKIAABCDEFGHIJKLMNOP", false, 1024, false}, {"password=supersecret", false, 1024, false}, {`{"password":"supersecret"}`, false, 1024, false}, {"postgres://user:secret@localhost/db", false, 1024, false}, {"Authorization: Bearer abcdefghijklmnop", false, 1024, false}, {"ghp_" + strings.Repeat("a", 36), false, 1024, false}, {"eyJabcdef.eyJabcde.signature", false, 1024, false}, {"https://user:password@example.com", false, 1024, false}, {"\x00abc", false, 1024, false}, {string([]byte{0xff}), false, 1024, false}, {"  hello\n world  ", false, 1024, true},
	} {
		if got := Eligible(v.text, v.sensitive, v.max) == nil; got != v.valid {
			t.Errorf("policy mismatch for %q", v.text)
		}
	}
}
func TestIdentityPreservesWhitespace(t *testing.T) {
	if Identity("x") != Identity("x") {
		t.Fatal("unstable")
	}
	if Identity("x") == Identity("x\n") {
		t.Fatal("whitespace collapsed")
	}
}
