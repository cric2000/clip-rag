import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "./App";
import { api } from "./lib/api";
import type { Entry, Settings, State } from "./types";
vi.mock("./lib/api", () => ({
  api: {
    list: vi.fn(),
    counts: vi.fn(),
    status: vi.fn(),
    settings: vi.fn(),
    save: vi.fn(),
    copy: vi.fn(),
    pin: vi.fn(),
    delete: vi.fn(),
    clear: vi.fn(),
    hide: vi.fn(),
    quit: vi.fn(),
    subscribe: vi.fn(() => () => {}),
  },
}));
const item: Entry = {
  id: 1,
  content: "git status",
  hash: "hash",
  category: "Git",
  firstCopied: Date.now(),
  lastCopied: Date.now(),
  copyCount: 5,
  restoreCount: 2,
  autocompleteCount: 3,
  pinned: false,
};
const cfg: Settings = {
  monitoring: true,
  maxEntries: 1000,
  retentionDays: 30,
  maxEntryBytes: 102400,
};
const state: State = {
  monitoring: true,
  clipboard: {
    supported: true,
    healthy: true,
    backend: "Test",
    detail: "Connected",
  },
  dataDir: "/test/cliprag",
  error: "",
};
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(api.list).mockResolvedValue({ entries: [item], total: 1 });
  vi.mocked(api.counts).mockResolvedValue({ "All History": 1, Git: 1 });
  vi.mocked(api.settings).mockResolvedValue(cfg);
  vi.mocked(api.status).mockResolvedValue(state);
  for (const fn of [api.copy, api.pin, api.delete, api.save, api.clear])
    vi.mocked(fn).mockResolvedValue(undefined);
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value: function (this: HTMLDialogElement) {
      this.setAttribute("open", "");
    },
  });
});
afterEach(cleanup);
async function ready() {
  render(<App />);
  await screen.findByText("git status");
}
describe("clipboard dashboard", () => {
  it("shows real counts and separates usage counters", async () => {
    await ready();
    expect(screen.getByText(/5 external copies/)).toBeVisible();
    expect(screen.getByText("Reused 2 times")).toHaveAttribute(
      "title",
      "Times you used Copy on this saved clip",
    );
    expect(screen.getByText("Monitoring")).toBeVisible();
    expect(screen.getByText("Autocomplete uses 3")).toHaveAttribute(
      "title",
      "Times accepted from terminal autocomplete. Acceptance does not mean the command was executed.",
    );
  });
  it("filters by type and sorts within that category", async () => {
    await ready();
    await userEvent.click(
      screen.getByRole("button", { name: "Filter by Docker" }),
    );
    await waitFor(() =>
      expect(api.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ category: "Docker" }),
      ),
    );
    await userEvent.selectOptions(
      screen.getByLabelText("Sort history"),
      "used",
    );
    await waitFor(() =>
      expect(api.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ sort: "used", category: "Docker" }),
      ),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Filter by Docker" }),
    );
    await waitFor(() =>
      expect(api.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ category: "", sort: "used" }),
      ),
    );
    expect(
      screen.getByRole("button", { name: "Show all types" }),
    ).toHaveAttribute("aria-pressed", "true");
  });
  it("combines pinned, type, and search without losing filters between tabs", async () => {
    await ready();
    await userEvent.click(
      screen.getByRole("button", { name: "Filter by SQL" }),
    );
    await userEvent.type(
      screen.getByRole("textbox", { name: "Search clipboard history" }),
      "users",
    );
    await userEvent.click(screen.getByRole("tab", { name: /Pinned/ }));
    await waitFor(() =>
      expect(api.list).toHaveBeenLastCalledWith(
        expect.objectContaining({
          category: "SQL",
          search: "users",
          pinned: true,
        }),
      ),
    );
    await userEvent.click(screen.getByRole("tab", { name: /All clips/ }));
    await waitFor(() =>
      expect(api.list).toHaveBeenLastCalledWith(
        expect.objectContaining({
          category: "SQL",
          search: "users",
          pinned: false,
        }),
      ),
    );
    expect(
      screen.getByRole("button", { name: "Filter by SQL" }),
    ).toHaveAttribute("aria-pressed", "true");
  });
  it("supports keyboard navigation between the two views", async () => {
    await ready();
    const all = screen.getByRole("tab", { name: /All clips/ });
    all.focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: /Pinned/ })).toHaveFocus();
    expect(screen.getByRole("tab", { name: /Pinned/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await waitFor(() =>
      expect(api.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ pinned: true }),
      ),
    );
    await userEvent.keyboard("{Home}");
    expect(all).toHaveFocus();
    expect(all).toHaveAttribute("aria-selected", "true");
  });
  it("debounces local search", async () => {
    await ready();
    await userEvent.type(
      screen.getByRole("textbox", { name: "Search clipboard history" }),
      "docker",
    );
    await waitFor(() =>
      expect(api.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ search: "docker" }),
      ),
    );
  });
  it("changes filters and sorting", async () => {
    await ready();
    await userEvent.click(screen.getByRole("tab", { name: /Pinned/ }));
    await waitFor(() =>
      expect(api.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ pinned: true }),
      ),
    );
    await userEvent.selectOptions(
      screen.getByLabelText("Sort history"),
      "alpha",
    );
    await waitFor(() =>
      expect(api.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ sort: "alpha" }),
      ),
    );
  });
  it("copies, pins, and deletes via the backend", async () => {
    await ready();
    await userEvent.click(screen.getByLabelText("Copy Git entry"));
    expect(api.copy).toHaveBeenCalledWith(1);
    await userEvent.click(screen.getByLabelText("Pin entry"));
    expect(api.pin).toHaveBeenCalledWith(1, true);
    await userEvent.click(screen.getByLabelText("Delete entry"));
    expect(api.delete).toHaveBeenCalledWith(1);
  });
  it("shows the empty state without invented data", async () => {
    vi.mocked(api.list).mockResolvedValue({ entries: [], total: 0 });
    render(<App />);
    expect(
      await screen.findByText("Your next copy starts here."),
    ).toBeVisible();
  });
  it("shows disconnected monitoring and copy errors", async () => {
    vi.mocked(api.status).mockResolvedValue({
      ...state,
      clipboard: { ...state.clipboard, healthy: false },
    });
    vi.mocked(api.copy).mockRejectedValue(new Error("bridge unavailable"));
    await ready();
    expect(screen.getByText("Bridge unavailable")).toBeVisible();
    await userEvent.click(screen.getByLabelText("Copy Git entry"));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "bridge unavailable",
    );
  });
  it("pauses monitoring", async () => {
    await ready();
    await userEvent.click(screen.getByRole("button", { name: "Pause" }));
    expect(api.save).toHaveBeenCalledWith({ ...cfg, monitoring: false });
  });
  it("edits settings and confirms clearing all history", async () => {
    await ready();
    await userEvent.click(screen.getByLabelText("Settings"));
    await userEvent.click(
      screen.getByText("Terminal autocomplete · Bash & Zsh"),
    );
    expect(screen.getByText("cliprag-shell install bash")).toBeVisible();
    fireEvent.change(screen.getByLabelText("Retention in days"), {
      target: { value: "14" },
    });
    await userEvent.click(
      screen.getByRole("button", { name: "Clear history…" }),
    );
    expect(api.clear).not.toHaveBeenCalled();
    await userEvent.click(
      screen.getByRole("button", { name: "Delete all history" }),
    );
    expect(api.clear).toHaveBeenCalled();
    await userEvent.click(
      screen.getByRole("button", { name: "Save settings" }),
    );
    expect(api.save).toHaveBeenCalledWith({ ...cfg, retentionDays: 14 });
  });
  it("expands multiline text safely", async () => {
    vi.mocked(api.list).mockResolvedValue({
      entries: [{ ...item, content: "<script>bad()</script>\nline 2" }],
      total: 1,
    });
    render(<App />);
    await screen.findByText("View full content");
    expect(document.querySelector("article script")).toBeNull();
    await userEvent.click(
      screen.getByRole("button", { name: "View full content" }),
    );
    expect(screen.getByRole("button", { name: "Collapse" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });
  it("confirms a successful copy on the entry itself", async () => {
    await ready();
    await userEvent.click(screen.getByLabelText("Copy Git entry"));
    await waitFor(() =>
      expect(screen.getByLabelText("Copy Git entry")).toHaveTextContent(
        "Copied",
      ),
    );
  });
  it("resets search and category filters from the empty view", async () => {
    vi.mocked(api.list).mockResolvedValue({ entries: [], total: 0 });
    render(<App />);
    await screen.findByText("Your next copy starts here.");
    await userEvent.type(
      screen.getByRole("textbox", { name: "Search clipboard history" }),
      "missing",
    );
    await screen.findByText("No matching clips.");
    await userEvent.click(
      screen.getByRole("button", { name: "Show all clips" }),
    );
    expect(
      screen.getByRole("textbox", { name: "Search clipboard history" }),
    ).toHaveValue("");
    await waitFor(() =>
      expect(api.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ search: "", category: "", pinned: false }),
      ),
    );
  });
  it("focuses search with Ctrl K and clears it with Escape", async () => {
    await ready();
    await userEvent.keyboard("{Control>}k{/Control}");
    const input = screen.getByRole("textbox", {
      name: "Search clipboard history",
    });
    expect(input).toHaveFocus();
    await userEvent.type(input, "docker");
    await userEvent.keyboard("{Escape}");
    expect(input).toHaveValue("");
  });
});
