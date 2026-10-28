package storage

const schemaV1 = `
CREATE TABLE entries (
 id INTEGER PRIMARY KEY,
 content TEXT NOT NULL,
 hash TEXT NOT NULL UNIQUE,
 category TEXT NOT NULL,
 first_copied INTEGER NOT NULL,
 last_copied INTEGER NOT NULL,
 copy_count INTEGER NOT NULL DEFAULT 1 CHECK(copy_count >= 1),
 restore_count INTEGER NOT NULL DEFAULT 0 CHECK(restore_count >= 0),
 pinned INTEGER NOT NULL DEFAULT 0 CHECK(pinned IN (0,1))
);
CREATE INDEX entries_recent ON entries(last_copied DESC, id DESC);
CREATE INDEX entries_category ON entries(category, last_copied DESC, id DESC);
CREATE INDEX entries_pinned ON entries(pinned, last_copied DESC, id DESC);
CREATE INDEX entries_used ON entries(copy_count DESC, last_copied DESC, id DESC);
CREATE TABLE settings (id INTEGER PRIMARY KEY CHECK(id=1), value TEXT NOT NULL);
PRAGMA user_version = 1;
`

const schemaV2 = `
ALTER TABLE entries ADD COLUMN autocomplete_count INTEGER NOT NULL DEFAULT 0 CHECK(autocomplete_count >= 0);
PRAGMA user_version = 2;
`
