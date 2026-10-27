import { useCallback, useEffect, useRef, useState } from "react";
import {
  Search,
  Pause,
  Play,
  ArrowDownWideNarrow,
  ShieldCheck,
  RefreshCw,
  ChevronRight,
  ChevronDown,
  X,
  Pin,
  Check,
  Archive,
  ArrowLeft,
  ArrowRight,
  AlertCircle,
  Settings as SettingsIcon,
  MoreHorizontal,
  EyeOff,
  Power,
} from "lucide-react";
import { api } from "./lib/api";
import { categoryIcons, categoryClass } from "./lib/categories";
import { BrandMark } from "./components/layout/BrandMark";
import { HistoryEntry } from "./components/history/HistoryEntry";
import { SettingsDialog } from "./components/settings/SettingsDialog";

import {
  categories,
  type Category,
  type Page,
  type Settings,
  type State,
  type Query,
} from "./types";

export default function App() {
  const [category, setCategory] = useState<Category | "">("");
  const [search, setSearch] = useState("");
  const [debounced, setDebounced] = useState("");
  const [sort, setSort] = useState<Query["sort"]>("newest");
  const [pinned, setPinned] = useState(false);
  const [offset, setOffset] = useState(0);
  const [page, setPage] = useState<Page>({ entries: [], total: 0 });
  const [counts, setCounts] = useState<Record<string, number>>({});
  const [state, setState] = useState<State>();
  const [settings, setSettings] = useState<Settings>();
  const [showSettings, setShowSettings] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const searchRef = useRef<HTMLInputElement>(null);
  const generation = useRef(0);
  useEffect(() => {
    const t = setTimeout(() => {
      setDebounced(search);
      setOffset(0);
    }, 180);
    return () => clearTimeout(t);
  }, [search]);
  const refresh = useCallback(async () => {
    const id = ++generation.current;
    setLoading(true);
    try {
      const [p, c, s, cfg] = await Promise.all([
        api.list({
          search: debounced,
          category,
          pinned,
          sort,
          limit: 50,
          offset,
        }),
        api.counts(),
        api.status(),
        api.settings(),
      ]);
      if (id !== generation.current) return;
      if (p.total > 0 && offset >= p.total) {
        setOffset(Math.floor((p.total - 1) / 50) * 50);
        return;
      }
      setPage(p);
      setCounts(c);
      setState(s);
      setSettings(cfg);
      setError("");
    } catch (e) {
      if (id === generation.current)
        setError(`Could not load your clipboard: ${String(e)}`);
    } finally {
      if (id === generation.current) setLoading(false);
    }
  }, [category, debounced, pinned, sort, offset]);
  useEffect(() => {
    void refresh();
    const requestGeneration = generation;
    const off = api.subscribe(() => void refresh());
    return () => {
      requestGeneration.current++;
      off();
    };
  }, [refresh]);
  // Refresh connection metadata only; clipboard content is never polled.
  useEffect(() => {
    const t = setInterval(() => {
      void api
        .status()
        .then(setState)
        .catch(() => {});
    }, 5000);
    return () => clearInterval(t);
  }, []);
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (showSettings) return;
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        searchRef.current?.focus();
      }
      if (e.key === "Escape" && document.activeElement === searchRef.current)
        setSearch("");
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [showSettings]);
  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(""), 2600);
    return () => clearTimeout(t);
  }, [notice]);
  async function action(
    work: () => Promise<unknown>,
    message: string,
  ): Promise<boolean> {
    try {
      await work();
      setNotice(message);
      setError("");
      await refresh();
      return true;
    } catch (e) {
      setError(String(e));
      return false;
    }
  }
  function reset() {
    setSearch("");
    setDebounced("");
    setPinned(false);
    setCategory("");
    setSort("newest");
    setOffset(0);
  }
  function selectTab(value: boolean) {
    setPinned(value);
    setOffset(0);
  }
  function toggleMonitoring() {
    if (settings)
      void action(
        () => api.save({ ...settings, monitoring: !settings.monitoring }),
        settings.monitoring ? "Monitoring paused" : "Monitoring resumed",
      );
  }
  const healthy = state?.clipboard.healthy;
  const active = healthy && state?.monitoring;
  const ready = !!settings;
  const isFiltered = !!search || !!category || pinned;
  const historyTitle = search
    ? "Search results"
    : pinned
      ? "Pinned clips"
      : "Recent clips";
  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="topbar-inner">
          <div className="brand">
            <BrandMark size={38} />
            <h1>ClipRAG</h1>
            <span className="brand-caption">Clipboard memory</span>
          </div>
          <div className="header-actions">
            <span
              className={`status ${active ? "active" : healthy ? "paused" : "offline"}`}
            >
              <i />
              {!state
                ? "Connecting…"
                : active
                  ? "Monitoring"
                  : !healthy
                    ? "Bridge unavailable"
                    : "Paused"}
            </span>
            <button
              className="button compact"
              disabled={!settings}
              onClick={toggleMonitoring}
            >
              {settings?.monitoring ? <Pause size={16} /> : <Play size={16} />}
              <span>{settings?.monitoring ? "Pause" : "Resume"}</span>
            </button>
            <span className="header-divider" />
            <button
              className="icon-button"
              aria-label="Settings"
              title="Settings"
              disabled={!ready}
              onClick={() => setShowSettings(true)}
            >
              <SettingsIcon size={20} />
            </button>
            <details
              className="app-menu"
              onBlur={(e) => {
                if (!e.currentTarget.contains(e.relatedTarget))
                  e.currentTarget.open = false;
              }}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  e.currentTarget.open = false;
                  e.currentTarget.querySelector("summary")?.focus();
                }
              }}
            >
              <summary
                className="icon-button"
                aria-label="More actions"
                title="More actions"
              >
                <MoreHorizontal size={21} />
              </summary>
              <div className="app-menu-items">
                <button onClick={() => void action(api.hide, "Window hidden")}>
                  <EyeOff size={17} />
                  <span>
                    Hide window
                    <small>Capture continues in the background</small>
                  </span>
                </button>
                <button
                  onClick={() =>
                    void api.quit().catch((e) => setError(String(e)))
                  }
                >
                  <Power size={17} />
                  <span>
                    Quit ClipRAG<small>Stop capturing and close the app</small>
                  </span>
                </button>
              </div>
            </details>
          </div>
        </div>
      </header>
      <main className="workspace-main">
        <div className="page-intro">
          <div>
            <h2>Your clipboard history</h2>
            <p>Find it. Copy it. Keep moving.</p>
          </div>
          <span className="local-label">
            <ShieldCheck size={16} />
            Only on this device
          </span>
        </div>
        <div className="search-toolbar">
          <div className="search-wrap">
            <Search size={19} />
            <input
              ref={searchRef}
              aria-label="Search clipboard history"
              placeholder="Search anything you’ve copied…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            {search ? (
              <button
                className="icon-button search-clear"
                aria-label="Clear search"
                title="Clear search · Esc"
                onClick={() => setSearch("")}
              >
                <X size={16} />
              </button>
            ) : (
              <kbd>
                <span>ctrl</span>K
              </kbd>
            )}
          </div>
        </div>
        <nav className="category-cards" aria-label="Filter by type">
          <button
            className={`category-card ${!category ? "category-selected" : ""}`}
            data-category=""
            aria-label="Show all types"
            aria-pressed={!category}
            onClick={() => {
              setCategory("");
              setOffset(0);
            }}
          >
            <span className="category-card-top">
              <Archive size={19} />
              <span>
                {ready ? (counts["All History"] ?? 0).toLocaleString() : "—"}
              </span>
            </span>
            <span>All types</span>
          </button>
          {categories.map((name) => {
            const Icon = categoryIcons[name];
            return (
              <button
                key={name}
                className={`category-card ${categoryClass(name)} ${category === name ? "category-selected" : ""}`}
                data-category={name}
                aria-label={`Filter by ${name}`}
                aria-pressed={category === name}
                onClick={() => {
                  setCategory(category === name ? "" : name);
                  setOffset(0);
                }}
              >
                <span className="category-card-top">
                  <Icon size={19} />
                  <span>
                    {ready ? (counts[name] ?? 0).toLocaleString() : "—"}
                  </span>
                </span>
                <span>{name}</span>
              </button>
            );
          })}
        </nav>
        {state && !healthy && (
          <div className="bridge-warning" role="status">
            <AlertCircle size={18} />
            <div>
              <strong>Let’s connect your clipboard.</strong>
              <p>{state.clipboard.detail}</p>
            </div>
          </div>
        )}
        {healthy && !state?.monitoring && (
          <div className="pause-banner">
            <Pause size={15} />
            <span>Capture is paused. Your saved clips are still here.</span>
            <button className="text-button" onClick={toggleMonitoring}>
              Resume capture
              <ChevronRight size={13} />
            </button>
          </div>
        )}
        {state?.error && (
          <div role="alert" className="error-banner">
            <AlertCircle size={17} />
            <span>{state.error}</span>
          </div>
        )}
        {error && (
          <div role="alert" className="error-banner">
            <AlertCircle size={17} />
            <span>{error}</span>
            <button className="text-button" onClick={() => void refresh()}>
              <RefreshCw size={14} />
              Retry
            </button>
          </div>
        )}
        <div className="library-toolbar">
          <div
            className="library-tabs"
            role="tablist"
            aria-label="Clipboard views"
            onKeyDown={(e) => {
              const tabs = Array.from(
                e.currentTarget.querySelectorAll<HTMLButtonElement>(
                  "[role=tab]",
                ),
              );
              const index = tabs.indexOf(
                document.activeElement as HTMLButtonElement,
              );
              let next: number;
              if (e.key === "ArrowRight" || e.key === "ArrowLeft")
                next =
                  (index + (e.key === "ArrowRight" ? 1 : -1) + tabs.length) %
                  tabs.length;
              else if (e.key === "Home") next = 0;
              else if (e.key === "End") next = tabs.length - 1;
              else return;
              e.preventDefault();
              tabs[next].focus();
              tabs[next].click();
            }}
          >
            <button
              role="tab"
              id="all-clips-tab"
              aria-controls="clip-panel"
              aria-selected={!pinned}
              tabIndex={pinned ? -1 : 0}
              onClick={() => selectTab(false)}
            >
              <Archive size={17} />
              All clips
              <span className="tab-count">
                {ready ? (counts["All History"] ?? 0).toLocaleString() : "—"}
              </span>
            </button>
            <button
              role="tab"
              id="pinned-clips-tab"
              aria-controls="clip-panel"
              aria-selected={pinned}
              tabIndex={pinned ? 0 : -1}
              onClick={() => selectTab(true)}
            >
              <Pin size={17} />
              Pinned
              <span className="tab-count">
                {ready ? (counts.Pinned ?? 0).toLocaleString() : "—"}
              </span>
            </button>
          </div>
          <label
            className="sort-control"
            title="Most used ranks clips by how often you copy them in other apps"
          >
            <ArrowDownWideNarrow size={16} />
            <select
              aria-label="Sort history"
              value={sort}
              onChange={(e) => {
                setSort(e.target.value as Query["sort"]);
                setOffset(0);
              }}
            >
              <option value="newest">Newest first</option>
              <option value="used">Most used</option>
              <option value="alpha">Alphabetical</option>
            </select>
            <ChevronDown size={14} aria-hidden="true" />
          </label>
        </div>
        <section
          className="history-section"
          id="clip-panel"
          role="tabpanel"
          aria-labelledby={pinned ? "pinned-clips-tab" : "all-clips-tab"}
          tabIndex={0}
        >
          <div className="history-heading">
            <div className="history-title">
              <h3>
                {historyTitle}
                {category ? ` · ${category}` : ""}
              </h3>
              <span className="result-count">
                {page.total.toLocaleString()}{" "}
                {page.total === 1 ? "clip" : "clips"}
              </span>
              {loading && ready && (
                <RefreshCw
                  size={14}
                  className="spinner updating"
                  aria-label="Updating history"
                />
              )}
            </div>
            {isFiltered && (
              <button className="text-button" onClick={reset}>
                Clear filters
                <X size={14} />
              </button>
            )}
          </div>
          <div aria-busy={loading} className="entry-list">
            {loading && !ready ? (
              <div className="loading-state" role="status">
                <RefreshCw size={20} className="spinner" />
                <span>Opening your clipboard…</span>
              </div>
            ) : page.entries.length ? (
              page.entries.map((entry) => (
                <HistoryEntry
                  key={entry.id}
                  entry={entry}
                  onCopy={() =>
                    action(() => api.copy(entry.id), "Copied to clipboard")
                  }
                  onPin={() =>
                    action(
                      () => api.pin(entry.id, !entry.pinned),
                      entry.pinned ? "Entry unpinned" : "Entry pinned",
                    )
                  }
                  onDelete={() =>
                    action(() => api.delete(entry.id), "Entry deleted")
                  }
                />
              ))
            ) : (
              <div className="empty-state">
                <div className="empty-illustration">
                  <span className="empty-sheet sheet-back" />
                  <span className="empty-sheet sheet-front">
                    {isFiltered ? <Search size={29} /> : <Archive size={29} />}
                  </span>
                  <i />
                </div>
                <span className="empty-eyebrow">
                  {isFiltered ? "NOTHING HERE YET" : "A FRESH START"}
                </span>
                <h3>
                  {isFiltered
                    ? "No matching clips."
                    : "Your next copy starts here."}
                </h3>
                <p>
                  {isFiltered
                    ? "Try a different search or open another category. Your saved clips are one click away."
                    : active
                      ? "Copy a command, snippet, or link in any app. We’ll keep it here for the next time you need it."
                      : "Once capture is connected and resumed, text you copy will appear here automatically."}
                </p>
                {isFiltered ? (
                  <button className="button" onClick={reset}>
                    <X size={14} />
                    Show all clips
                  </button>
                ) : (
                  <span className="empty-hint">
                    <ShieldCheck size={13} />
                    Saved locally. Always yours.
                  </span>
                )}
              </div>
            )}
          </div>
          {page.total > 50 && (
            <div className="pagination">
              <span>
                Showing{" "}
                <strong>
                  {offset + 1}–{Math.min(offset + 50, page.total)}
                </strong>{" "}
                of {page.total.toLocaleString()}
              </span>
              <div>
                <button
                  className="button"
                  disabled={offset === 0 || loading}
                  onClick={() => setOffset(Math.max(0, offset - 50))}
                >
                  <ArrowLeft size={13} />
                  Previous
                </button>
                <button
                  className="button"
                  disabled={offset + 50 >= page.total || loading}
                  onClick={() => setOffset(offset + 50)}
                >
                  Next
                  <ArrowRight size={13} />
                </button>
              </div>
            </div>
          )}
        </section>
        <footer className="workspace-footer">
          <span>
            <ShieldCheck size={14} />
            Your clips stay on this device.
          </span>
          <span>
            <kbd>Ctrl K</kbd> to search <span className="footer-dot">·</span>
            <kbd>Esc</kbd> to clear
          </span>
        </footer>
      </main>
      {notice && (
        <div role="status" className="toast">
          <span>
            <Check size={15} />
          </span>
          {notice}
        </div>
      )}
      {showSettings && settings && (
        <SettingsDialog
          initial={settings}
          dataDir={state?.dataDir ?? ""}
          onClose={() => setShowSettings(false)}
          onSave={async (s) => {
            await api.save(s);
            await refresh();
            setNotice("Settings saved");
          }}
          onClear={async () => {
            await api.clear();
            await refresh();
            setNotice("History cleared");
          }}
        />
      )}
    </div>
  );
}
