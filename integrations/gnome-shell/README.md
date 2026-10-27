# ClipRAG GNOME Shell bridge

Original integration targeting GNOME Shell **50**. The installed Mutter 18 typelib
was inspected for `Meta.Selection.transfer_async`, `get_mimetypes`, and the
CLIPBOARD enum. A fresh, isolated headless GNOME 50.1 test verifies loading and capture from a native Shell selection fixture into Go/SQLite. Capture has also been confirmed in the active GNOME desktop; provider-specific behavior can still vary.

From the repository, run `./scripts/install-extension.sh`. It queues this UUID for automatic activation without replacing other enabled extensions. Save work, choose Log Out in Ubuntu and log back in (or restart), then verify with `gnome-extensions info cliprag@cliprag.local`. Locking the screen does not restart the session. A `.deb` installation places
the same extension in the system extensions directory; enable it after login.
Do not install both copies unless intentionally overriding the packaged version.

The extension listens to `Meta.Selection::owner-changed`, requests only text with
a 1 MB hard limit, checks sensitive MIME hints, and forwards events to
`io.github.cliprag.ClipRAG` at `/io/github/cliprag/ClipRAG` on the **session bus**.
There is no polling, categorization, history, networking, automatic paste, or
command execution in the extension. Newer ownership changes cancel stale reads.
Very rapid changes may therefore be skipped rather than attributed incorrectly.

`io.github.cliprag.Shell` exports Read/Write only to the unique connection owning
the ClipRAG service. Go accepts Capture only from the owner of `org.gnome.Shell`,
validates UTF-8, size and event sequence, and exposes no history on D-Bus. Processes
running as the same user remain inside the desktop trust boundary; this is not
protection against a compromised account.

The Go adapter identifies one matching restore echo within two seconds. A real
external re-copy of identical text in that interval can be suppressed if the echo
was missing. Repeated same-content copies are counted only when Shell emits a new
ownership notification. Clipboard providers that change content without changing
ownership may not be observable. Capture from every application is not guaranteed.

Disable with `gnome-extensions disable cliprag@cliprag.local`. For a user install,
remove `$XDG_DATA_HOME/gnome-shell/extensions/cliprag@cliprag.local` (default
`~/.local/share/gnome-shell/extensions/cliprag@cliprag.local`) after disabling.
See the [root README](../../README.md#storage-and-privacy) for storage and privacy details.
