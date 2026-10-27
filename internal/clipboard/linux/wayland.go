//go:build linux

package linux

import (
	"cliprag/internal/clipboard"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/godbus/dbus/v5"
	"sync"
	"time"
	"unicode/utf8"
)

const ServiceName = "io.github.cliprag.ClipRAG"
const ObjectPath dbus.ObjectPath = "/io/github/cliprag/ClipRAG"
const ShellName = "io.github.cliprag.Shell"
const ShellPath dbus.ObjectPath = "/io/github/cliprag/Shell"
const ShellInterface = "io.github.cliprag.Shell"

type Wayland struct {
	mu            sync.Mutex
	conn          *dbus.Conn
	cancel        context.CancelFunc
	done          chan struct{}
	callback      func(clipboard.Event)
	lastSequence  uint64
	lastSender    string
	expectedHash  [32]byte
	expectedUntil time.Time
}

func NewWayland() *Wayland { return &Wayland{} }
func (w *Wayland) Start(ctx context.Context, cb func(clipboard.Event)) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn != nil {
		return errors.New("clipboard monitor already started")
	}
	c, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("session bus: %w", err)
	}
	reply, err := c.RequestName(ServiceName, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		c.Close()
		return errors.New("another ClipRAG instance owns the session service")
	}
	if err = c.Export(w, ObjectPath, ServiceName); err != nil {
		c.Close()
		return err
	}
	child, cancel := context.WithCancel(ctx)
	w.conn, w.cancel, w.callback, w.done = c, cancel, cb, make(chan struct{})
	go func(done chan struct{}) { defer close(done); <-child.Done(); c.Close() }(w.done)
	return nil
}

// Capture is callable only by the connection owning org.gnome.Shell. No history
// is exposed on D-Bus. Sequence numbers suppress retries without deduping text.
func (w *Wayland) Capture(sender dbus.Sender, value string, sensitive bool, sequence uint64) *dbus.Error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn == nil {
		return dbus.MakeFailedError(errors.New("monitor stopped"))
	}
	var owner string
	if err := w.conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, "org.gnome.Shell").Store(&owner); err != nil || string(sender) != owner {
		return dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []any{"only GNOME Shell may submit clipboard events"})
	}
	if !utf8.ValidString(value) || len(value) > clipboard.HardLimit || sequence == 0 {
		return dbus.MakeFailedError(errors.New("invalid clipboard event"))
	}
	if w.lastSender == owner && sequence <= w.lastSequence {
		return nil
	}
	w.lastSender, w.lastSequence = owner, sequence
	if w.callback != nil {
		self := w.expectedHash == sha256.Sum256([]byte(value)) && time.Now().Before(w.expectedUntil)
		w.expectedHash = [32]byte{}
		w.callback(clipboard.Event{Text: value, Self: self, Sensitive: sensitive, Sequence: sequence})
	}
	return nil
}
func (w *Wayland) shellCall(ctx context.Context, method string, args ...any) *dbus.Call {
	w.mu.Lock()
	c := w.conn
	w.mu.Unlock()
	if c == nil {
		return &dbus.Call{Err: errors.New("clipboard monitor stopped")}
	}
	return c.Object(ShellName, ShellPath).CallWithContext(ctx, ShellInterface+"."+method, 0, args...)
}
func (w *Wayland) ReadText(ctx context.Context) (string, error) {
	var s string
	err := w.shellCall(ctx, "Read").Store(&s)
	return s, err
}
func (w *Wayland) WriteText(ctx context.Context, s string) error {
	if len(s) > clipboard.HardLimit || !utf8.ValidString(s) {
		return errors.New("invalid clipboard text")
	}
	w.mu.Lock()
	w.expectedHash = sha256.Sum256([]byte(s))
	w.expectedUntil = time.Now().Add(2 * time.Second)
	w.mu.Unlock()
	err := w.shellCall(ctx, "Write", s).Err
	if err != nil {
		w.mu.Lock()
		w.expectedHash = [32]byte{}
		w.mu.Unlock()
	}
	return err
}
func (w *Wayland) Status() clipboard.Status {
	w.mu.Lock()
	c := w.conn
	w.mu.Unlock()
	s := clipboard.Status{Supported: true, Backend: "GNOME Shell / session D-Bus", Detail: "GNOME has not loaded the clipboard bridge. Run scripts/install-extension.sh, then save work and choose Log Out from the Ubuntu system menu and log back in (locking the screen is not enough)"}
	if c != nil && c.Connected() {
		var owner bool
		if c.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, ShellName).Store(&owner) == nil && owner {
			s.Healthy = true
			s.Detail = "GNOME bridge connected"
		}
	}
	return s
}
func (w *Wayland) Stop() error {
	w.mu.Lock()
	if w.conn == nil {
		w.mu.Unlock()
		return nil
	}
	cancel, done := w.cancel, w.done
	w.mu.Unlock()
	cancel()
	<-done
	w.mu.Lock()
	w.conn = nil
	w.callback = nil
	w.mu.Unlock()
	return nil
}
