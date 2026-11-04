# Browser autocomplete

Requires **ClipRAG v0.3.0+** for Linux Chrome, Chromium, Brave or Edge.
The v0.2.0 release contains terminal autocomplete only; use the source build or
a v0.3.0+ package for browser support.

Browser suggestions cover **all saved text categories**: URLs, normal text,
commands, code, SQL and JSON. Multiline clips are offered in textareas and
standard rich-text fields. The existing secret filter still applies.

## Set up once

1. Install a v0.3.0+ `.deb`, or follow **Build and run from source** in the
   [main README](https://github.com/cric2000/clip-rag#install-on-ubuntu).
   Open ClipRAG once and copy some text.
2. Register the native bridge as your normal user, without `sudo`. Run the command
   for your installation; replace `chrome` with `chromium`, `brave`, or `edge` as needed:

   | Installation | Command |
   | --- | --- |
   | Installed `.deb` | `cliprag-browser install chrome` |
   | Source build (from project root) | `./bin/cliprag-browser install chrome` |

3. Open `chrome://extensions` (or your browser's Extensions page), turn on
   **Developer mode**, choose **Load unpacked**, and select:

   - Source build: the project's `bin/cliprag-browser-extension` folder.
   - Installed package: `/usr/share/cliprag/browser-extension`.

4. Open a website, click ClipRAG's toolbar icon, and choose **Enable on this
   website**. Approve access to that website when Chrome asks. Activation takes
   effect on the open page and is remembered for future visits.

The native bridge uses the same local history as ClipRAG. No HTTP server,
remote service, browser history import or clipboard-read browser permission is needed.
The desktop app can be closed after clips have been captured.

## Use

Type **three or more characters** at the end of a line in an enabled website's
text field. A small ClipRAG panel appears automatically with up to five matches.

- **Up/Down:** choose a match.
- **Tab, Enter or Right Arrow:** insert the chosen text; a click works too.
- **Esc:** close suggestions and keep your input.
- Accepting never submits the website form. Enter can submit normally after
  the suggestions have closed.

Matching is case-insensitive and checks the current line, then the last word
if the line has no matches. Pinned clips rank first, followed by external copy
count and recency. Insertion preserves surrounding text and uses normal browser
editing where available. A clip's **Autocomplete uses** count increases only
after acceptance, and appears live in the desktop app. It shares the counter
with terminal autocomplete; it does not indicate submission or execution.

### Address bar

Type **`clip` then Space**, followed by at least three characters of a saved
HTTP/HTTPS URL. You can omit `https://` and `www.`: for example, type
`clip`, Space, then `github.com`. Press Enter to open the first match or
use Down to select another saved URL. Browser extension APIs
require this keyword; the extension cannot add unrestricted automatic
suggestions to the normal address bar. Website text fields support all text
types without the keyword.

## Controls and limits

- Enable or disable a website from the toolbar popup. Its pause switch stops
  browser suggestions everywhere without affecting terminal suggestions or capture.
- Access is optional per website. The extension only queries fields after trusted
  typing events. Preview text is rendered as plain text inside a closed shadow root;
  it is not inserted into the website's field until you accept it.
- No copied content, typed prefixes or matching results are saved in browser
  storage. Only enabled website origins and the pause preference are stored.
- Password, payment and common credential/OTP fields, private browsing, read-only
  fields and browser-internal pages are excluded. Native browser autofill may
  still appear independently. Custom canvas editors and some specialized rich
  editors may need their own adapter.
- A single-line field does not receive multiline clips. Browser matching supports
  clips up to 100 KB and prefixes up to 2 KB; results and native messages are bounded.
  Browser textareas normalize line endings according to normal browser behavior.

## Troubleshooting

- **Local bridge unavailable:** run the install command for the browser you
  actually use. The popup reports whether the native bridge is connected.
- **No matches:** copy eligible text with ClipRAG running, enable this website,
  and type at least three characters from the beginning of that clip. In Gmail,
  type in the message body; signatures or quoted messages below the cursor do
  not prevent suggestions. Browser matching includes URLs and prose; terminal
  matching continues to use its command-only filter.
- **Website says enabled but is inactive:** open the ClipRAG popup once to repair
  activation. Disabling and enabling again is not needed. After updating the
  extension, refresh an older open page if the popup asks you to do so.
- **Custom data location:** pass `--data-dir /absolute/path/to/cliprag` during
  installation. It must be the directory containing `history.db` shown in Settings.
  This setting is stored privately under `$XDG_CONFIG_HOME/cliprag/browser.json`.
- **Custom Chrome profile:** pass `--manifest-dir /absolute/profile/NativeMessagingHosts`.
  Sandboxed Snap/Flatpak browsers may not be able to launch this native bridge;
  use a native browser package for this version.
- **Updating:** after upgrading the package or rebuilding, click **Reload** on
  the extension's card in Extensions, then refresh already-open websites once.
  If switching from source to the `.deb`, run `cliprag-browser install chrome`
  again and load `/usr/share/cliprag/browser-extension` instead of the source
  folder. Re-register the bridge whenever its binary path changes.

## Remove

```bash
cliprag-browser uninstall chrome
# Source build: ./bin/cliprag-browser uninstall chrome
```

Then remove ClipRAG from your browser's Extensions page. Your saved history is retained.

## Checks

`./scripts/test-browser.sh` runs a real headless Google Chrome instance with a
private profile, temporary SQLite history, localhost-only fixtures, and native
messaging. It tests text/URL/SQL/JSON/code matching, multiline and rich-text
insertion (including Gmail-style composers with signatures and quoted mail),
React inputs, secret/field exclusions, acceptance counts, cancellation,
stale results, pausing, first-enable activation, concurrent preference reads,
saved opt-ins after navigation and saved-URL navigation. Your normal profile and clipboard
are not touched. `CLIPRAG_CHROME` can select a different native Chrome executable.
