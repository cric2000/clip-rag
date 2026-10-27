//go:build linux

package linux

import (
	"cliprag/internal/clipboard"
	"context"
	"crypto/sha256"
	"github.com/godbus/dbus/v5"
	"os"
	"testing"
	"time"
)

func TestEnvironmentSelection(t *testing.T) {
	for _, v := range []struct {
		session, desktop string
		want             bool
	}{{"wayland", "ubuntu:GNOME", true}, {"wayland", "sway", false}, {"x11", "GNOME", true}, {"", "", false}} {
		t.Setenv("XDG_SESSION_TYPE", v.session)
		t.Setenv("XDG_CURRENT_DESKTOP", v.desktop)
		_, err := New()
		if (err == nil) != v.want {
			t.Fatal(v, err)
		}
	}
}
func TestWaylandStoppedAndLimits(t *testing.T) {
	w := NewWayland()
	if w.Status().Healthy {
		t.Fatal("not started")
	}
	if w.Stop() != nil {
		t.Fatal("stop must be idempotent")
	}
	if w.WriteText(context.Background(), "text") == nil {
		t.Fatal("write while stopped")
	}
	if _, e := w.ReadText(context.Background()); e == nil {
		t.Fatal("read while stopped")
	}
	if w.Capture(dbus.Sender(":1.99"), "value", false, 1) == nil {
		t.Fatal("capture while stopped")
	}
}

// Run in an isolated bus: CLIPRAG_DBUS_TEST=1 dbus-run-session -- go test ...
func TestWaylandPrivateBus(t *testing.T) {
	if os.Getenv("CLIPRAG_DBUS_TEST") != "1" {
		t.Skip("requires isolated D-Bus session")
	}
	shell, e := dbus.ConnectSessionBus()
	if e != nil {
		t.Fatal(e)
	}
	defer shell.Close()
	if _, e = shell.RequestName("org.gnome.Shell", dbus.NameFlagDoNotQueue); e != nil {
		t.Fatal(e)
	}
	w := NewWayland()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan clipboard.Event, 8)
	if e = w.Start(ctx, func(v clipboard.Event) { events <- v }); e != nil {
		t.Fatal(e)
	}
	defer w.Stop()
	if w.Start(ctx, nil) == nil {
		t.Fatal("double start accepted")
	}
	obj := shell.Object(ServiceName, ObjectPath)
	for range 2 {
		if e = obj.Call(ServiceName+".Capture", 0, "git status", false, uint64(1)).Err; e != nil {
			t.Fatal(e)
		}
	}
	if len(events) != 1 {
		t.Fatalf("duplicate sequence: %d", len(events))
	}
	if e = obj.Call(ServiceName+".Capture", 0, "git status", false, uint64(2)).Err; e != nil {
		t.Fatal(e)
	}
	if len(events) != 2 {
		t.Fatal("same-content external event discarded")
	}
	w.mu.Lock()
	w.expectedHash = sha256.Sum256([]byte("restored"))
	w.expectedUntil = time.Now().Add(time.Second)
	w.mu.Unlock()
	if e = obj.Call(ServiceName+".Capture", 0, "restored", false, uint64(3)).Err; e != nil {
		t.Fatal(e)
	}
	<-events
	<-events
	if !((<-events).Self) {
		t.Fatal("own write not identified")
	}
	if e = obj.Call(ServiceName+".Capture", 0, "restored", false, uint64(4)).Err; e != nil {
		t.Fatal(e)
	}
	if (<-events).Self {
		t.Fatal("later independent same text suppressed")
	}
	stranger, e := dbus.ConnectSessionBus()
	if e != nil {
		t.Fatal(e)
	}
	defer stranger.Close()
	if stranger.Object(ServiceName, ObjectPath).Call(ServiceName+".Capture", 0, "injected", false, uint64(3)).Err == nil {
		t.Fatal("untrusted sender accepted")
	}
	if obj.Call(ServiceName+".Capture", 0, "invalid", false, uint64(0)).Err == nil {
		t.Fatal("invalid sequence accepted")
	}
	if _, e = w.ReadText(ctx); e == nil {
		t.Fatal("missing extension error lost")
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for w.Status().Healthy {
		if time.Now().After(deadline) {
			t.Fatal("cancellation leaked")
		}
		time.Sleep(time.Millisecond)
	}
	if e = w.Stop(); e != nil {
		t.Fatal(e)
	}
	if e = w.Start(context.Background(), nil); e != nil {
		t.Fatal("restart:", e)
	}
}
