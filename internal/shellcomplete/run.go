// Package shellcomplete provides terminal-only history lookup and shell setup.
// It has no desktop, clipboard, network, or command-execution dependency.
package shellcomplete

import (
	shellassets "cliprag/integrations/shell"
	platform "cliprag/internal/platform/linux"
	"cliprag/internal/storage"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const help = `ClipRAG terminal completion

  cliprag-shell install bash|zsh    Enable automatic suggestions in new sessions
  cliprag-shell uninstall bash|zsh  Remove the managed startup entry
  cliprag-shell init bash|zsh       Print shell integration for the current session
  cliprag-shell suggest [--json] [--limit 5]
                                   Read a command prefix from stdin; print matches
  cliprag-shell pick                Read prefix from stdin; interactively select
  cliprag-shell record-use          Record acceptance of a saved command from stdin

At a shell prompt, type at least 3 characters: a saved match appears faded.
Right Arrow accepts it; Esc dismisses it. Enter executes only accepted text.
Alt+/ opens more matches: arrows/Tab select, Enter inserts, Esc cancels.
Normal Tab completion stays intact. ClipRAG never executes a suggestion.
Set CLIPRAG_COMPLETION_ENABLED=0 to pause completion in the current shell.
`

func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func Init(shell, binary string) (string, error) {
	var script string
	switch shell {
	case "bash":
		script = shellassets.Bash
	case "zsh":
		script = shellassets.Zsh
	default:
		return "", errors.New("choose bash or zsh")
	}
	prelude := "_CLIPRAG_COMPLETION_BIN=" + Quote(binary) + "\n"
	if shell == "bash" {
		prelude += `if [[ $- == *i* && -z ${BLE_VERSION-} ]]; then
    if builtin source "$_CLIPRAG_BLE_PATH" --noattach --norc; then
        _CLIPRAG_BASH_OWNS_BLE=1
    fi
fi
`
	}
	result := prelude + "_cliprag_load_integration() {\n" + script + "\n}\n_cliprag_load_integration\nunset -f _cliprag_load_integration\n"
	if shell == "bash" {
		result += "[[ $- != *i* || -z ${_CLIPRAG_BASH_LOADED-} ]] || ble-attach\n"
	}
	return result, nil
}

func Run(args []string, in io.Reader, out, errout io.Writer, binary string) int {
	fail := func(err error) int { fmt.Fprintln(errout, "cliprag-shell:", err); return 1 }
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprint(out, help)
		return 0
	}
	switch args[0] {
	case "init":
		if len(args) != 2 {
			return fail(errors.New("usage: cliprag-shell init bash|zsh"))
		}
		script, err := Init(args[1], binary)
		if err != nil {
			return fail(err)
		}
		if args[1] == "bash" {
			cache, e := os.UserCacheDir()
			if e != nil {
				return fail(e)
			}
			runtime, e := EnsureBashRuntime(cache)
			if e != nil {
				return fail(e)
			}
			script = "_CLIPRAG_BLE_PATH=" + Quote(runtime) + "\n" + script
		}
		_, err = io.WriteString(out, script)
		if err != nil {
			return fail(err)
		}
		return 0
	case "install", "uninstall":
		if len(args) < 2 || (args[1] != "bash" && args[1] != "zsh") {
			return fail(errors.New("choose bash or zsh"))
		}
		flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
		flags.SetOutput(errout)
		rc := flags.String("rc-file", "", "optional absolute path to a shell startup file")
		if err := flags.Parse(args[2:]); err != nil {
			return 1
		}
		if flags.NArg() != 0 {
			return fail(errors.New("unexpected arguments"))
		}
		if *rc == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fail(err)
			}
			if args[1] == "zsh" {
				if zd := os.Getenv("ZDOTDIR"); zd != "" {
					home = zd
				}
			}
			*rc = filepath.Join(home, "."+args[1]+"rc")
		}
		if args[0] == "install" && args[1] == "bash" {
			cache, e := os.UserCacheDir()
			if e != nil {
				return fail(e)
			}
			if _, e = EnsureBashRuntime(cache); e != nil {
				return fail(e)
			}
		}
		changed, err := UpdateRC(*rc, args[1], binary, args[0] == "install")
		if err != nil {
			return fail(err)
		}
		if changed {
			fmt.Fprintf(out, "Updated %s\n", *rc)
		} else {
			fmt.Fprintf(out, "No changes needed in %s\n", *rc)
		}
		if args[0] == "install" {
			fmt.Fprintln(out, "Open a new terminal, or activate in this shell with:")
			fmt.Fprintf(out, "eval \"$(%s init %s)\"\n", Quote(binary), args[1])
		} else {
			fmt.Fprintln(out, "Open a new terminal, or run cliprag_completion_disable in the active shell.")
		}
		return 0
	case "record-use":
		if len(args) != 1 {
			return fail(errors.New("pipe the accepted command through stdin"))
		}
		if file, ok := in.(*os.File); ok {
			if st, err := file.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				return fail(errors.New("pipe the accepted command through stdin"))
			}
		}
		raw, err := io.ReadAll(io.LimitReader(in, storage.MaxSuggestionBytes+1))
		if err != nil {
			return fail(errors.New("could not read accepted command"))
		}
		dir, err := platform.DataDir()
		if err != nil {
			return fail(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Millisecond)
		defer cancel()
		if err = storage.RecordAutocompleteUse(ctx, filepath.Join(dir, "history.db"), string(raw)); err != nil {
			return fail(errors.New("could not record autocomplete use"))
		}
		return 0
	case "suggest", "pick":
		flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
		flags.SetOutput(errout)
		limit := flags.Int("limit", 5, "maximum matches (1–20)")
		jsonOutput := flags.Bool("json", false, "output a JSON string array")
		if err := flags.Parse(args[1:]); err != nil {
			return 1
		}
		if flags.NArg() != 0 || *limit < 1 || *limit > 20 {
			return fail(errors.New("use --limit 1–20; pipe the prefix through stdin"))
		}
		// Avoid waiting indefinitely if someone runs the lookup directly at a prompt.
		if file, ok := in.(*os.File); ok {
			if st, err := file.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				return fail(errors.New("pipe a command prefix through stdin"))
			}
		}
		raw, err := io.ReadAll(io.LimitReader(in, storage.MaxSuggestionBytes+1))
		if err != nil {
			return fail(errors.New("could not read prefix"))
		}
		prefix := string(raw)
		matches, err := Lookup(prefix, *limit)
		if err != nil {
			return fail(errors.New("history is unavailable; try again shortly"))
		}
		if args[0] == "pick" {
			selected, ok, err := Pick(matches, storage.ValidSuggestionPrefix(prefix))
			if err != nil {
				return fail(err)
			}
			if !ok {
				return 2
			}
			_, err = io.WriteString(out, selected)
			if err != nil {
				return fail(err)
			}
			return 0
		}
		if *jsonOutput {
			if err = json.NewEncoder(out).Encode(matches); err != nil {
				return fail(err)
			}
		} else {
			for _, match := range matches {
				if _, err = fmt.Fprintln(out, match); err != nil {
					return fail(err)
				}
			}
		}
		return 0
	default:
		return fail(errors.New("unknown command; run cliprag-shell --help"))
	}
}

func Lookup(prefix string, limit int) ([]string, error) {
	if !storage.ValidSuggestionPrefix(prefix) {
		return []string{}, nil
	}
	dir, err := platform.DataDir()
	if err != nil {
		return nil, err
	}
	db, err := storage.OpenReadOnly(filepath.Join(dir, "history.db"))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Millisecond)
	defer cancel()
	return db.Suggestions(ctx, prefix, limit)
}
