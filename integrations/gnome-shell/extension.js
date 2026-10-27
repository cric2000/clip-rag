// Original ClipRAG integration. GNOME Shell 50; no third-party source copied.
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import Meta from 'gi://Meta';
import St from 'gi://St';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

const NAME = 'io.github.cliprag.ClipRAG';
const PATH = '/io/github/cliprag/ClipRAG';
const LIMIT = 1024 * 1024;
const XML = `<node><interface name="io.github.cliprag.Shell">
<method name="Read"><arg type="s" direction="out"/></method>
<method name="Write"><arg type="s" direction="in"/></method>
</interface></node>`;

export default class ClipRAGBridge extends Extension {
    enable() {
        this._sequence = GLib.get_monotonic_time();
        this._selection = global.display.get_selection();
        this._clipboard = St.Clipboard.get_default();
        this._watch = Gio.bus_watch_name(Gio.BusType.SESSION, NAME, Gio.BusNameWatcherFlags.NONE,
            (_c, _n, owner) => { this._backend = owner; }, () => { this._backend = null; });
        this._object = Gio.DBusExportedObject.wrapJSObject(XML, this);
        this._object.export(Gio.DBus.session, '/io/github/cliprag/Shell');
        this._own = Gio.bus_own_name_on_connection(Gio.DBus.session, 'io.github.cliprag.Shell', Gio.BusNameOwnerFlags.NONE, null, null);
        this._signal = this._selection.connect('owner-changed', (_s, type, owner) => {
            if (type !== Meta.SelectionType.SELECTION_CLIPBOARD || !owner || !this._backend) return;
            const sequence = ++this._sequence;
            this._read((text, sensitive, error) => {
                // Never forward stale results from an older selection owner.
                if (error || !this._backend || sequence !== this._sequence) return;
                Gio.DBus.session.call(this._backend, PATH, NAME, 'Capture',
                    new GLib.Variant('(sbt)', [text, sensitive, sequence]), null,
                    Gio.DBusCallFlags.NONE, 2000, null, (c, r) => {
                        try { c.call_finish(r); } catch (_) { /* daemon stopped; no content logged */ }
                    });
            });
        });
    }
    _read(callback) {
        this._transfer?.cancel();
        const cancel = new Gio.Cancellable();
        this._transfer = cancel;
        const types = this._selection.get_mimetypes(Meta.SelectionType.SELECTION_CLIPBOARD);
        const sensitive = types.some(t => ['x-kde-passwordManagerHint', 'application/x-keepassxc-sensitive', 'application/x-cliprag-sensitive'].includes(t));
        if (sensitive) { callback('', true, null); return; }
        const mime = ['text/plain;charset=utf-8', 'text/plain;charset=UTF-8', 'UTF8_STRING', 'text/plain'].find(t => types.includes(t));
        if (!mime) { callback('', false, null); return; }
        const stream = Gio.MemoryOutputStream.new_resizable();
        this._selection.transfer_async(Meta.SelectionType.SELECTION_CLIPBOARD, mime, LIMIT + 1,
            stream, cancel, (selection, result) => {
                try {
                    selection.transfer_finish(result);
                    stream.close(null);
                    const bytes = stream.steal_as_bytes().get_data();
                    if (bytes.length > LIMIT) throw new Error('oversized');
                    callback(new TextDecoder('utf-8', {fatal: true}).decode(bytes), false, null);
                } catch (_) { callback('', false, true); }
            });
    }
    _authorized(invocation) {
        if (this._backend && invocation.get_sender() === this._backend) return true;
        invocation.return_dbus_error('org.freedesktop.DBus.Error.AccessDenied', 'Only the ClipRAG daemon may access the bridge');
        return false;
    }
    ReadAsync(_args, invocation) {
        if (!this._authorized(invocation)) return;
        this._read((text, sensitive, error) => {
            if (error || sensitive) invocation.return_dbus_error('io.github.cliprag.Shell.Unavailable', 'Clipboard unavailable or marked sensitive');
            else invocation.return_value(new GLib.Variant('(s)', [text]));
        });
    }
    WriteAsync([text], invocation) {
        if (!this._authorized(invocation)) return;
        if (new TextEncoder().encode(text).length > LIMIT) {
            invocation.return_dbus_error('io.github.cliprag.Shell.TooLarge', 'Clipboard text exceeds the bridge limit'); return;
        }
        this._clipboard.set_text(St.ClipboardType.CLIPBOARD, text);
        invocation.return_value(null);
    }
    disable() {
        this._backend = null;
        this._sequence++;
        this._transfer?.cancel();
        if (this._signal) this._selection.disconnect(this._signal);
        if (this._watch) Gio.bus_unwatch_name(this._watch);
        if (this._own) Gio.bus_unown_name(this._own);
        this._object?.unexport();
        this._object = this._selection = this._clipboard = this._transfer = null;
        this._signal = this._watch = this._own = 0;
    }
}
