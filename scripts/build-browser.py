#!/usr/bin/env python3
"""Build the unpacked Chromium extension and its portable ZIP, without npm."""
import json
import pathlib
import shutil
import zipfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
source = ROOT / "integrations/browser/extension"
destination = ROOT / "bin/cliprag-browser-extension"
if destination.is_symlink():
    raise SystemExit("Extension build directory must not be a symlink")
if destination.exists():
    shutil.rmtree(destination)
destination.mkdir(parents=True, exist_ok=True)
manifest = json.loads((source / "manifest.json").read_text())
identity = json.loads((ROOT / "integrations/browser/identity.json").read_text())
assert manifest["key"] == identity["key"]
for path in source.iterdir():
    if path.is_file():
        shutil.copy2(path, destination / path.name)
with zipfile.ZipFile(ROOT / "bin/cliprag-browser-extension.zip", "w", zipfile.ZIP_DEFLATED) as archive:
    for path in sorted(destination.iterdir()):
        archive.write(path, path.name)
print("Browser extension:", destination)
print("Stable ID:", identity["id"])
