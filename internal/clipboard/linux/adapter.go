//go:build linux

package linux

import (
	"cliprag/internal/clipboard"
	"fmt"
	"os"
	"strings"
)

// New refuses XWayland as a substitute for native Wayland monitoring.
func New() (clipboard.Adapter, error) {
	switch os.Getenv("XDG_SESSION_TYPE") {
	case "wayland":
		if strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "gnome") {
			return NewWayland(), nil
		}
		return nil, fmt.Errorf("Wayland monitoring requires the ClipRAG GNOME Shell extension; this desktop is unsupported")
	case "x11":
		return NewX11(), nil
	default:
		return nil, fmt.Errorf("unsupported or missing XDG_SESSION_TYPE (expected wayland/GNOME or x11)")
	}
}
