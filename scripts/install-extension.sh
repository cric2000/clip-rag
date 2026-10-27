#!/bin/sh
set -eu
# Explicit user setup command; application startup never installs an extension.
source_dir=$(CDPATH='' cd -- "$(dirname -- "$0")/../integrations/gnome-shell" && pwd)
destination="${XDG_DATA_HOME:-$HOME/.local/share}/gnome-shell/extensions/cliprag@cliprag.local"
install -d -m 0755 "$destination"
install -m 0644 "$source_dir/extension.js" "$source_dir/metadata.json" "$destination/"
printf '%s\n' "Installed ClipRAG bridge files at $destination"
# Queue only this UUID, preserving all other enabled/disabled extensions. The
# CLI cannot enable an undiscovered UUID, but this preference takes effect when
# GNOME next scans the extension directory at login.
if /usr/bin/python3 - <<'PY'
import gi
from gi.repository import Gio
settings = Gio.Settings.new('org.gnome.shell')
uuid = 'cliprag@cliprag.local'
enabled = settings.get_strv('enabled-extensions')
if uuid not in enabled and not settings.set_strv('enabled-extensions', enabled + [uuid]):
    raise SystemExit('Cannot queue ClipRAG enablement')
disabled = settings.get_strv('disabled-extensions')
if uuid in disabled and not settings.set_strv('disabled-extensions', [value for value in disabled if value != uuid]):
    raise SystemExit('Cannot remove ClipRAG disabled override')
Gio.Settings.sync()
PY
then
    printf '%s\n' 'ClipRAG is queued to enable automatically at your next GNOME login.'
else
    printf '%s\n' 'Could not queue activation. After a new GNOME login, run: gnome-extensions enable cliprag@cliprag.local' >&2
fi
if gnome-extensions info cliprag@cliprag.local >/dev/null 2>&1; then
    if gnome-extensions enable cliprag@cliprag.local; then
        gnome-extensions info cliprag@cliprag.local
    else
        printf '%s\n' 'GNOME discovered the bridge but could not enable it; inspect its extension error details.' >&2
        exit 1
    fi
else
    printf '%s\n' 'GNOME has not discovered these files in the current session.' \
        'Save your work, choose Log Out from the Ubuntu system menu, and log back in (or restart Ubuntu).' \
        'Locking the screen, closing a terminal, and relaunching ClipRAG do not restart GNOME.' \
        'After the new login, verify with: gnome-extensions info cliprag@cliprag.local'
fi
