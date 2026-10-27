//go:build linux

// clipboard-probe prints metadata only. Use -restore FILE to write exact bytes;
// no plaintext clipboard content is ever printed.
package main

import (
	"cliprag/internal/clipboard"
	linuxclip "cliprag/internal/clipboard/linux"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	file := flag.String("restore", "", "restore exact UTF-8 text from a file")
	flag.Parse()
	a, e := linuxclip.New()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e = a.Start(ctx, func(v clipboard.Event) {
		fmt.Printf("event sequence=%d bytes=%d sensitive=%v self=%v\n", v.Sequence, len(v.Text), v.Sensitive, v.Self)
	}); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	defer a.Stop()
	fmt.Printf("status: %+v\n", a.Status())
	timed, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err == nil {
			err = a.WriteText(timed, string(b))
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	}
	text, err := a.ReadText(timed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
	} else {
		fmt.Printf("read bytes=%d\n", len(text))
	}
	<-ctx.Done()
}
