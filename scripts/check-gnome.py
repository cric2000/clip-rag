"""Verify the bridge in a fresh, private, headless GNOME Shell session.

Invoke via test-gnome.sh; never on the real desktop's session bus. Only synthetic
fixture text and temporary settings/data directories are used.
"""
import json
import os
import pathlib
import shutil
import sqlite3
import subprocess
import tempfile
import time


def wait_for(condition, description, timeout=20):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if condition():
            return
        time.sleep(0.1)
    raise AssertionError(description)


with tempfile.TemporaryDirectory(prefix="cliprag-gnome-") as temporary:
    root = pathlib.Path(temporary)
    for directory in ("runtime", "config", "data"):
        (root / directory).mkdir(mode=0o700)
    environment = dict(os.environ, XDG_RUNTIME_DIR=str(root / "runtime"),
                       XDG_CONFIG_HOME=str(root / "config"),
                       XDG_DATA_HOME=str(root / "data"),
                       XDG_CURRENT_DESKTOP="GNOME", XDG_SESSION_TYPE="wayland",
                       GSETTINGS_BACKEND="keyfile", GDK_BACKEND="wayland",
                       WAYLAND_DISPLAY="cliprag-integration", GTK_A11Y="none",
                       GIO_USE_VFS="local")
    extension = root / "data/gnome-shell/extensions/cliprag@cliprag.local"
    shutil.copytree("integrations/gnome-shell", extension)
    fixture = "SELECT id, name\nFROM users\nWHERE active = true;\n"
    source = (extension / "extension.js").read_text()
    # Wayland clipboard ownership requires a valid user-input serial, which a
    # headless GTK client does not have. Create the fixture through this test
    # Shell's native clipboard instead. No input is injected, and this temporary
    # source change is never installed on the real desktop or packaged.
    marker = "this._backend = owner;"
    assert marker in source, "fixture hook no longer matches the bridge source"
    fixture_hook = (
        marker + " GLib.timeout_add(GLib.PRIORITY_DEFAULT, 800, () => {"
        " this._clipboard.set_text(St.ClipboardType.CLIPBOARD, "
        + json.dumps(fixture)
        + "); return GLib.SOURCE_REMOVE; });"
    )
    (extension / "extension.js").write_text(source.replace(marker, fixture_hook))
    subprocess.run(["gsettings", "set", "org.gnome.shell", "enabled-extensions",
                    "['cliprag@cliprag.local']"], env=environment, check=True)
    subprocess.run(["gsettings", "set", "org.gnome.desktop.interface",
                    "enable-animations", "false"], env=environment, check=True)
    processes = []
    shell_log = open(root / "shell.log", "w+")
    app_log = open(root / "app.log", "w+")

    def owns(name):
        result = subprocess.run(["gdbus", "call", "--session", "--dest",
                                 "org.freedesktop.DBus", "--object-path",
                                 "/org/freedesktop/DBus", "--method",
                                 "org.freedesktop.DBus.NameHasOwner", name],
                                env=environment, text=True, capture_output=True,
                                timeout=3)
        return result.returncode == 0 and "true" in result.stdout

    try:
        shell = subprocess.Popen(["gnome-shell", "--wayland", "--headless",
                                  "--virtual-monitor", "1280x720", "--no-x11",
                                  "--wayland-display", "cliprag-integration",
                                  "--mode", "user"], env=environment,
                                 stdout=shell_log, stderr=subprocess.STDOUT)
        processes.append(shell)
        wait_for(lambda: owns("io.github.cliprag.Shell"),
                 "GNOME did not load the clipboard bridge")
        print("PASS GNOME 50 discovers and enables the bridge in a fresh session", flush=True)
        app = subprocess.Popen(["./bin/cliprag", "--hidden"], env=environment,
                               stdout=app_log, stderr=subprocess.STDOUT)
        processes.append(app)
        wait_for(lambda: owns("io.github.cliprag.ClipRAG"), "Go adapter did not start")
        database = root / "data/cliprag/history.db"

        def captured():
            if not database.exists():
                return False
            with sqlite3.connect(database) as connection:
                row = connection.execute("SELECT content,category,copy_count FROM entries").fetchone()
                return row == (fixture, "SQL", 1)

        wait_for(captured, "Shell clipboard fixture was not captured exactly", 12)
        print("PASS GNOME Wayland: native Shell selection fixture -> bridge -> Go -> SQLite; exact multiline text, SQL category and count", flush=True)
    except Exception:
        for label, stream in (("GNOME", shell_log), ("app", app_log)):
            stream.flush()
            stream.seek(0)
            print(label + " test log:\n" + stream.read()[-16000:])
        raise
    finally:
        for process in reversed(processes):
            if process.poll() is None:
                process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        shell_log.close()
        app_log.close()
