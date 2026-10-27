"""Register the source build and its icon in the current user's application menu."""
import os
import pathlib
import shutil
import subprocess

root = pathlib.Path(__file__).resolve().parent.parent
binary = root / 'bin/cliprag'
if not binary.is_file():
    raise SystemExit('Build ClipRAG before installing the desktop launcher')
data = pathlib.Path(os.environ.get('XDG_DATA_HOME', pathlib.Path.home() / '.local/share'))
if not data.is_absolute():
    raise SystemExit('XDG_DATA_HOME must be an absolute path')
applications = data / 'applications'
icons = data / 'icons/hicolor'
icon_directory = icons / 'scalable/apps'
applications.mkdir(parents=True, exist_ok=True)
icon_directory.mkdir(parents=True, exist_ok=True)
shutil.copyfile(root / 'packaging/linux/cliprag.svg', icon_directory / 'cliprag.svg')

# Desktop Entry Exec quoting differs from shell quoting. Preserve literal paths,
# including percent characters, without allowing desktop field-code expansion.
quoted = str(binary).replace('\\', '\\\\').replace('"', '\\"').replace('`', '\\`').replace('$', '\\$').replace('%', '%%')
source = (root / 'packaging/linux/cliprag.desktop').read_text()
source = source.replace('Exec=cliprag ', f'Exec="{quoted}" ')
launcher = applications / 'cliprag.desktop'
launcher.write_text(source)
launcher.chmod(0o644)
for command, argument in [('update-desktop-database', applications), ('gtk-update-icon-cache', icons)]:
    if shutil.which(command):
        flags = ['-f', '-t'] if command == 'gtk-update-icon-cache' else []
        subprocess.run([command, *flags, str(argument)], check=True)
print(f'Installed ClipRAG launcher: {launcher}')
print(f'Installed ClipRAG icon: {icon_directory / "cliprag.svg"}')
