#!/bin/sh
set -eu
# These tests use generated fixture text and an isolated bus/display, never the
# real user's clipboard. xvfb-run cleans up the X server on exit.
CLIPRAG_DBUS_TEST=1 dbus-run-session -- go test -count=1 -race ./internal/clipboard/linux -run 'TestWayland'
CLIPRAG_X11_TEST=1 xvfb-run -a go test -count=1 -race ./internal/clipboard/linux -run 'TestX11'
