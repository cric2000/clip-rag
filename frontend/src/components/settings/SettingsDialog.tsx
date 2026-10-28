import { useEffect, useRef, useState } from "react";
import {
  X,
  ShieldCheck,
  Trash2,
  HardDrive,
  Radio,
  Check,
  AlertCircle,
  FolderOpen,
} from "lucide-react";
import type { Settings } from "../../types";
export function SettingsDialog({
  initial,
  dataDir,
  onClose,
  onSave,
  onClear,
}: {
  initial: Settings;
  dataDir: string;
  onClose: () => void;
  onSave: (s: Settings) => Promise<void>;
  onClear: () => Promise<void>;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [value, setValue] = useState(initial);
  const [confirm, setConfirm] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  async function save(e: React.SubmitEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await onSave(value);
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <dialog
      ref={dialog}
      className="settings-dialog"
      onCancel={onClose}
      aria-labelledby="settings-title"
    >
      <div className="dialog-heading">
        <div>
          <h2 id="settings-title">Clipboard settings</h2>
          <p>Choose what ClipRAG captures and keeps.</p>
        </div>
        <button
          className="icon-button"
          aria-label="Close settings"
          title="Close · Esc"
          onClick={onClose}
        >
          <X size={20} />
        </button>
      </div>
      <form onSubmit={save}>
        <section className="settings-section">
          <h3>
            <Radio size={16} />
            Capture
          </h3>
          <label className="checkbox-row">
            <span>
              <strong>Capture clipboard changes</strong>
              <small>Save eligible text as you copy it in other apps.</small>
            </span>
            <input
              type="checkbox"
              role="switch"
              checked={value.monitoring}
              onChange={(e) =>
                setValue({ ...value, monitoring: e.target.checked })
              }
            />
          </label>
        </section>
        <section className="settings-section">
          <h3>
            <HardDrive size={16} />
            Keep what’s useful
          </h3>
          <div className="settings-grid">
            <label>
              History limit
              <span className="input-with-unit">
                <input
                  type="number"
                  aria-label="Unpinned history limit"
                  min="1"
                  max="100000"
                  required
                  value={value.maxEntries}
                  onChange={(e) =>
                    setValue({ ...value, maxEntries: Number(e.target.value) })
                  }
                />
                <span>clips</span>
              </span>
            </label>
            <label>
              Keep history for
              <span className="input-with-unit">
                <input
                  type="number"
                  aria-label="Retention in days"
                  min="1"
                  max="3650"
                  required
                  value={value.retentionDays}
                  onChange={(e) =>
                    setValue({
                      ...value,
                      retentionDays: Number(e.target.value),
                    })
                  }
                />
                <span>days</span>
              </span>
            </label>
            <label>
              Maximum clip size
              <span className="input-with-unit">
                <input
                  type="number"
                  aria-label="Maximum entry size (KB)"
                  min="1"
                  max="1024"
                  required
                  value={value.maxEntryBytes / 1024}
                  onChange={(e) =>
                    setValue({
                      ...value,
                      maxEntryBytes: Math.round(Number(e.target.value) * 1024),
                    })
                  }
                />
                <span>KB</span>
              </span>
            </label>
          </div>
          <p className="muted">
            Pinned clips are kept until you choose to delete them.
          </p>
        </section>
        <div className="privacy-note">
          <ShieldCheck size={19} />
          <div>
            <strong>Your clipboard stays yours.</strong>
            <p>
              Stored only on this device. Detectable secrets are excluded, but
              filtering cannot catch everything. Your database is not encrypted.
            </p>
          </div>
        </div>
        <div className="data-location">
          <FolderOpen size={15} />
          <div>
            <span>LOCAL DATA DIRECTORY</span>
            <code>{dataDir}</code>
          </div>
        </div>
        <details className="terminal-setup">
          <summary>Terminal autocomplete · Bash &amp; Zsh</summary>
          <p>Enable once, then open a new terminal:</p>
          <code>cliprag-shell install bash</code>
          <p>
            Use <code>zsh</code> instead of <code>bash</code> for Zsh.
          </p>
          <p>
            Type at least 3 characters. A saved match appears as faded text.
            Press <kbd>→</kbd> to accept or <kbd>Esc</kbd> to dismiss. Enter runs
            only the text you’ve typed or accepted.
          </p>
          <p>
            For more matches, press <kbd>Alt /</kbd>. Choose with arrows or Tab;
            Enter inserts your selection.
          </p>
        </details>
        {error && (
          <p role="alert" className="form-error">
            <AlertCircle size={15} />
            {error}
          </p>
        )}
        <div className="dialog-footer">
          <button className="button" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="button primary" disabled={busy} type="submit">
            <Check size={14} />
            {busy ? "Saving…" : "Save settings"}
          </button>
        </div>
      </form>
      <div className="clear-zone">
        {confirm ? (
          <div className="clear-confirm">
            <AlertCircle size={18} />
            <div>
              <strong>Clear your entire clipboard history?</strong>
              <p>This includes pinned clips and cannot be undone.</p>
              <div className="row">
                <button
                  className="button"
                  disabled={busy}
                  onClick={() => setConfirm(false)}
                >
                  Keep history
                </button>
                <button
                  className="button destructive"
                  disabled={busy}
                  onClick={async () => {
                    setBusy(true);
                    setError("");
                    try {
                      await onClear();
                      setConfirm(false);
                    } catch (e) {
                      setError(String(e));
                    } finally {
                      setBusy(false);
                    }
                  }}
                >
                  Delete all history
                </button>
              </div>
            </div>
          </div>
        ) : (
          <>
            <div>
              <strong>A clean slate</strong>
              <p>Remove all saved clips, including pins.</p>
            </div>
            <button
              className="button danger-outline"
              disabled={busy}
              onClick={() => setConfirm(true)}
            >
              <Trash2 size={14} />
              Clear history…
            </button>
          </>
        )}
      </div>
    </dialog>
  );
}
