//go:build linux && desktopintegration

// desktop-check exercises the compiled React UI in the real WebKitGTK engine,
// backed by the real history service, SQLite and XFixes adapter on an isolated
// Xvfb display. It never touches the user's desktop clipboard.
package main

import (
	"cliprag/frontend"
	"cliprag/internal/app"
	linuxclip "cliprag/internal/clipboard/linux"
	"cliprag/internal/history"
	"cliprag/internal/storage"
	"context"
	"errors"
	"fmt"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"image"
	"image/png"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL native desktop:", err)
		os.Exit(1)
	}
	fmt.Println("PASS native WebKitGTK UI / XFixes / SQLite workflow")
}
func run() error {
	dir, err := os.MkdirTemp("", "cliprag-desktop-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	store, err := storage.Open(filepath.Join(dir, "cliprag", "history.db"))
	if err != nil {
		return err
	}
	var ui *application.App
	engine, err := app.New(store, linuxclip.NewX11(), dir, func() {
		if ui != nil {
			ui.Event.Emit("history:changed")
		}
	})
	if err != nil {
		return err
	}
	svc := app.NewService(engine)
	assets, err := fs.Sub(frontend.Assets, "dist")
	if err != nil {
		return err
	}
	reports := make(chan string, 20)
	ready := make(chan struct{}, 1)
	ui = application.New(application.Options{Name: "ClipRAG test", LogLevel: slog.LevelError, Assets: application.AssetOptions{Handler: application.BundledAssetFileServer(assets)}, Services: []application.Service{application.NewService(svc)}, RawMessageHandler: func(_ application.Window, s string, _ *application.OriginInfo) {
		if strings.HasPrefix(s, "cliprag-check:") {
			reports <- strings.TrimPrefix(s, "cliprag-check:")
		}
	}})
	win := ui.Window.NewWithOptions(application.WebviewWindowOptions{Title: "ClipRAG — integration check", Width: 1120, Height: 840, URL: "/"})
	win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		select {
		case ready <- struct{}{}:
		default:
		}
	})
	if err = engine.Start(context.Background()); err != nil {
		return err
	}
	outcome := make(chan error, 1)
	go func() {
		defer ui.Quit()
		outcome <- exercise(win, svc, reports, ready)
	}()
	if err = ui.Run(); err != nil {
		return err
	}
	engine.Stop()
	result := <-outcome
	// Verify the same database reopens with the exact saved contents and pins.
	store.Close()
	reopened, e := storage.Open(filepath.Join(dir, "cliprag", "history.db"))
	if e != nil {
		return e
	}
	defer reopened.Close()
	p, e := reopened.List(context.Background(), history.Query{Pinned: true})
	if e != nil {
		return e
	}
	if result == nil && (p.Total != 1 || p.Entries[0].Content != "git status") {
		return errors.New("restart persistence/pin verification failed")
	}
	return result
}
func exercise(win *application.WebviewWindow, svc *app.Service, reports <-chan string, ready <-chan struct{}) error {
	select {
	case <-ready:
	case <-time.After(15 * time.Second):
		return errors.New("WebKit runtime did not become ready")
	}
	js := func(body string) error {
		win.ExecJS(`(async()=>{const wait=async(fn)=>{const until=Date.now()+6000;while(!fn()){if(Date.now()>until)throw Error('DOM condition timed out');await new Promise(r=>setTimeout(r,30));}};const button=(text)=>Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim().startsWith(text));const type=(value)=>{document.querySelector('[data-category="'+value+'"]').click();};` + body + `;window._wails.invoke('cliprag-check:ok');})().catch(e=>window._wails.invoke('cliprag-check:'+String(e)));`)
		select {
		case s := <-reports:
			if s != "ok" {
				return errors.New(s)
			}
			return nil
		case <-time.After(9 * time.Second):
			return errors.New("UI script response timed out")
		}
	}
	if err := js(`await wait(()=>document.querySelector('.empty-state'));if(document.querySelector('[role=alert]'))throw Error('UI backend initialization failed')`); err != nil {
		return err
	}
	if err := js(`await Promise.all([document.fonts.load('15px Inter'),document.fonts.load('14px "JetBrains Mono"')]);await document.fonts.ready;if(!document.fonts.check('15px Inter')||!document.fonts.check('14px "JetBrains Mono"'))throw Error('bundled fonts failed to load');if(document.querySelector('aside'))throw Error('duplicate sidebar still present');if(document.querySelectorAll('[role=tab]').length!==2)throw Error('expected two views')`); err != nil {
		return err
	}
	fmt.Println("PASS WebKit renders React, loads bundled fonts and calls real Go bindings")
	time.Sleep(150 * time.Millisecond)
	if err := screenshot("bin/cliprag-empty.png"); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var owners []*exec.Cmd
	defer func() {
		for _, c := range owners {
			if c.Process != nil {
				c.Process.Kill()
				c.Wait()
			}
		}
	}()
	copyExternal := func(text string) error {
		owner := exec.CommandContext(ctx, "xclip", "-quiet", "-selection", "clipboard", "-in")
		owner.Stdin = strings.NewReader(text)
		if e := owner.Start(); e != nil {
			return e
		}
		owners = append(owners, owner)
		return nil
	}
	waitCount := func(n int) error {
		until := time.Now().Add(4 * time.Second)
		for {
			p, e := svc.List(history.Query{})
			if e != nil {
				return e
			}
			if p.Total == n {
				return nil
			}
			if time.Now().After(until) {
				return fmt.Errorf("expected %d saved entries, got %d", n, p.Total)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	sqlText := "SELECT id, name\nFROM users\nWHERE active = true;\n"
	for i, text := range []string{"git status", "docker ps", sqlText, "https://github.com/example/repo", "a plain text fixture"} {
		if e := copyExternal(text); e != nil {
			return e
		}
		if e := waitCount(i + 1); e != nil {
			return e
		}
	}
	if err := js(`await wait(()=>document.querySelectorAll('article').length===5);type('Git');await wait(()=>document.querySelectorAll('article').length===1&&document.querySelector('article pre').textContent==='git status');document.querySelector('[aria-label="Pin entry"]').click();await wait(()=>document.querySelector('[aria-label="Unpin entry"]'));button('Pinned').click();await wait(()=>document.querySelector('[aria-selected="true"]').id==='pinned-clips-tab');type('SQL');await wait(()=>document.querySelector('.empty-state'));button('All clips').click();await wait(()=>document.querySelectorAll('article').length===1&&document.querySelector('article pre').textContent.startsWith('SELECT'));type('');await wait(()=>document.querySelectorAll('article').length===5)`); err != nil {
		return fmt.Errorf("category/pinning: %w", err)
	}
	fmt.Println("PASS category counts, filters, pinning, frontend update events")
	accepted := exec.CommandContext(ctx, "./bin/cliprag-shell", "record-use")
	accepted.Env = append(os.Environ(), "XDG_DATA_HOME="+svc.Status().DataDir)
	accepted.Stdin = strings.NewReader("git status")
	if output, err := accepted.CombinedOutput(); err != nil {
		return fmt.Errorf("autocomplete acceptance: %s: %w", output, err)
	}
	if err := js(`await wait(()=>Array.from(document.querySelectorAll('article')).some(a=>a.querySelector('pre').textContent==='git status'&&a.textContent.includes('Autocomplete uses 1')))`); err != nil {
		return fmt.Errorf("live autocomplete count: %w", err)
	}
	fmt.Println("PASS accepted autocomplete usage updates the open UI through an external helper")
	if err := js(`const input=document.querySelector('[aria-label="Search clipboard history"]');Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(input,'docker');input.dispatchEvent(new Event('input',{bubbles:true}));await wait(()=>document.querySelectorAll('article').length===1&&document.querySelector('article pre').textContent==='docker ps');Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(input,'');input.dispatchEvent(new Event('input',{bubbles:true}));await wait(()=>document.querySelectorAll('article').length===5);type('SQL');await wait(()=>document.querySelectorAll('article').length===1);document.querySelector('[aria-label="Copy SQL entry"]').click();await wait(()=>document.querySelector('.toast')?.textContent==='Copied to clipboard')`); err != nil {
		return fmt.Errorf("search/restore: %w", err)
	}
	out, e := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-out").Output()
	if e != nil || string(out) != sqlText {
		return fmt.Errorf("external paste target read wrong restored bytes: %v", e)
	}
	p, e := svc.List(history.Query{Category: "SQL"})
	if e != nil || p.Total != 1 || p.Entries[0].CopyCount != 1 || p.Entries[0].RestoreCount != 1 {
		return errors.New("restore counters/echo mismatch")
	}
	fmt.Println("PASS search, SQL restore and external xclip read with exact whitespace")
	win.Hide()
	if e = copyExternal("kubectl get pods"); e != nil {
		return e
	}
	if e = waitCount(6); e != nil {
		return e
	}
	win.Show()
	if e = js(`type('');await wait(()=>document.querySelectorAll('article').length===6);button('Pause').click();await wait(()=>button('Resume'))`); e != nil {
		return e
	}
	if e = copyExternal("paused text must not persist"); e != nil {
		return e
	}
	time.Sleep(150 * time.Millisecond)
	if e = waitCount(6); e != nil {
		return e
	}
	if e = js(`button('Resume').click();await wait(()=>button('Pause'))`); e != nil {
		return e
	}
	if e = copyExternal("journalctl -u nginx"); e != nil {
		return e
	}
	if e = waitCount(7); e != nil {
		return e
	}
	if e = js(`await wait(()=>document.querySelectorAll('article').length===7);document.querySelector('[aria-label="Delete entry"]').click();await wait(()=>document.querySelectorAll('article').length===6)`); e != nil {
		return e
	}
	fmt.Println("PASS capture with hidden window, pause/resume, deletion")
	if e = js(`await wait(()=>!document.querySelector('.toast'))`); e != nil {
		return e
	}
	time.Sleep(100 * time.Millisecond)
	if e = screenshot("bin/cliprag-x11.png"); e != nil {
		return e
	}
	win.SetSize(640, 780)
	if e = js(`await wait(()=>window.innerWidth<700);if(document.documentElement.scrollWidth>window.innerWidth)throw Error('horizontal overflow on compact window');if(parseFloat(getComputedStyle(document.querySelector('pre')).fontSize)<14)throw Error('compact preview text too small')`); e != nil {
		return e
	}
	time.Sleep(150 * time.Millisecond)
	if e = screenshot("bin/cliprag-compact.png"); e != nil {
		return e
	}
	win.SetSize(540, 740)
	if e = js(`await wait(()=>window.innerWidth===540);if(document.documentElement.scrollWidth>window.innerWidth)throw Error('horizontal overflow at minimum window width');if(parseFloat(getComputedStyle(document.querySelector('pre')).fontSize)<14)throw Error('minimum-window preview text too small')`); e != nil {
		return e
	}
	win.SetSize(1120, 840)
	if e = js(`await wait(()=>window.innerWidth>1000);document.querySelector('[aria-label="Settings"]').click();await wait(()=>document.querySelector('dialog[open]'))`); e != nil {
		return e
	}
	time.Sleep(150 * time.Millisecond)
	if e = screenshot("bin/cliprag-settings.png"); e != nil {
		return e
	}
	if e = js(`button('Clear history').click();await wait(()=>button('Delete all history'));if(document.querySelectorAll('article').length!==6)throw Error('cleared before confirmation');button('Delete all history').click();await wait(()=>document.querySelectorAll('article').length===0);document.querySelector('[aria-label="Close settings"]').click()`); e != nil {
		return fmt.Errorf("confirmed clear: %w", e)
	}
	if e = copyExternal("git status"); e != nil {
		return e
	}
	if e = waitCount(1); e != nil {
		return e
	}
	if e = js(`await wait(()=>document.querySelector('[aria-label="Pin entry"]'));document.querySelector('[aria-label="Pin entry"]').click();await wait(()=>document.querySelector('[aria-label="Unpin entry"]'))`); e != nil {
		return e
	}
	fmt.Println("PASS native settings dialog and confirmed clear history")
	return nil
}
func screenshot(path string) error {
	c, e := xgb.NewConn()
	if e != nil {
		return e
	}
	defer c.Close()
	screen := xproto.Setup(c).DefaultScreen(c)
	drawable := xproto.Drawable(screen.Root)
	width, height := screen.WidthInPixels, screen.HeightInPixels
	tree, err := xproto.QueryTree(c, screen.Root).Reply()
	if err == nil {
		largestArea := 0
		for _, child := range tree.Children {
			g, err := xproto.GetGeometry(c, xproto.Drawable(child)).Reply()
			if err == nil && int(g.Width)*int(g.Height) > largestArea {
				largestArea = int(g.Width) * int(g.Height)
				drawable = xproto.Drawable(child)
				width, height = g.Width, g.Height
			}
		}
	}
	r, e := xproto.GetImage(c, xproto.ImageFormatZPixmap, drawable, 0, 0, width, height, 0xffffffff).Reply()
	if e != nil {
		return e
	}
	w, h := int(width), int(height)
	if len(r.Data) != w*h*4 {
		return errors.New("unsupported screenshot pixel format")
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(r.Data); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = r.Data[i+2], r.Data[i+1], r.Data[i], 255
	}
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return png.Encode(f, img)
}
