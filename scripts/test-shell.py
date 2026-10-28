#!/usr/bin/env python3
"""Exercise the built helper and real interactive Bash/Zsh in a private PTY.
No user startup file or desktop clipboard is read or changed.
"""
import argparse
import hashlib
import os
import pathlib
import pty
import re
import select
import shlex
import shutil
import signal
import sqlite3
import struct
import subprocess
import tempfile
import termios
import time
import fcntl

ROOT = pathlib.Path(__file__).resolve().parent.parent


class Shell:
    def __init__(self, command, environment, cwd):
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            os.chdir(cwd)
            os.execvpe(command[0], command, environment)
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ, struct.pack("HHHH", 28, 110, 0, 0))
        self.pending = b""
        self.sequence = 0

    def send(self, text):
        os.write(self.fd, text.encode() if isinstance(text, str) else text)

    def until(self, token, timeout=8, visible=False):
        if isinstance(token, str):
            token = token.encode()
        # ZLE may change colour between every character of the faded suffix.
        ansi = rb'(?:\x1b\[[0-?]*[ -/]*[@-~]|\x1b[()][0-9A-Za-z])*'
        pattern = re.compile(ansi.join(re.escape(bytes([c])) for c in token)) if visible else re.compile(re.escape(token))
        deadline = time.monotonic() + timeout
        while not (match := pattern.search(self.pending)):
            if time.monotonic() > deadline:
                raise AssertionError(f"PTY did not produce {token!r}: {self.pending[-1400:]!r}")
            readable, _, _ = select.select([self.fd], [], [], .05)
            if readable:
                try:
                    data = os.read(self.fd, 65536)
                except OSError as exc:
                    raise AssertionError(f"shell exited: {self.pending[-1000:]!r}") from exc
                if not data:
                    raise AssertionError("PTY closed")
                # Reply to terminal capability queries used by the Bash editor.
                for _ in range(data.count(b"\x1b[6n")):
                    self.send(b"\x1b[1;1R")
                for _ in range(data.count(b"\x1b[?6n")):
                    self.send(b"\x1b[?1;1R")
                if b"\x1b[c" in data:
                    self.send(b"\x1b[?1;2c")
                self.pending += data
        index = match.end()
        result, self.pending = self.pending[:index], self.pending[index:]
        return result

    def run(self, command):
        self.sequence += 1
        marker = f"__CLIPRAG_DONE_{self.sequence}__"
        self.send(command + "; printf '\\n" + marker + "\\n'\n")
        result = self.until("\r\n" + marker + "\r\n")
        return result

    def probe(self):
        self.send(b"\x18\x02")
        self.until(b"\r\nBUFFER[")
        result = self.until(b"]END")
        return result[:-4].decode()

    def close(self):
        try:
            self.send(b"\x03\x15exit\n")
            deadline = time.monotonic() + 2
            while time.monotonic() < deadline:
                if os.waitpid(self.pid, os.WNOHANG)[0]:
                    return
                time.sleep(.02)
            os.kill(self.pid, signal.SIGKILL)
            os.waitpid(self.pid, 0)
        finally:
            os.close(self.fd)


def exercise(name, binary, environment, work, helper, sentinel):
    database = pathlib.Path(environment['XDG_DATA_HOME']) / 'cliprag/history.db'
    def uses(command):
        with sqlite3.connect(database) as connection:
            return connection.execute('SELECT autocomplete_count FROM entries WHERE content=?', (command,)).fetchone()[0]
    baseline = {command: uses(command) for command in ['git cherry-pick abc', 'git checkout main', 'git checkout café']}
    args = [binary, "--noprofile", "--norc", "-i"] if name == "bash" else [binary, "-d", "-f", "-i"]
    shell = Shell(args, environment, work)
    try:
        # Initial prompts differ; a marker distinguishes startup from typed input.
        bootstrap = ""
        if name == "zsh" and environment.get("CLIPRAG_TEST_MODULE_PATH"):
            bootstrap = "module_path=(" + shlex.quote(environment["CLIPRAG_TEST_MODULE_PATH"]) + " $module_path); fpath=(" + shlex.quote(str(pathlib.Path(binary).parents[1] / "share/zsh/functions/Misc")) + " $fpath); zmodload zsh/zle; setopt zle; "
        shell.send(bootstrap + "PS1='CLIPRAG> '; PROMPT='CLIPRAG> '; printf '\\nSTARTED\\n'\n")
        shell.until(b"\r\nSTARTED\r\n")
        shell.until("CLIPRAG> ")
        init = f'eval "$({shlex.quote(str(helper))} init {name})"'
        if name == "bash":
            shell.run("bind -m emacs-standard '\"\\e/\": forward-word'")
            shell.run("_cliprag_probe() { printf '\\nBUFFER[%s]END\\n' \"$READLINE_LINE\"; READLINE_LINE=; READLINE_POINT=0; }; bind -x '\"\\C-x\\C-b\":_cliprag_probe'")
        else:
            shell.run("bindkey -e; bindkey -M emacs '^[/' forward-word")
            shell.run("_cliprag_probe() { zle -I; print -r -- ''; print -r -- \"BUFFER[${BUFFER}]END\"; BUFFER=; zle reset-prompt; }; zle -N cliprag-probe _cliprag_probe; bindkey '^X^B' cliprag-probe")
        shell.run(init)
        shell.run(init)  # Loading twice must preserve the original key binding.

        # No shortcut: the faded suffix must arrive solely from typing.
        shell.send("git ch")
        shell.until("erry-pick abc", visible=True)
        assert shell.probe() == "git ch", name + ": ghost text entered the actual command"
        assert uses('git cherry-pick abc') == baseline['git cherry-pick abc'], name + ': viewing counted as acceptance'
        shell.send('git ch')
        shell.until('erry-pick abc', visible=True)
        shell.send('erry-pick abc')
        assert shell.probe() == 'git cherry-pick abc', name + ': typing full command changed text'
        assert uses('git cherry-pick abc') == baseline['git cherry-pick abc'], name + ': manually typed text counted'
        shell.send("git ch")
        shell.until("erry-pick abc", visible=True)
        shell.send(b"\x1b[C")
        shell.send(b"\x1b[C")  # Another cursor movement must not count twice.
        assert shell.probe() == "git cherry-pick abc", name + ": Right Arrow did not accept"
        assert uses('git cherry-pick abc') == baseline['git cherry-pick abc'] + 1, name + ': acceptance was not recorded exactly once'
        shell.send("git che")
        shell.until("rry-pick abc", visible=True)
        shell.send("c")
        shell.until("kout main", visible=True)
        shell.send(b"\x1b[C")
        assert shell.probe() == "git checkout main", name + ": typing did not update suggestion"
        shell.send("git chec")
        shell.until("kout main", visible=True)
        shell.send(b"\x7f")
        shell.until("rry-pick abc", visible=True)
        shell.send(b"\x1b[C")
        assert shell.probe() == "git cherry-pick abc", name + ": deletion did not refresh suggestion"
        shell.send("git checkout c")
        shell.until("afé", visible=True)
        shell.send(b"\x1b[C")
        assert shell.probe() == "git checkout café", name + ": automatic Unicode acceptance"
        shell.send("git ch")
        shell.until("erry-pick abc", visible=True)
        shell.send(b"\x1b")
        time.sleep(.6)
        assert shell.probe() == "git ch", name + ": dismiss changed typed text"
        assert uses('git cherry-pick abc') == baseline['git cherry-pick abc'] + 2, name + ': dismissal counted as acceptance'
        shell.send("echo SAFE_ENTER")
        shell.until("&& touch", visible=True)
        shell.send(b"\r")
        shell.until(b"SAFE_ENTER\r\n")
        shell.run(":")
        assert not sentinel.exists(), name + ": Enter executed an unaccepted suggestion"
        assert uses(f'echo SAFE_ENTER && touch {sentinel}') == 0, name + ': Enter counted the unaccepted suffix'
        print(f"PASS {name}: automatic ghost text, Right Arrow, live prefix updates, dismissal, Enter isolation", flush=True)

        def choose(prefix, keys):
            shell.send(prefix)
            shell.send(b"\x1b/")
            screen = shell.until("clipboard commands")
            shell.send(keys)
            shell.until(b"\x1b[?1049l")
            return shell.probe()

        assert choose("git ch", b"\x1b[B\r") == "git checkout main", name + ": ranked selection"
        assert choose("git ch", b"\x1b") == "git ch", name + ": Escape changed prefix"
        assert choose("git ch", b"\x03") == "git ch", name + ": Ctrl-C changed prefix"
        assert uses('git checkout main') == baseline['git checkout main'] + 2, name + ': picker did not count exactly once'
        assert uses('git cherry-pick abc') == baseline['git cherry-pick abc'] + 2, name + ': cancelled picker counted'
        assert choose("git ch", b"\t\t\r") == "git checkout café", name + ": Unicode text changed"
        assert choose("go t", b"\r") == "go test ./...", name + ": developer command missing"
        # Usage persistence is best effort: a locked database must not lose input.
        go_uses = uses('go test ./...')
        with sqlite3.connect(database) as writer:
            writer.execute('BEGIN IMMEDIATE')
            shell.send('go t')
            shell.until('est ./...', visible=True)
            shell.send(b'\x1b[C')
            assert shell.probe() == 'go test ./...', name + ': usage write failure prevented insertion'
            writer.rollback()
        assert uses('go test ./...') == go_uses, name + ': locked write unexpectedly counted'
        expected = f"echo $(touch {sentinel}) && printf never"
        assert choose("echo ", b"\r") == expected, name + ": literal metacharacters changed"
        assert not sentinel.exists(), "selecting a completion executed shell syntax"
        assert choose("no-match", b"\r") == "no-match", name + ": empty selection changed input"
        assert choose("gi", b"\x1b") == "gi", name + ": short prefix changed input"

        # Editing in the middle must not replace the suffix or open a picker.
        shell.send("git chXYZ")
        shell.send(b"\x01\x1b/")
        assert shell.probe() == "git chXYZ", name + ": middle-of-line editing changed"
        # Standard Tab still uses the shell's normal file completion.
        shell.send("cat completion-fixt\t")
        shell.until("ure", visible=True)
        actual = shell.probe()
        assert actual.rstrip() == "cat completion-fixture", (name, actual)
        # Opt-out is immediate, without needing to remove startup configuration.
        shell.run("CLIPRAG_COMPLETION_ENABLED=0")
        shell.send("git ch")
        time.sleep(.3)
        shell.send(b"\x1b[C\x1b/")
        assert shell.probe() == "git ch", name + ": pause ignored"
        shell.run("CLIPRAG_COMPLETION_ENABLED=1")
        # The same shortcut works in vi insert mode without changing vi command mode.
        if name == "bash":
            shell.run("set -o vi; bind -m vi-insert -x '\"\\C-x\\C-b\":_cliprag_probe'")
        else:
            shell.run("bindkey -v; bindkey -M viins '^X^B' cliprag-probe")
        shell.send("git ch")
        shell.until("erry-pick abc", visible=True)
        shell.send(b"\x1b[C")
        assert shell.probe() == "git cherry-pick abc", name + ": automatic vi insert mode"
        assert choose("git ch", b"\r") == "git cherry-pick abc", name + ": vi insert mode"
        assert uses('git cherry-pick abc') == baseline['git cherry-pick abc'] + 4, name + ': vi acceptance count'
        assert uses('git checkout café') == baseline['git checkout café'] + 2, name + ': Unicode acceptance count'
        if name == "bash":
            shell.run("set -o emacs")
        else:
            shell.run("bindkey -e")
        shell.run("cliprag_completion_disable")
        if name == "bash":
            restored = shell.run("if [[ $(bind -m emacs-standard -p) == *'\"\\e/\": forward-word'* ]]; then printf '\\nRESTORED\\n'; fi")
        else:
            restored = shell.run("if [[ $(bindkey -M emacs '^[/' ) == *' forward-word' ]]; then printf '\\nRESTORED\\n'; fi")
        assert b"\r\nRESTORED\r\n" in restored, (name, "previous binding was not restored", restored[-1000:])
        shell.send("printf '\\nFINISHED\\n'\n")
        shell.until(b"\r\nFINISHED\r\n")
        shell.until("CLIPRAG> ")
        print(f"PASS {name}: ranked picker, literal insertion, Unicode, cancel, normal Tab, cursor guard, pause, exact acceptance counts")
    finally:
        shell.close()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--zsh", default=shutil.which("zsh"))
    parser.add_argument("--zsh-module-path")
    options = parser.parse_args()
    if not options.zsh:
        raise SystemExit("Zsh is required for this check: install zsh or pass --zsh /path/to/zsh")
    helper = ROOT / "bin/cliprag-shell"
    with tempfile.TemporaryDirectory(prefix="cliprag-shell-test-") as temporary:
        work = pathlib.Path(temporary)
        data = work / "data/cliprag"
        data.mkdir(parents=True, mode=0o700)
        database = data / "history.db"
        schema = re.search(r"const schemaV1 = `(.*?)`", (ROOT / "internal/storage/migrations.go").read_text(), re.S).group(1)
        migration = re.search(r"const schemaV2 = `(.*?)`", (ROOT / "internal/storage/migrations.go").read_text(), re.S).group(1)
        sentinel = work / "MUST-NOT-EXECUTE"
        with sqlite3.connect(database) as connection:
            connection.executescript(schema)
            connection.executescript(migration)
            records = [
                ("git checkout main", "Git", 5, 0),
                ("git cherry-pick abc", "Git", 1, 1),
                ("git checkout café", "Git", 2, 0),
                ("go test ./...", "Text", 1, 0),
                (f"echo $(touch {sentinel}) && printf never", "Text", 2, 0),
                (f"echo SAFE_ENTER && touch {sentinel}", "Text", 1, 0),
                ("git malicious\x1b[31m", "Git", 100, 0),
                ("git multiline\nwhoami", "Git", 100, 0),
            ]
            now = int(time.time() * 1000)
            for text, category, count, pin in records:
                connection.execute("INSERT INTO entries(content,hash,category,first_copied,last_copied,copy_count,pinned) VALUES(?,?,?,?,?,?,?)",
                                   (text, hashlib.sha256(text.encode()).hexdigest(), category, now, now, count, pin))
        database.chmod(0o600)
        (work / "completion-fixture").touch()
        environment = dict(os.environ, XDG_DATA_HOME=str(work / "data"), XDG_CACHE_HOME=str(work / "cache"), TERM="xterm-256color", LC_ALL="C.UTF-8")
        (work / "cache").mkdir(mode=0o700)
        if options.zsh_module_path:
            environment["CLIPRAG_TEST_MODULE_PATH"] = options.zsh_module_path
        fields = 'id,content,hash,category,first_copied,last_copied,copy_count,restore_count,pinned'
        with sqlite3.connect(database) as connection:
            before = connection.execute('SELECT '+fields+' FROM entries ORDER BY id').fetchall()
        # An installation path with spaces, quotes and shell syntax stays literal.
        quoted_helper = work / "Clip RAG's tools $(not-a-command)/cliprag-shell"
        quoted_helper.parent.mkdir()
        shutil.copy2(helper, quoted_helper)
        for name, binary in [("bash", shutil.which("bash")), ("zsh", options.zsh)]:
            exercise(name, binary, environment, work, quoted_helper, sentinel)
        with sqlite3.connect(database) as connection:
            after = connection.execute('SELECT '+fields+' FROM entries ORDER BY id').fetchall()
        assert after == before, "acceptance changed clipboard contents, timestamps or other counters"
        print("PASS shell completion: only accepted-use counters changed; no command execution or real clipboard/config changes")


if __name__ == "__main__":
    main()
