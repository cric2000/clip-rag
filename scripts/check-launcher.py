"""Production binary lifecycle check; run only in the isolated test display/bus."""
import json
import os
import pathlib
import sqlite3
import subprocess
import tempfile
import time


def wait_for(predicate, label):
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        try:
            if predicate():
                return
        except (sqlite3.Error, FileNotFoundError):
            pass
        time.sleep(0.03)
    raise AssertionError(label)


with tempfile.TemporaryDirectory(prefix="cliprag-launcher-") as temporary:
    environment = dict(os.environ, XDG_DATA_HOME=temporary)
    database = pathlib.Path(temporary) / "cliprag/history.db"
    owners = []
    process = None

    def count():
        with sqlite3.connect(database) as connection:
            return connection.execute("SELECT count(*) FROM entries").fetchone()[0]

    def monitoring():
        with sqlite3.connect(database) as connection:
            row = connection.execute("SELECT value FROM settings WHERE id=1").fetchone()
            return json.loads(row[0])["monitoring"] if row else True

    def external_copy(value):
        owner = subprocess.Popen(["xclip", "-quiet", "-selection", "clipboard", "-in"],
                                 stdin=subprocess.PIPE, env=environment)
        owner.stdin.write(value.encode())
        owner.stdin.close()
        owners.append(owner)

    def command(flag):
        subprocess.run(["./bin/cliprag", flag], env=environment, check=True,
                       timeout=10, stdout=subprocess.DEVNULL)

    def check_shell_lookup():
        lookup = subprocess.run(["./bin/cliprag-shell", "suggest", "--json"],
                                input="git s", text=True, capture_output=True,
                                env=environment, check=True, timeout=3)
        assert json.loads(lookup.stdout) == ["git status"], "shell helper missed captured history"

    def record_use(expected):
        subprocess.run(["./bin/cliprag-shell", "record-use"], input="git status",
                       text=True, capture_output=True, env=environment, check=True, timeout=3)
        with sqlite3.connect(database) as connection:
            row = connection.execute("SELECT autocomplete_count,copy_count,restore_count FROM entries WHERE content='git status'").fetchone()
            assert row == (expected, 1, 0), "acceptance counter or unrelated counters incorrect"

    try:
        process = subprocess.Popen(["./bin/cliprag", "--hidden"], env=environment)
        wait_for(lambda: database.exists() and count() == 0, "database startup")
        # Wait for the XFixes listener, then create an external selection owner.
        time.sleep(0.8)
        external_copy("git status")
        wait_for(lambda: count() == 1, "production capture")
        check_shell_lookup()
        record_use(1)
        command("--pause")
        wait_for(lambda: not monitoring(), "single instance pause")
        external_copy("paused fixture")
        time.sleep(0.2)
        assert count() == 1, "paused content persisted"
        command("--resume")
        wait_for(monitoring, "single instance resume")
        external_copy("docker ps")
        wait_for(lambda: count() == 2, "resumed capture")
        command("--show")
        command("--quit")
        assert process.wait(timeout=10) == 0
        check_shell_lookup()
        record_use(2)
        process = subprocess.Popen(["./bin/cliprag", "--hidden"], env=environment)
        time.sleep(0.8)
        assert count() == 2, "history lost on process restart"
        with sqlite3.connect(database) as connection:
            assert connection.execute("SELECT autocomplete_count FROM entries WHERE content='git status'").fetchone()[0] == 2, "acceptance count lost on restart"
        command("--quit")
        assert process.wait(timeout=10) == 0
        print("PASS production launcher: hidden capture, single-instance controls, persistent autocomplete counts with app running/closed")
    finally:
        if process and process.poll() is None:
            process.terminate()
            process.wait(timeout=10)
        for owner in owners:
            if owner.poll() is None:
                owner.terminate()
            owner.wait(timeout=5)
