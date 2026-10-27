#!/bin/sh
set -eu
# Fresh GNOME, private D-Bus, temporary settings, synthetic clipboard only.
dbus-run-session --config-file=scripts/test-session.conf -- /usr/bin/python3 scripts/check-gnome.py
