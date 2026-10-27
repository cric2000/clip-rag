export const categories = [
  "Git",
  "Linux",
  "Docker",
  "Kubernetes",
  "SQL",
  "JSON",
  "URLs",
  "Code",
  "Text",
] as const;
export type Category = (typeof categories)[number];
export interface Entry {
  id: number;
  content: string;
  hash: string;
  category: Category;
  firstCopied: number;
  lastCopied: number;
  copyCount: number;
  restoreCount: number;
  pinned: boolean;
}
export interface Query {
  search: string;
  category: string;
  pinned: boolean;
  sort: "newest" | "used" | "alpha";
  limit: number;
  offset: number;
}
export interface Page {
  entries: Entry[];
  total: number;
}
export interface Settings {
  monitoring: boolean;
  maxEntries: number;
  retentionDays: number;
  maxEntryBytes: number;
}
export interface State {
  monitoring: boolean;
  clipboard: {
    supported: boolean;
    healthy: boolean;
    backend: string;
    detail: string;
  };
  error: string;
  dataDir: string;
}
