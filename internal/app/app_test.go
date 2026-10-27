package app

import (
	"cliprag/internal/clipboard"
	"cliprag/internal/history"
	"cliprag/internal/storage"
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeClipboard struct {
	mu       sync.Mutex
	callback func(clipboard.Event)
	written  string
	fail     bool
	stopped  bool
}

func (f *fakeClipboard) Start(_ context.Context, fn func(clipboard.Event)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callback = fn
	return nil
}
func (f *fakeClipboard) ReadText(context.Context) (string, error) { return "", nil }
func (f *fakeClipboard) WriteText(_ context.Context, s string) error {
	f.mu.Lock()
	f.written = s
	cb, fail := f.callback, f.fail
	f.mu.Unlock()
	if fail {
		return errors.New("write failure")
	}
	if cb != nil {
		cb(clipboard.Event{Text: s, Self: true, Sequence: 99})
	}
	return nil
}
func (f *fakeClipboard) Status() clipboard.Status {
	return clipboard.Status{Supported: true, Healthy: true}
}
func (f *fakeClipboard) Stop() error { f.mu.Lock(); f.stopped = true; f.mu.Unlock(); return nil }
func newEngine(t *testing.T) (*Engine, *fakeClipboard, *Service) {
	t.Helper()
	s, e := storage.Open(filepath.Join(t.TempDir(), "data", "history.db"))
	if e != nil {
		t.Fatal(e)
	}
	f := &fakeClipboard{}
	engine, e := New(s, f, "test", nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { engine.Stop(); s.Close() })
	return engine, f, NewService(engine)
}
func TestCaptureRestoreAndPrivacy(t *testing.T) {
	e, f, s := newEngine(t)
	ctx := context.Background()
	for _, v := range []clipboard.Event{{Text: "git status", Sequence: 1}, {Text: "git status", Sequence: 1}, {Text: "git status", Sequence: 2}, {Text: "   ", Sequence: 3}, {Text: "password=supersecret", Sequence: 4}, {Text: "sensitive", Sensitive: true, Sequence: 5}} {
		e.capture(ctx, v)
	}
	p, err := s.List(history.Query{})
	if err != nil || p.Total != 1 || p.Entries[0].CopyCount != 2 {
		t.Fatalf("capture %+v %v", p, err)
	}
	id := p.Entries[0].ID
	// Mimic synchronous adapter echo, then validate it is excluded by capture.
	if err = s.Copy(id); err != nil {
		t.Fatal(err)
	}
	e.capture(ctx, clipboard.Event{Text: "git status", Self: true, Sequence: 6})
	p, _ = s.List(history.Query{})
	if p.Entries[0].CopyCount != 2 || p.Entries[0].RestoreCount != 1 || f.written != "git status" {
		t.Fatal("restore counters/echo")
	}
	f.fail = true
	if s.Copy(id) == nil {
		t.Fatal("write error hidden")
	}
	p, _ = s.List(history.Query{})
	if p.Entries[0].RestoreCount != 1 {
		t.Fatal("failed copy counted")
	}
	cfg := s.GetSettings()
	cfg.Monitoring = false
	if err = s.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	e.capture(ctx, clipboard.Event{Text: "docker ps", Sequence: 7})
	p, _ = s.List(history.Query{})
	if p.Total != 1 {
		t.Fatal("pause ignored")
	}
	cfg.Monitoring = true
	s.SaveSettings(cfg)
	e.capture(ctx, clipboard.Event{Text: "docker ps", Sequence: 8})
	p, _ = s.List(history.Query{})
	if p.Total != 2 {
		t.Fatal("resume ignored")
	}
	cfg.MaxEntryBytes = 1024
	s.SaveSettings(cfg)
	e.capture(ctx, clipboard.Event{Text: string(make([]byte, 2048)), Sequence: 9})
	p, _ = s.List(history.Query{})
	if p.Total != 2 {
		t.Fatal("size policy ignored")
	}
}
func TestEngineLifecycleCancellation(t *testing.T) {
	e, f, s := newEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	if err := e.Start(ctx); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	cb := f.callback
	f.mu.Unlock()
	cb(clipboard.Event{Text: "  x\n y  ", Sequence: 1})
	deadline := time.Now().Add(time.Second)
	for {
		p, _ := s.List(history.Query{})
		if p.Total == 1 {
			if p.Entries[0].Content != "  x\n y  " {
				t.Fatal("format changed")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("event not captured")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-e.done:
	case <-time.After(time.Second):
		t.Fatal("worker leaked")
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	if !f.stopped {
		t.Fatal("adapter not stopped")
	}
}
func TestClearInvalidatesQueuedEvents(t *testing.T) {
	e, _, s := newEngine(t)
	cfg := s.GetSettings()
	cfg.MaxEntries = 0
	if s.SaveSettings(cfg) == nil {
		t.Fatal("invalid settings")
	}
	if s.Clear("") == nil {
		t.Fatal("missing confirmation")
	}
	before := e.epoch.Load()
	if err := s.Clear("CLEAR"); err != nil {
		t.Fatal(err)
	}
	e.captureAt(context.Background(), clipboard.Event{Text: "stale", Sequence: 1}, before)
	page, err := s.List(history.Query{})
	if err != nil || page.Total != 0 {
		t.Fatal("clear allowed a stale in-flight event")
	}
	if e.epoch.Load() == before {
		t.Fatal("stale queue not invalidated")
	}
}
