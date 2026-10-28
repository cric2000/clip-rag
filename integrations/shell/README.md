# Terminal completion

ClipRAG v0.2 includes `cliprag-shell`, an optional helper for interactive Bash 4.4+
and Zsh 5.8+. It reads the same local SQLite history as the desktop app and does
not require the app to be running. No network service, root access, global typing
monitor or command-history import is involved.

## Enable

For an installed package:

```bash
cliprag-shell install bash
# Or: cliprag-shell install zsh
```

For a source build, use `./bin/cliprag-shell` instead. Open a new terminal, or
activate immediately in the current shell:

```bash
eval "$(cliprag-shell init bash)"
# Or: eval "$(cliprag-shell init zsh)"
```

Use `./bin/cliprag-shell` in that command for a source build too.

Installation adds one marked block to `~/.bashrc` or `${ZDOTDIR:-$HOME}/.zshrc`.
An existing file is backed up as `.bashrc.cliprag-backup` or `.zshrc.cliprag-backup`.
Repeated installation updates the block without duplicating it. For symlinked
startup files, pass `--rc-file /absolute/path/to/the/actual/file` after the shell
name, or add the `eval` line manually. Your existing Tab completion stays intact.

## Use

1. Save commands by copying them while ClipRAG is monitoring.
2. Type at least three characters at your Bash/Zsh prompt, with the cursor at the
   end of the line: for example, `git ch`.
3. The best saved match appears automatically as **faded text**. It updates as
   you type or delete characters.
4. Press **Right Arrow** to accept the full command, or **Esc** to dismiss it.
   **Enter runs only your typed or accepted text**, never the faded suffix.
5. For more matches, press **Alt+/**. Use **Up/Down**, **Tab**, or **Ctrl+P/Ctrl+N**
   to select; **Enter inserts** the command. **Esc** or **Ctrl+C** cancels. If Alt
   is intercepted, press **Esc**, then **/** quickly. Review before pressing
   Enter again to execute.

The optional picker uses the terminal’s alternate screen, then restores the prompt.
This integration applies only at the shell prompt, not inside editors, browsers,
password prompts or other programs running in the terminal. Both Emacs-style
and vi insert-mode keymaps are supported; vi command mode is left alone.

**Warp’s native input editor is unsupported.** It replaces the shell input editor
that these hooks use. See [Warp’s compatibility notes](https://docs.warp.dev/support-and-community/troubleshooting-and-support/known-issues#list-of-incompatible-tools).

Bash uses a bundled, pinned [ble.sh](https://github.com/akinomyoga/ble.sh) line
editor, extracted once to `$XDG_CACHE_HOME/cliprag/shell` (normally
`~/.cache/cliprag/shell`). No download is needed. An existing ble.sh session is
reused. Zsh uses its native line editor. Normal Tab completion remains available.

Exact, case-sensitive prefixes are ranked by pinned status, external copy count,
then recency. Matches include single-line Git/Linux/Docker/Kubernetes commands
and common developer commands such as `go`, `npm`, `python3` and `cargo`. Generic
prose, SQL, JSON, URLs, multiline text, terminal control sequences, overlong clips
and detected secrets are excluded. Commands are inserted literally—there is no
`eval`, execution, clipboard modification, or reuse-count increment on selection.
Accepting a full suggestion with Right Arrow or selecting one in the picker adds
one **Autocomplete use** to that saved clip. Viewing, dismissing, typing the command
yourself and ordinary Tab completion do not count. The desktop updates automatically;
counts also persist while it is closed. This records acceptance, not execution.
Only this local counter is written; no prefix log or remote telemetry is collected.
Unicode command text is preserved, with escapes in the picker for predictable
terminal rendering. Input prefixes are passed through a pipe, not process
arguments, and are not logged or saved.

## Pause or remove

```bash
CLIPRAG_COMPLETION_ENABLED=0    # pause in this shell
CLIPRAG_COMPLETION_ENABLED=1    # resume
cliprag_completion_disable     # remove hooks and restore bindings in this shell
cliprag-shell uninstall bash   # remove startup block; use zsh for Zsh
```

Open a new terminal after uninstalling. The uninstaller preserves unrelated
startup configuration and your ClipRAG database.

## Troubleshooting and checks

- **No matches:** copy a supported single-line command into ClipRAG first. Type
  three or more characters; ensure Settings and your shell use the same
  `XDG_DATA_HOME`. Monitoring pauses new capture, not access to saved suggestions.
- **Missing helper:** install v0.2.0+ or build `./cmd/cliprag-shell`; v0.1.0 does
  not include terminal completion.
- **Warp:** use a conventional terminal with a supported Bash/Zsh editor. Reloading
  the startup file does not add support to Warp’s native input box.
- **Shortcut does nothing:** activate the integration in the current shell,
  place the cursor at the end of a single line, and check that the terminal is
  not intercepting Alt+/. Dumb/non-interactive terminals are unsupported.
- **History busy/unavailable:** retry after the desktop operation finishes.
  Suggestion lookups use a read-only connection. Acceptance writes only the local
  counter, with a short lock timeout; an unavailable counter never blocks insertion.
  The helper never creates or migrates the database. Open the updated desktop app
  once to enable counters on an older history file. Counts start at zero with this update.

Run `./scripts/test-shell.sh` with Bash and Zsh installed for private PTY checks.
The checks cover automatic display, acceptance, Enter isolation, live edits,
literal insertion, Unicode, ranking, cancellation, normal Tab, mid-line editing,
pausing and restoring bindings, using isolated test history.
