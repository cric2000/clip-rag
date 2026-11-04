//go:build linux

package main

import (
	browserassets "cliprag/integrations/browser"
	"cliprag/internal/browsercomplete"
	platform "cliprag/internal/platform/linux"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cliprag-browser:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) > 0 && (args[0] == browsercomplete.Origin() || args[0] == "serve") {
		database, err := browsercomplete.DatabasePath()
		if err != nil {
			return err
		}
		return browsercomplete.Serve(os.Stdin, os.Stdout, database)
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Println(`ClipRAG browser autocomplete (Chrome / Chromium / Brave / Edge)

  cliprag-browser install chrome|chromium|brave|edge
  cliprag-browser uninstall chrome|chromium|brave|edge

Load the bundled unpacked extension once from your browser's Extensions page.
Click ClipRAG's toolbar icon to enable automatic suggestions on a website.
The address bar supports saved URLs after typing "clip" and Space.
All matching stays on this device. No HTTP server or cloud account is used.`)
		return nil
	}
	if len(args) < 2 || (args[0] != "install" && args[0] != "uninstall") {
		return errors.New("choose install or uninstall and a browser; see --help")
	}
	directory, ok := browsercomplete.BrowserDirectories[args[1]]
	if !ok {
		return errors.New("choose chrome, chromium, brave, or edge")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	custom := flags.String("manifest-dir", "", "absolute native-host directory for a custom browser profile")
	data := flags.String("data-dir", "", "absolute ClipRAG data directory, if customized")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	if *custom == "" {
		*custom = filepath.Join(root, directory, "NativeMessagingHosts")
	}
	if *data == "" {
		*data, err = platform.DataDir()
		if err != nil {
			return err
		}
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	if err = browsercomplete.Install(*custom, binary, *data, root, args[0] == "uninstall"); err != nil {
		return err
	}
	if args[0] == "uninstall" {
		fmt.Println("Removed ClipRAG's native browser bridge.")
		return nil
	}
	folder := filepath.Join(filepath.Dir(binary), "cliprag-browser-extension")
	if _, err = os.Stat(folder); err != nil {
		folder = "/usr/share/cliprag/browser-extension"
	}
	fmt.Printf("Native bridge ready for %s.\nExtension ID: %s\nLoad unpacked extension from:\n%s\n", args[1], browserassets.ExtensionID(), folder)
	return nil
}
