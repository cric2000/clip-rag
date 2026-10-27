//go:build linux

package linux

import (
	"cliprag/internal/clipboard"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestX11Integration(t *testing.T) {
	if os.Getenv("CLIPRAG_X11_TEST") != "1" {
		t.Skip("requires isolated Xvfb display and xclip")
	}
	x := NewX11()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan clipboard.Event, 20)
	if e := x.Start(ctx, func(v clipboard.Event) { events <- v }); e != nil {
		t.Fatal(e)
	}
	defer x.Stop()
	if x.Start(ctx, nil) == nil {
		t.Fatal("double start")
	}
	for _, text := range []string{"  git status\n\twith spaces  ", "  git status\n\twith spaces  ", strings.Repeat("multiline text\n", 10000)} {
		ownerCtx, kill := context.WithCancel(ctx)
		owner := exec.CommandContext(ownerCtx, "xclip", "-quiet", "-selection", "clipboard", "-in")
		owner.Stdin = strings.NewReader(text)
		if e := owner.Start(); e != nil {
			kill()
			t.Fatal(e)
		}
		select {
		case v := <-events:
			if v.Text != text {
				kill()
				owner.Wait()
				t.Fatalf("capture mismatch got %d want %d bytes", len(v.Text), len(text))
			}
		case <-time.After(4 * time.Second):
			kill()
			owner.Wait()
			t.Fatal("missing XFixes event")
		}
		readCtx, stop := context.WithTimeout(ctx, 3*time.Second)
		got, e := x.ReadText(readCtx)
		stop()
		if e != nil || got != text {
			kill()
			owner.Wait()
			t.Fatalf("read mismatch: %d %v", len(got), e)
		}
		if e = x.WriteText(ctx, text); e != nil {
			kill()
			owner.Wait()
			t.Fatal(e)
		}
		outCtx, done := context.WithTimeout(ctx, 3*time.Second)
		gotBytes, e := exec.CommandContext(outCtx, "xclip", "-selection", "clipboard", "-out").Output()
		done()
		kill()
		owner.Wait()
		if e != nil || string(gotBytes) != text {
			t.Fatalf("restore mismatch: %d %v", len(gotBytes), e)
		}
		select {
		case <-events:
			t.Fatal("own restore captured as external")
		case <-time.After(30 * time.Millisecond):
		}
	}
	cancel()
	if e := x.Stop(); e != nil {
		t.Fatal(e)
	}
	if x.Status().Healthy {
		t.Fatal("healthy after stop")
	}
	if e := x.Stop(); e != nil {
		t.Fatal(e)
	}
}
func TestX11Unavailable(t *testing.T) {
	t.Setenv("DISPLAY", ":9999")
	x := NewX11()
	if x.Start(context.Background(), nil) == nil {
		x.Stop()
		t.Fatal("unsupported server accepted")
	}
}
