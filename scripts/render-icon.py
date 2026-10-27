"""Render the original SVG mark to the PNG used by Wails and the system tray."""
import ctypes
import pathlib

root = pathlib.Path(__file__).resolve().parent.parent
rsvg = ctypes.CDLL('librsvg-2.so.2')
cairo = ctypes.CDLL('libcairo.so.2')
gobject = ctypes.CDLL('libgobject-2.0.so.0')
rsvg.rsvg_handle_new_from_file.argtypes = [ctypes.c_char_p, ctypes.c_void_p]
rsvg.rsvg_handle_new_from_file.restype = ctypes.c_void_p
rsvg.rsvg_handle_render_cairo.argtypes = [ctypes.c_void_p, ctypes.c_void_p]
rsvg.rsvg_handle_render_cairo.restype = ctypes.c_int
cairo.cairo_image_surface_create.argtypes = [ctypes.c_int, ctypes.c_int, ctypes.c_int]
cairo.cairo_image_surface_create.restype = ctypes.c_void_p
cairo.cairo_create.argtypes = [ctypes.c_void_p]
cairo.cairo_create.restype = ctypes.c_void_p
cairo.cairo_surface_write_to_png.argtypes = [ctypes.c_void_p, ctypes.c_char_p]
cairo.cairo_surface_write_to_png.restype = ctypes.c_int
cairo.cairo_destroy.argtypes = [ctypes.c_void_p]
cairo.cairo_surface_destroy.argtypes = [ctypes.c_void_p]
gobject.g_object_unref.argtypes = [ctypes.c_void_p]
handle = rsvg.rsvg_handle_new_from_file(str(root / 'packaging/linux/cliprag.svg').encode(), None)
if not handle:
    raise SystemExit('Could not load the SVG icon')
surface = cairo.cairo_image_surface_create(0, 512, 512)
context = cairo.cairo_create(surface)
try:
    if not rsvg.rsvg_handle_render_cairo(handle, context):
        raise SystemExit('Could not render the SVG icon')
    result = cairo.cairo_surface_write_to_png(surface, str(root / 'cmd/cliprag/icon.png').encode())
    if result:
        raise SystemExit('Could not save the PNG icon')
finally:
    cairo.cairo_destroy(context)
    cairo.cairo_surface_destroy(surface)
    gobject.g_object_unref(handle)
print('Rendered cmd/cliprag/icon.png from packaging/linux/cliprag.svg')
