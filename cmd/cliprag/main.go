//go:build linux

package main

import (
	"cliprag/frontend"
	"cliprag/internal/app"
	linuxclip "cliprag/internal/clipboard/linux"
	platform "cliprag/internal/platform/linux"
	"cliprag/internal/storage"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"github.com/godbus/dbus/v5"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"io/fs"
	"log/slog"
	"os"
	"slices"
	"sync"
)

//go:embed icon.png
var icon []byte

type Desktop struct {
	window *application.WebviewWindow
	ui     *application.App
}

func (d *Desktop) Hide() { d.window.Hide() }
func (d *Desktop) Quit() { d.ui.Quit() }
func (d *Desktop) show() { d.window.Show(); d.window.Restore(); d.window.Focus() }
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ClipRAG:", err)
		os.Exit(1)
	}
}
func run() error {
	hidden := flag.Bool("hidden", false, "start with window hidden")
	pause := flag.Bool("pause", false, "pause capture (also works with a running instance)")
	resume := flag.Bool("resume", false, "resume capture")
	quit := flag.Bool("quit", false, "quit the running instance")
	flag.Bool("show", false, "show the running instance (default)")
	flag.Parse()
	if *quit {
		c, e := dbus.ConnectSessionBus()
		if e != nil {
			return e
		}
		var running bool
		e = c.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "io.github.cliprag.SingleInstance").Store(&running)
		c.Close()
		if e != nil {
			return e
		}
		if !running {
			return nil
		}
	}
	adapter, err := linuxclip.New()
	if err != nil {
		return err
	}
	dir, err := platform.DataDir()
	if err != nil {
		return err
	}
	store, err := storage.Open(dir + "/history.db")
	if err != nil {
		return err
	}
	var ui *application.App
	engine, err := app.New(store, adapter, dir, func() {
		if ui != nil {
			ui.Event.Emit("history:changed")
		}
	})
	if err != nil {
		store.Close()
		return err
	}
	svc := app.NewService(engine)
	desktop := &Desktop{}
	assets, err := fs.Sub(frontend.Assets, "dist")
	if err != nil {
		store.Close()
		return err
	}
	var once sync.Once
	cleanup := func() { once.Do(func() { engine.Stop(); store.Close() }) }
	defer cleanup()
	ui = application.New(application.Options{
		Linux: application.LinuxOptions{ApplicationID: "io.github.cliprag", ProgramName: "cliprag"},
		Name:  "ClipRAG", Description: "Your developer clipboard memory.", Icon: icon, LogLevel: slog.LevelError,
		Assets:     application.AssetOptions{Handler: application.BundledAssetFileServer(assets)},
		Services:   []application.Service{application.NewService(svc), application.NewService(desktop)},
		OnShutdown: cleanup,
		SingleInstance: &application.SingleInstanceOptions{UniqueID: "io.github.cliprag", OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
			switch {
			case slices.Contains(data.Args, "--quit"):
				ui.Quit()
			case slices.Contains(data.Args, "--pause"):
				cfg := svc.GetSettings()
				cfg.Monitoring = false
				if e := svc.SaveSettings(cfg); e != nil {
					slog.Error("Could not pause capture")
				}
			case slices.Contains(data.Args, "--resume"):
				cfg := svc.GetSettings()
				cfg.Monitoring = true
				if e := svc.SaveSettings(cfg); e != nil {
					slog.Error("Could not resume capture")
				}
			default:
				if desktop.window != nil {
					desktop.show()
				}
			}
		}},
	})
	desktop.ui = ui
	desktop.window = ui.Window.NewWithOptions(application.WebviewWindowOptions{Name: "ClipRAG", Title: "ClipRAG — Your developer clipboard memory", Width: 1200, Height: 820, MinWidth: 540, MinHeight: 500, URL: "/", Hidden: *hidden, BackgroundColour: application.NewRGB(16, 17, 21)})
	desktop.window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) { event.Cancel(); desktop.window.Hide() })
	if *pause || *resume {
		cfg := svc.GetSettings()
		cfg.Monitoring = !*pause
		if err = svc.SaveSettings(cfg); err != nil {
			return err
		}
	}
	// A failed monitor still permits inspecting existing local history and shows a
	// health warning in the UI. No insecure monitoring fallback is attempted.
	if err = engine.Start(context.Background()); err != nil {
		slog.Error("Clipboard monitor could not start; see application status")
	}
	tray := ui.SystemTray.New()
	tray.SetIcon(icon)
	tray.SetTooltip("ClipRAG — clipboard memory")
	menu := ui.NewMenu()
	menu.Add("Open ClipRAG").OnClick(func(*application.Context) { desktop.show() })
	menu.Add("Pause / resume monitoring").OnClick(func(*application.Context) {
		cfg := svc.GetSettings()
		cfg.Monitoring = !cfg.Monitoring
		if e := svc.SaveSettings(cfg); e != nil {
			slog.Error("Could not update monitoring")
		}
	})
	menu.AddSeparator()
	menu.Add("Quit ClipRAG").OnClick(func(*application.Context) { ui.Quit() })
	tray.SetMenu(menu)
	return ui.Run()
}
