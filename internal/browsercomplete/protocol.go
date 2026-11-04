// Package browsercomplete provides the local, framed native-messaging host.
package browsercomplete

import (
	"cliprag/internal/storage"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

const MaxMessageBytes = 256 * 1024

type Request struct {
	RequestID string `json:"requestId"`
	Type      string `json:"type"`
	Prefix    string `json:"prefix"`
	Multiline bool   `json:"multiline"`
	URLsOnly  bool   `json:"urlsOnly"`
	Token     string `json:"token"`
}
type Response struct {
	RequestID string                 `json:"requestId"`
	OK        bool                   `json:"ok"`
	Error     string                 `json:"error,omitempty"`
	Ready     bool                   `json:"ready,omitempty"`
	Matches   []storage.BrowserMatch `json:"matches,omitempty"`
}
type offered struct {
	id      int64
	hash    string
	expires time.Time
}
type Host struct {
	Database string
	tokens   map[string]offered
}

func (h *Host) Handle(request Request) Response {
	response := Response{RequestID: request.RequestID, OK: true}
	if len(request.RequestID) > 80 {
		response.OK = false
		response.Error = "invalid request"
		return response
	}
	if h.tokens == nil {
		h.tokens = make(map[string]offered)
	}
	for token, value := range h.tokens {
		if time.Now().After(value.expires) {
			delete(h.tokens, token)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	fail := func(message string) Response { response.OK = false; response.Error = message; return response }
	switch request.Type {
	case "status":
		db, err := storage.OpenReadOnly(h.Database)
		if errors.Is(err, os.ErrNotExist) {
			return response
		}
		if err != nil {
			return fail("History unavailable. Open the updated ClipRAG app.")
		}
		db.Close()
		response.Ready = true
	case "suggest":
		if !storage.ValidBrowserPrefix(request.Prefix) {
			return response
		}
		db, err := storage.OpenReadOnly(h.Database)
		if errors.Is(err, os.ErrNotExist) {
			return response
		}
		if err != nil {
			return fail("History unavailable. Try again shortly.")
		}
		matches, err := db.BrowserSuggestions(ctx, request.Prefix, request.Multiline, request.URLsOnly)
		db.Close()
		if err != nil {
			return fail("History unavailable. Try again shortly.")
		}
		if len(h.tokens)+len(matches) > 128 {
			clear(h.tokens)
		}
		for i := range matches {
			var random [24]byte
			if _, err := rand.Read(random[:]); err != nil {
				return fail("Could not prepare suggestions.")
			}
			matches[i].Token = hex.EncodeToString(random[:])
			h.tokens[matches[i].Token] = offered{matches[i].ID, matches[i].Hash, time.Now().Add(2 * time.Minute)}
		}
		response.Matches = matches
	case "accept":
		value, ok := h.tokens[request.Token]
		if !ok {
			return fail("Suggestion expired. Type again to refresh it.")
		}
		delete(h.tokens, request.Token) // One accepted offer can only count once.
		if err := storage.RecordBrowserAutocompleteUse(ctx, h.Database, value.id, value.hash); err != nil {
			return fail("Usage count unavailable.")
		}
	default:
		return fail("Unsupported request.")
	}
	return response
}

func Serve(in io.Reader, out io.Writer, database string) error {
	host := Host{Database: database}
	for {
		var header [4]byte
		_, err := io.ReadFull(in, header[:])
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		size := binary.NativeEndian.Uint32(header[:])
		if size == 0 || size > MaxMessageBytes {
			return errors.New("invalid native-message size")
		}
		payload := make([]byte, size)
		if _, err = io.ReadFull(in, payload); err != nil {
			return err
		}
		var request Request
		response := Response{Error: "Invalid request."}
		if json.Unmarshal(payload, &request) == nil {
			response = host.Handle(request)
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			return err
		}
		if len(encoded) > 1024*1024 {
			return errors.New("native response too large")
		}
		binary.NativeEndian.PutUint32(header[:], uint32(len(encoded)))
		if _, err = out.Write(header[:]); err != nil {
			return err
		}
		if _, err = out.Write(encoded); err != nil {
			return err
		}
	}
}
