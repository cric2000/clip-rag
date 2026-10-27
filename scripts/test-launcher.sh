#!/bin/sh
set -eu
dbus-run-session --config-file=scripts/test-session.conf -- xvfb-run -a -s '-screen 0 1280x1024x24' env XDG_SESSION_TYPE=x11 GDK_BACKEND=x11 GTK_A11Y=none GIO_USE_VFS=local python3 scripts/check-launcher.py
