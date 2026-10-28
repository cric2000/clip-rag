import { useEffect, useRef, useState } from "react";
import {
  Copy,
  Check,
  Pin,
  Trash2,
  ChevronDown,
  ChevronUp,
  Clock3,
  RotateCcw,
  LoaderCircle,
  Terminal,
} from "lucide-react";
import type { Entry } from "../../types";
import { categoryClass, categoryIcons } from "../../lib/categories";
function relative(ms: number) {
  const minutes = Math.max(0, Math.floor((Date.now() - ms) / 60000));
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  if (minutes < 1440) return `${Math.floor(minutes / 60)}h ago`;
  return `${Math.floor(minutes / 1440)}d ago`;
}
export function HistoryEntry({
  entry,
  onCopy,
  onPin,
  onDelete,
}: {
  entry: Entry;
  onCopy: () => Promise<boolean>;
  onPin: () => Promise<boolean>;
  onDelete: () => Promise<boolean>;
}) {
  const [expanded, setExpanded] = useState(false);
  const [copied, setCopied] = useState(false);
  const [pending, setPending] = useState<"copy" | "pin" | "delete" | null>(
    null,
  );
  const copyTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );
  useEffect(() => () => clearTimeout(copyTimer.current), []);
  const large = entry.content.length > 180 || entry.content.includes("\n");
  const Icon = categoryIcons[entry.category] ?? categoryIcons.Text;
  async function act(
    type: "copy" | "pin" | "delete",
    work: () => Promise<boolean>,
  ) {
    setPending(type);
    if (type === "copy") {
      setCopied(false);
      clearTimeout(copyTimer.current);
    }
    try {
      const success = await work();
      if (success && type === "copy") {
        setCopied(true);
        clearTimeout(copyTimer.current);
        copyTimer.current = setTimeout(() => setCopied(false), 1800);
      }
    } finally {
      setPending(null);
    }
  }
  return (
    <article
      className={`entry ${categoryClass(entry.category)} ${entry.pinned ? "entry-pinned" : ""}`}
    >
      <span className="entry-type-icon">
        <Icon size={20} strokeWidth={1.7} />
      </span>
      <div className="entry-content">
        <div className="entry-meta">
          <span className="badge">{entry.category}</span>
          {entry.pinned && (
            <span className="pinned-label">
              <Pin size={11} aria-label="Pinned entry" />
              Pinned
            </span>
          )}
          <span className="entry-time">
            <Clock3 size={11} />
            <time
              dateTime={new Date(entry.lastCopied).toISOString()}
              title={new Date(entry.lastCopied).toLocaleString()}
            >
              {relative(entry.lastCopied)}
            </time>
          </span>
        </div>
        <pre
          className={`${entry.category === "Text" ? "prose" : ""} ${expanded ? "expanded" : "preview"}`}
        >
          {entry.content}
        </pre>
        <div className="entry-foot">
          <span className="usage-count">
            <Copy size={11} />
            <span title="Times this text was copied in other apps">
              {entry.copyCount} external{" "}
              {entry.copyCount === 1 ? "copy" : "copies"}
            </span>
            <span className="separator">·</span>
            <RotateCcw size={11} />
            <span title="Times you used Copy on this saved clip">
              Reused {entry.restoreCount}{" "}
              {entry.restoreCount === 1 ? "time" : "times"}
            </span>
            {entry.autocompleteCount > 0 && (
              <>
                <span className="separator">·</span>
                <Terminal size={12} />
                <span title="Times accepted from terminal autocomplete. Acceptance does not mean the command was executed.">
                  Autocomplete uses {entry.autocompleteCount}
                </span>
              </>
            )}
          </span>
          {large && (
            <button
              className="text-button expand-button"
              aria-expanded={expanded}
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? "Collapse" : "View full content"}
              {expanded ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
            </button>
          )}
        </div>
      </div>
      <div className="entry-actions">
        <button
          className={`copy-button ${copied ? "copy-success" : ""}`}
          disabled={!!pending}
          title="Copy exact content"
          aria-label={`Copy ${entry.category} entry`}
          onClick={() => void act("copy", onCopy)}
        >
          {pending === "copy" ? (
            <LoaderCircle size={14} className="spinner" />
          ) : copied ? (
            <Check size={14} />
          ) : (
            <Copy size={14} />
          )}
          <span>{copied ? "Copied" : "Copy"}</span>
        </button>
        <div className="secondary-actions">
          <button
            className={`icon-button ${entry.pinned ? "is-pinned" : ""}`}
            disabled={!!pending}
            title={entry.pinned ? "Unpin" : "Pin"}
            aria-label={entry.pinned ? "Unpin entry" : "Pin entry"}
            onClick={() => void act("pin", onPin)}
          >
            <Pin size={15} />
          </button>
          <button
            className="icon-button danger"
            disabled={!!pending}
            title="Delete"
            aria-label="Delete entry"
            onClick={() => void act("delete", onDelete)}
          >
            <Trash2 size={15} />
          </button>
        </div>
      </div>
    </article>
  );
}
