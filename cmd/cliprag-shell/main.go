//go:build linux

package main

import (
	"cliprag/internal/shellcomplete"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	binary, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cliprag-shell: cannot locate executable")
		os.Exit(1)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		os.Exit(1)
	}
	os.Exit(shellcomplete.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, binary))
}
