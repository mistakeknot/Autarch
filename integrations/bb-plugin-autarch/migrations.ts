// Staged, expand-only migrations [D-10].
//
// schema_meta holds schema_version and min_reader_version. A migration only adds
// tables, nullable or defaulted columns, or indexes that current data satisfies.
// Nothing is dropped, renamed or retyped. min_reader_version rises only in a later
// release, after every running instance can read the new form. An instance whose
// code version is below min_reader_version refuses to start; an instance that is
// merely older than the schema keeps working, which is what lets Aleph keep the
// old instance after a failed candidate activation.
import type Database from "better-sqlite3";

/** The schema version this code writes and the highest it can read. */
export const CODE_VERSION = 1;

export interface Migration {
  version: number;
  /** Raised only when the new form is unreadable by older code. Expand-only means: rarely. */
  minReaderVersion?: number;
  sql: string;
}

export class SchemaTooNewError extends Error {
  readonly code = "schema-too-new";
  constructor(
    readonly minReaderVersion: number,
    readonly codeVersion: number,
  ) {
    super(
      `database requires reader version ${minReaderVersion}, this plugin is version ${codeVersion}; upgrade the plugin`,
    );
  }
}

const V1 = `
CREATE TABLE requests (
  request_id TEXT PRIMARY KEY,
  identity TEXT NOT NULL,
  result TEXT NOT NULL CHECK (result IN ('decision','mention')),
  decision_id TEXT NOT NULL,
  thread TEXT,
  at TEXT NOT NULL
);
CREATE TABLE decisions (
  id TEXT PRIMARY KEY,
  request_id TEXT NOT NULL UNIQUE,
  identity TEXT NOT NULL,
  revision TEXT NOT NULL,
  semantic_key TEXT NOT NULL,
  subject TEXT NOT NULL,
  kind TEXT NOT NULL,
  ask_key TEXT,
  project TEXT NOT NULL,
  project_root TEXT,
  root_dev TEXT,
  root_ino TEXT,
  asker TEXT NOT NULL,
  thread TEXT NOT NULL,
  owner_thread TEXT,
  body_json TEXT NOT NULL,
  supersedes TEXT UNIQUE REFERENCES decisions(id),
  delegable INTEGER NOT NULL DEFAULT 1,
  filed_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  withdrawn_at TEXT,
  resolved_at TEXT
);
CREATE INDEX decisions_thread ON decisions(thread);
CREATE INDEX decisions_semantic ON decisions(semantic_key);
CREATE TABLE picks (
  decision_id TEXT PRIMARY KEY REFERENCES decisions(id),
  pick_id TEXT NOT NULL UNIQUE,
  option_id TEXT NOT NULL,
  revision TEXT NOT NULL,
  "by" TEXT NOT NULL,
  surface TEXT NOT NULL CHECK (surface IN ('home','overlay','cli')),
  reason TEXT,
  picked_at TEXT NOT NULL,
  params_hash TEXT
);
CREATE TABLE obligations (
  id TEXT PRIMARY KEY,
  decision_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'pending'
    CHECK (state IN ('pending','sending','queued','uncertain','done','undeliverable','dismissed')),
  attempt INTEGER NOT NULL DEFAULT 0,
  recipient TEXT,
  payload TEXT,
  op TEXT UNIQUE,
  after_id TEXT REFERENCES obligations(id),
  voided_at TEXT,
  resend_permit INTEGER NOT NULL DEFAULT 0,
  resend_click TEXT,
  last_error TEXT,
  next_try_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX obligations_decision ON obligations(decision_id);
CREATE INDEX obligations_state ON obligations(state, next_try_at);
CREATE TABLE attempts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  obligation_id TEXT NOT NULL REFERENCES obligations(id),
  n INTEGER NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('sending','queued','delivered','not-delivered','uncertain')),
  handle TEXT,
  evidence TEXT,
  error TEXT,
  at TEXT NOT NULL,
  UNIQUE (obligation_id, n)
);
CREATE TABLE queue_events (
  queued_row TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  at TEXT NOT NULL
);
CREATE TABLE mentions (
  decision_id TEXT NOT NULL,
  thread TEXT NOT NULL,
  request_id TEXT NOT NULL,
  at TEXT NOT NULL,
  PRIMARY KEY (decision_id, thread)
);
CREATE TABLE events (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  at TEXT NOT NULL,
  type TEXT NOT NULL,
  decision_id TEXT,
  detail_json TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE seen (
  account TEXT NOT NULL,
  item_id TEXT NOT NULL,
  seen_at TEXT NOT NULL,
  PRIMARY KEY (account, item_id)
);
CREATE TABLE notes (
  id TEXT PRIMARY KEY,
  at TEXT NOT NULL,
  text TEXT NOT NULL,
  cites_json TEXT NOT NULL DEFAULT '[]'
);
CREATE TABLE settings_kv (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
INSERT INTO settings_kv(key, value) VALUES ('store_id', lower(hex(randomblob(16))));
`;

export const MIGRATIONS: readonly Migration[] = [{ version: 1, sql: V1 }];

const META = `CREATE TABLE IF NOT EXISTS schema_meta (key TEXT PRIMARY KEY, value INTEGER NOT NULL)`;

export interface SchemaState {
  schemaVersion: number;
  minReaderVersion: number;
}

export function readSchemaState(db: Database.Database): SchemaState {
  const exists = db
    .prepare("SELECT 1 FROM sqlite_master WHERE type='table' AND name='schema_meta'")
    .get();
  if (!exists) return { schemaVersion: 0, minReaderVersion: 0 };
  const rows = db.prepare("SELECT key, value FROM schema_meta").all() as {
    key: string;
    value: number;
  }[];
  const get = (k: string) => rows.find((r) => r.key === k)?.value ?? 0;
  return { schemaVersion: get("schema_version"), minReaderVersion: get("min_reader_version") };
}

export interface MigrateOptions {
  codeVersion?: number;
  migrations?: readonly Migration[];
}

/**
 * Bring the database up to this code's schema, in one transaction. Throws
 * SchemaTooNewError when the database demands a newer reader than this code.
 * A database already newer than the code (but still readable by it) is left alone.
 */
export function migrate(db: Database.Database, opts: MigrateOptions = {}): SchemaState {
  const codeVersion = opts.codeVersion ?? CODE_VERSION;
  const migrations = opts.migrations ?? MIGRATIONS;
  const run = db.transaction((): SchemaState => {
    db.exec(META);
    const state = readSchemaState(db);
    if (state.minReaderVersion > codeVersion) {
      throw new SchemaTooNewError(state.minReaderVersion, codeVersion);
    }
    let { schemaVersion, minReaderVersion } = state;
    for (const m of [...migrations].sort((a, b) => a.version - b.version)) {
      if (m.version <= schemaVersion || m.version > codeVersion) continue;
      db.exec(m.sql);
      schemaVersion = m.version;
      minReaderVersion = Math.max(minReaderVersion, m.minReaderVersion ?? 0);
    }
    const put = db.prepare(
      "INSERT INTO schema_meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
    );
    put.run("schema_version", schemaVersion);
    put.run("min_reader_version", minReaderVersion);
    return { schemaVersion, minReaderVersion };
  });
  // IMMEDIATE takes the write lock up front so two instances cannot both migrate.
  return run.immediate();
}
