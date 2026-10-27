//go:build linux

package linux

import (
	"cliprag/internal/clipboard"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xfixes"
	"github.com/jezek/xgb/xproto"
	"sync"
	"time"
	"unicode/utf8"
)

type property struct {
	kind xproto.Atom
	data []byte
	err  error
}
type outgoing struct {
	target  xproto.Atom
	data    []byte
	updated time.Time
}
type transferKey struct {
	window xproto.Window
	atom   xproto.Atom
}
type X11 struct {
	mu          sync.Mutex
	operations  sync.Mutex
	conn        *xgb.Conn
	win         xproto.Window
	atoms       map[string]xproto.Atom
	text        string
	pending     chan property
	incremental bool
	cancel      context.CancelFunc
	done        chan struct{}
	workers     sync.WaitGroup
	status      clipboard.Status
}

func NewX11() *X11 {
	return &X11{status: clipboard.Status{Supported: true, Backend: "X11 / XFixes", Detail: "Monitor not started"}}
}
func (x *X11) Start(ctx context.Context, cb func(clipboard.Event)) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.conn != nil {
		return errors.New("clipboard monitor already started")
	}
	c, err := xgb.NewConn()
	if err != nil {
		return err
	}
	fail := func(e error) error { c.Close(); return e }
	if err = xfixes.Init(c); err != nil {
		return fail(err)
	}
	if _, err = xfixes.QueryVersion(c, 5, 0).Reply(); err != nil {
		return fail(err)
	}
	win, err := xproto.NewWindowId(c)
	if err != nil {
		return fail(err)
	}
	root := xproto.Setup(c).DefaultScreen(c)
	if err = xproto.CreateWindowChecked(c, 0, win, root.Root, 0, 0, 1, 1, 0, xproto.WindowClassInputOnly, 0, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange}).Check(); err != nil {
		return fail(err)
	}
	atoms := map[string]xproto.Atom{}
	for _, name := range []string{"CLIPBOARD", "UTF8_STRING", "TARGETS", "TEXT", "STRING", "INCR", "CLIPRAG_DATA", "x-kde-passwordManagerHint", "application/x-keepassxc-sensitive"} {
		r, e := xproto.InternAtom(c, false, uint16(len(name)), name).Reply()
		if e != nil {
			return fail(e)
		}
		atoms[name] = r.Atom
	}
	if err = xfixes.SelectSelectionInputChecked(c, win, atoms["CLIPBOARD"], xfixes.SelectionEventMaskSetSelectionOwner|xfixes.SelectionEventMaskSelectionWindowDestroy|xfixes.SelectionEventMaskSelectionClientClose).Check(); err != nil {
		return fail(err)
	}
	child, cancel := context.WithCancel(ctx)
	x.conn, x.win, x.atoms, x.cancel, x.done = c, win, atoms, cancel, make(chan struct{})
	x.status.Healthy = true
	x.status.Detail = "CLIPBOARD ownership notifications active"
	changes := make(chan uint64, 32)
	x.workers.Add(1)
	go func() {
		defer x.workers.Done()
		for {
			select {
			case <-child.Done():
				return
			case seq := <-changes:
				readCtx, stop := context.WithTimeout(child, 2*time.Second)
				s, e := x.ReadText(readCtx)
				stop()
				if e == nil && cb != nil {
					cb(clipboard.Event{Text: s, Sequence: seq})
				}
			}
		}
	}()
	go func() { <-child.Done(); c.Close() }()
	go x.events(child, c, changes)
	return nil
}
func (x *X11) events(ctx context.Context, c *xgb.Conn, changes chan<- uint64) {
	defer close(x.done)
	defer x.cancel()
	outgoingTransfers := map[transferKey]outgoing{}
	var seq uint64
	for {
		ev, err := c.WaitForEvent()
		if err != nil || ev == nil {
			x.mu.Lock()
			x.status.Healthy = false
			x.status.Detail = "X11 connection closed"
			x.mu.Unlock()
			return
		}
		switch e := ev.(type) {
		case xfixes.SelectionNotifyEvent:
			if e.Owner != 0 && e.Owner != x.win {
				seq++
				select {
				case changes <- seq:
				case <-ctx.Done():
					return
				default:
				}
			}
		case xproto.SelectionNotifyEvent:
			if e.Requestor == x.win {
				if e.Property == 0 {
					x.deliver(property{err: errors.New("clipboard target unavailable")})
				} else {
					p := x.getProperty(c, x.win, e.Property)
					x.mu.Lock()
					x.incremental = p.kind == x.atoms["INCR"]
					x.mu.Unlock()
					x.deliver(p)
				}
			}
		case xproto.PropertyNotifyEvent:
			if e.Window == x.win && e.Atom == x.atoms["CLIPRAG_DATA"] && e.State == xproto.PropertyNewValue {
				x.mu.Lock()
				active := x.pending != nil && x.incremental
				x.mu.Unlock()
				if active {
					p := x.getProperty(c, x.win, e.Atom)
					if p.kind != x.atoms["INCR"] {
						x.deliver(p)
					}
				}
			}
			if e.State == xproto.PropertyDelete {
				key := transferKey{e.Window, e.Atom}
				t, ok := outgoingTransfers[key]
				if !ok {
					continue
				}
				n := len(t.data)
				if n > 32768 {
					n = 32768
				}
				xproto.ChangeProperty(c, xproto.PropModeReplace, e.Window, e.Atom, t.target, 8, uint32(n), t.data[:n])
				if n == 0 {
					delete(outgoingTransfers, key)
				} else {
					t.data = t.data[n:]
					t.updated = time.Now()
					outgoingTransfers[key] = t
				}
			}
		case xproto.SelectionRequestEvent:
			// Bound abandoned INCR transfers and discard requests older than 30 seconds.
			for k, t := range outgoingTransfers {
				if time.Since(t.updated) > 30*time.Second {
					delete(outgoingTransfers, k)
				}
			}
			prop := e.Property
			if prop == 0 {
				prop = e.Target
			}
			notify := xproto.SelectionNotifyEvent{Time: e.Time, Requestor: e.Requestor, Selection: e.Selection, Target: e.Target, Property: prop}
			x.mu.Lock()
			s := x.text
			x.mu.Unlock()
			switch e.Target {
			case x.atoms["TARGETS"]:
				b := make([]byte, 16)
				for i, a := range []xproto.Atom{x.atoms["TARGETS"], x.atoms["UTF8_STRING"], x.atoms["TEXT"], x.atoms["STRING"]} {
					binary.LittleEndian.PutUint32(b[i*4:], uint32(a))
				}
				xproto.ChangeProperty(c, xproto.PropModeReplace, e.Requestor, prop, xproto.AtomAtom, 32, 4, b)
			case x.atoms["UTF8_STRING"], x.atoms["TEXT"], x.atoms["STRING"]:
				if len(s) > 32768 && len(outgoingTransfers) < 16 {
					xproto.ChangeWindowAttributes(c, e.Requestor, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange})
					b := make([]byte, 4)
					binary.LittleEndian.PutUint32(b, uint32(len(s)))
					outgoingTransfers[transferKey{e.Requestor, prop}] = outgoing{e.Target, []byte(s), time.Now()}
					xproto.ChangeProperty(c, xproto.PropModeReplace, e.Requestor, prop, x.atoms["INCR"], 32, 1, b)
				} else if len(s) <= 32768 {
					xproto.ChangeProperty(c, xproto.PropModeReplace, e.Requestor, prop, e.Target, 8, uint32(len(s)), []byte(s))
				} else {
					notify.Property = 0
				}
			default:
				notify.Property = 0
			}
			xproto.SendEvent(c, false, e.Requestor, 0, string(notify.Bytes()))
		}
	}
}
func (x *X11) getProperty(c *xgb.Conn, w xproto.Window, a xproto.Atom) property {
	r, e := xproto.GetProperty(c, false, w, a, xproto.GetPropertyTypeAny, 0, clipboard.HardLimit/4+1).Reply()
	if e != nil {
		return property{err: e}
	}
	xproto.DeleteProperty(c, w, a)
	if r.BytesAfter > 0 || len(r.Value) > clipboard.HardLimit {
		return property{err: errors.New("clipboard exceeds size limit")}
	}
	return property{kind: r.Type, data: r.Value}
}
func (x *X11) deliver(p property) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.pending != nil {
		select {
		case x.pending <- p:
		default:
		}
	}
}
func (x *X11) ReadText(ctx context.Context) (string, error) {
	x.operations.Lock()
	defer x.operations.Unlock()
	x.mu.Lock()
	c := x.conn
	if c == nil {
		x.mu.Unlock()
		return "", errors.New("clipboard monitor stopped")
	}
	ch := make(chan property, 64)
	x.pending = ch
	win := x.win
	a := x.atoms
	x.mu.Unlock()
	defer func() { x.mu.Lock(); x.pending = nil; x.incremental = false; x.mu.Unlock() }()
	// SelectionNotify is authoritative. PropertyNotify for ordinary transfers can
	// arrive first, so the event loop only handles property chunks during INCR.
	request := func(target xproto.Atom) (property, error) {
		for len(ch) > 0 {
			<-ch
		}
		xproto.ConvertSelection(c, win, a["CLIPBOARD"], target, a["CLIPRAG_DATA"], xproto.TimeCurrentTime)
		select {
		case p := <-ch:
			return p, p.err
		case <-ctx.Done():
			return property{}, ctx.Err()
		}
	}
	targets, err := request(a["TARGETS"])
	if err != nil {
		return "", err
	}
	target := a["UTF8_STRING"]
	for i := 0; i+4 <= len(targets.data); i += 4 {
		v := xproto.Atom(binary.LittleEndian.Uint32(targets.data[i:]))
		if v == a["x-kde-passwordManagerHint"] || v == a["application/x-keepassxc-sensitive"] {
			return "", errors.New("clipboard marked sensitive")
		}
	}
	p, err := request(target)
	if err != nil {
		return "", err
	}
	b := p.data
	if p.kind == a["INCR"] {
		b = nil
		for {
			select {
			case next := <-ch:
				if next.err != nil {
					return "", next.err
				}
				if len(next.data) == 0 {
					if !utf8.Valid(b) {
						return "", errors.New("invalid UTF-8")
					}
					return string(b), nil
				}
				b = append(b, next.data...)
				if len(b) > clipboard.HardLimit {
					return "", errors.New("clipboard exceeds size limit")
				}
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	if !utf8.Valid(b) {
		return "", errors.New("invalid UTF-8")
	}
	return string(b), nil
}
func (x *X11) WriteText(ctx context.Context, s string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(s) > clipboard.HardLimit || !utf8.ValidString(s) {
		return errors.New("invalid clipboard text")
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.conn == nil {
		return errors.New("clipboard monitor stopped")
	}
	x.text = s
	if err := xproto.SetSelectionOwnerChecked(x.conn, x.win, x.atoms["CLIPBOARD"], xproto.TimeCurrentTime).Check(); err != nil {
		return err
	}
	r, e := xproto.GetSelectionOwner(x.conn, x.atoms["CLIPBOARD"]).Reply()
	if e != nil {
		return e
	}
	if r.Owner != x.win {
		return fmt.Errorf("could not acquire CLIPBOARD")
	}
	return nil
}
func (x *X11) Status() clipboard.Status { x.mu.Lock(); defer x.mu.Unlock(); return x.status }
func (x *X11) Stop() error {
	x.mu.Lock()
	if x.conn == nil {
		x.mu.Unlock()
		return nil
	}
	cancel, done := x.cancel, x.done
	x.mu.Unlock()
	cancel()
	<-done
	x.workers.Wait()
	x.mu.Lock()
	x.conn = nil
	x.mu.Unlock()
	return nil
}
