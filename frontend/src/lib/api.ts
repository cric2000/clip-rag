import { Call, Events } from "@wailsio/runtime";
import type { Page, Query, Settings, State } from "../types";
const call = <T>(name: string, ...args: unknown[]): Promise<T> =>
  Call.ByName(`cliprag/internal/app.Service.${name}`, ...args);
export const api = {
  list: (q: Query) => call<Page>("List", q),
  counts: () => call<Record<string, number>>("Counts"),
  status: () => call<State>("Status"),
  settings: () => call<Settings>("GetSettings"),
  save: (s: Settings) => call<void>("SaveSettings", s),
  copy: (id: number) => call<void>("Copy", id),
  pin: (id: number, pinned: boolean) => call<void>("Pin", id, pinned),
  delete: (id: number) => call<void>("Delete", id),
  clear: () => call<void>("Clear", "CLEAR"),
  hide: () => Call.ByName("main.Desktop.Hide"),
  quit: () => Call.ByName("main.Desktop.Quit"),
  subscribe: (fn: () => void) => Events.On("history:changed", fn),
};
