// Staged, expand-only migrations [D-10].
//
// schema_meta holds schema_version and min_reader_version. A migration only adds
// tables, nullable or defaulted columns, or indexes that current data satisfies.
// Nothing is dropped, renamed or retyped. An instance whose code version is below
// min_reader_version refuses to start.
//
// v3 is forward-only (plan 1.3.8): it raises min_reader_version to 3, and it migrates only
// while this connection is the only one open on the file, after a written and verified
// backup (backup.ts). Rollback is a restore from that backup, never a v2 reading a v3 file.
import type Database from "better-sqlite3";
import {
  acquireHold,
  backupBeforeMigrate,
  releaseHold,
  type BackupResult,
  type BackupTestSeams,
} from "./backup.js";

export { BackupNotVerifiedError, QuiesceRequiredError } from "./backup.js";

/** The schema version this code writes and the highest it can read. */
export const CODE_VERSION = 4;

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

// Interim approvals (Task 1.13): a record of what mk approved, never an authorization. The
// `authorizing` column is pinned to 0 by CHECK; the events table cannot be edited.
const V2 = `
CREATE TABLE approvals (
  approval_id TEXT PRIMARY KEY,
  decision_id TEXT NOT NULL UNIQUE REFERENCES decisions(id),
  option_id TEXT NOT NULL,
  pick_id TEXT NOT NULL,
  account TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('merge','deploy','release')),
  target TEXT NOT NULL,
  identity TEXT NOT NULL,
  minted_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  authorizing INTEGER NOT NULL DEFAULT 0 CHECK (authorizing = 0)
);
CREATE INDEX approvals_tuple ON approvals(kind, target, identity);
CREATE TABLE approval_events (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  at TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('minted','checked','revoked')),
  approval_id TEXT,
  detail_json TEXT NOT NULL
);
CREATE TRIGGER approval_events_no_update BEFORE UPDATE ON approval_events
BEGIN SELECT RAISE(ABORT, 'approval_events is append-only'); END;
CREATE TRIGGER approval_events_no_delete BEFORE DELETE ON approval_events
BEGIN SELECT RAISE(ABORT, 'approval_events is append-only'); END;
`;

// Home S1 on bb tasks (plan 1.1-1.3, Task 2.3). Stored asks stay v1; card generations are rows of
// `decisions` with source = 'card'. Every immutability rule is a trigger, so no code path (a stale
// build included) can bypass it.
const V3 = `
CREATE TABLE cards (
  task_id TEXT PRIMARY KEY,
  project_id TEXT,
  card_key TEXT,
  title TEXT,
  request_key TEXT,
  request_identity TEXT,
  asking_thread TEXT,
  routing_mode TEXT CHECK (routing_mode IN ('thread','pull')),
  routed_thread TEXT,
  routing_check_at TEXT,
  state TEXT NOT NULL DEFAULT 'observed' CHECK (state IN ('observed','display','open','ruled','closed')),
  display_reason TEXT,
  root_state TEXT,
  root_reason TEXT,
  changed_after_ruling INTEGER NOT NULL DEFAULT 0,
  home_unlabelled_at TEXT,
  next_check_at TEXT,
  check_attempts INTEGER NOT NULL DEFAULT 0,
  created_at TEXT,
  updated_at TEXT,
  status TEXT,
  labelled INTEGER,
  blocks_json TEXT,
  first_seen_at TEXT,
  last_seen_at TEXT,
  deleted_at TEXT
);
CREATE INDEX cards_state ON cards(state, next_check_at);
CREATE TRIGGER cards_routing_frozen BEFORE UPDATE OF routing_mode, routed_thread ON cards
WHEN (OLD.routing_mode IS NOT NULL AND NEW.routing_mode IS NOT OLD.routing_mode)
  OR (OLD.routed_thread IS NOT NULL AND NEW.routed_thread IS NOT OLD.routed_thread)
BEGIN SELECT RAISE(ABORT, 'cards routing is frozen at first materialization'); END;

CREATE TABLE card_requests (
  request_key TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  identity TEXT NOT NULL,
  registered_at TEXT NOT NULL
);
CREATE TRIGGER card_requests_no_update BEFORE UPDATE ON card_requests
BEGIN SELECT RAISE(ABORT, 'card_requests is insert-only'); END;
CREATE TRIGGER card_requests_no_delete BEFORE DELETE ON card_requests
BEGIN SELECT RAISE(ABORT, 'card_requests is insert-only'); END;

ALTER TABLE decisions ADD COLUMN source TEXT NOT NULL DEFAULT 'home' CHECK (source IN ('home','card'));
ALTER TABLE decisions ADD COLUMN task_id TEXT;
ALTER TABLE decisions ADD COLUMN generation INTEGER;
ALTER TABLE decisions ADD COLUMN tasks_project_id TEXT;
ALTER TABLE decisions ADD COLUMN card_fp TEXT;
CREATE UNIQUE INDEX decisions_task_generation ON decisions(task_id, generation) WHERE task_id IS NOT NULL;

CREATE TRIGGER decisions_ask_immutable BEFORE UPDATE OF body_json, revision, identity, subject, semantic_key, card_fp ON decisions
WHEN OLD.body_json IS NOT NEW.body_json OR OLD.revision IS NOT NEW.revision OR OLD.identity IS NOT NEW.identity
  OR OLD.subject IS NOT NEW.subject OR OLD.semantic_key IS NOT NEW.semantic_key OR OLD.card_fp IS NOT NEW.card_fp
BEGIN SELECT RAISE(ABORT, 'decisions ask is immutable'); END;

CREATE TRIGGER decisions_card_link BEFORE UPDATE OF source, task_id, generation, tasks_project_id ON decisions
WHEN OLD.source IS NOT NEW.source
  OR OLD.task_id IS NOT NEW.task_id
  OR OLD.generation IS NOT NEW.generation
  OR OLD.tasks_project_id IS NOT NEW.tasks_project_id
BEGIN SELECT RAISE(ABORT, 'decisions card link: columns are fixed at insert'); END;

CREATE TRIGGER decisions_home_insert BEFORE INSERT ON decisions
WHEN NEW.source = 'home' AND (NEW.task_id IS NOT NULL OR NEW.generation IS NOT NULL OR NEW.tasks_project_id IS NOT NULL)
BEGIN SELECT RAISE(ABORT, 'decisions card link: a home row never carries a link column'); END;

CREATE TABLE decision_blocks (
  decision_id TEXT NOT NULL REFERENCES decisions(id),
  ref TEXT NOT NULL,
  PRIMARY KEY (decision_id, ref)
);
CREATE TRIGGER decision_blocks_no_update BEFORE UPDATE ON decision_blocks
BEGIN SELECT RAISE(ABORT, 'decision_blocks is insert-only'); END;
CREATE TRIGGER decision_blocks_no_delete BEFORE DELETE ON decision_blocks
BEGIN SELECT RAISE(ABORT, 'decision_blocks is insert-only'); END;

CREATE TABLE card_writes (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  decision_id TEXT NOT NULL REFERENCES decisions(id),
  kind TEXT NOT NULL CHECK (kind IN ('comment','unlabel','relabel')),
  payload TEXT,
  state TEXT NOT NULL DEFAULT 'pending',
  attempt INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  next_try_at TEXT,
  updated_at TEXT,
  UNIQUE (decision_id, kind)
);
CREATE INDEX card_writes_due ON card_writes(state, next_try_at);

CREATE TABLE project_bindings (
  tasks_project_id TEXT PRIMARY KEY,
  home_project TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('suggested','confirmed','rejected')),
  suggested_at TEXT,
  confirmed_at TEXT
);

-- Written by migrate() in the same transaction as the DDL (plan 1.3.8 step 7).
CREATE TABLE migration_log (
  version INTEGER PRIMARY KEY,
  at TEXT NOT NULL,
  backup_path TEXT,
  digest TEXT,
  table_counts_json TEXT
);
`;


// Home Your move (plan mk-okek.24). Expand-only: no min_reader bump, so a v3 reader still opens a v4
// database (it ignores `moves`). A move is one thing mk owes, keyed by (task ULID, generation).
const V4 = `
CREATE TABLE moves (
  task_id TEXT NOT NULL,
  generation INTEGER NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('script','pr','read','context')),
  payload_json TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('open','claimed','closed')),
  opened_by TEXT NOT NULL CHECK (opened_by IN ('card','pick')),
  opened_at TEXT NOT NULL,
  claimed_at TEXT,
  claimed_by TEXT,
  closed_at TEXT,
  closed_by TEXT,
  evidence TEXT,
  -- What the script itself reported: kept apart from what mk said he did. Reported, never verified.
  report_state TEXT CHECK (report_state IS NULL OR report_state IN ('succeeded','failed','no-report')),
  report_json TEXT,
  report_at TEXT,
  report_deadline_at TEXT,
  skipped_at TEXT,
  hidden_by TEXT,
  hidden_at TEXT,
  last_checked_at TEXT,
  last_error TEXT,
  PRIMARY KEY (task_id, generation)
);
CREATE INDEX moves_state ON moves(state, opened_at);
-- Closing the tasks card of a move Home filed (label mk-move) after the move closes. One row per move.
CREATE TABLE move_closes (
  task_id TEXT NOT NULL,
  generation INTEGER NOT NULL,
  state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','done','skipped')),
  attempt INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  next_try_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (task_id, generation)
);
-- Conversation on the card. A read-only mirror of the tasks card's comments: display only, never an input
-- to any decision. author_id is the id the tasks plugin recorded (thread id, or null); the author class is derived
-- from it, never from the text.
CREATE TABLE card_comments (
  task_id TEXT NOT NULL,
  comment_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  author_name TEXT NOT NULL,
  author_id TEXT,
  body TEXT NOT NULL,
  created_at TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (task_id, comment_id)
);
CREATE INDEX card_comments_task ON card_comments(task_id, created_at, comment_id);
-- When the comments of a card were last read, and whether the last read failed ("comments may be stale").
CREATE TABLE card_comment_polls (
  task_id TEXT PRIMARY KEY,
  last_ok_at TEXT,
  last_attempt_at TEXT NOT NULL,
  last_error TEXT
);
-- How far mk has read a card's conversation (unread dots). One row per card.
CREATE TABLE card_seen (
  task_id TEXT PRIMARY KEY,
  seen_through TEXT NOT NULL,
  seen_at TEXT NOT NULL
);
`;

export const MIGRATIONS: readonly Migration[] = [
  { version: 1, sql: V1 },
  { version: 2, sql: V2 },
  { version: 3, minReaderVersion: 3, sql: V3 },
  { version: 4, sql: V4 },
];

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
  /** Receives the one-line outcome (`bb.log`). */
  log?: { info(msg: string): void };
  /** Clock for the backup file name and migration_log.at. */
  now?: () => Date;
  /** Test-only seams (see BackupTestSeams). */
  test?: BackupTestSeams;
}

/**
 * Bring the database up to this code's schema. Throws SchemaTooNewError when the database
 * demands a newer reader than this code (before anything is written), QuiesceRequiredError when
 * another connection holds the file, and BackupNotVerifiedError when the pre-migration backup
 * fails any check. When a migration is pending, the DDL runs in one IMMEDIATE transaction while
 * this connection holds the file exclusively, after a verified backup of a schema 1+ database.
 */
export function migrate(db: Database.Database, opts: MigrateOptions = {}): SchemaState {
  const codeVersion = opts.codeVersion ?? CODE_VERSION;
  const migrations = [...(opts.migrations ?? MIGRATIONS)].sort((a, b) => a.version - b.version);
  const clock = opts.now ?? (() => new Date());
  const hook = opts.test?.hook ?? (() => {});

  const pre = readSchemaState(db);
  if (pre.minReaderVersion > codeVersion) throw new SchemaTooNewError(pre.minReaderVersion, codeVersion);
  const pending = migrations.some((m) => m.version > pre.schemaVersion && m.version <= codeVersion);

  const run = (backup: BackupResult | null): SchemaState =>
    db
      .transaction((): SchemaState => {
        db.exec(META);
        const state = readSchemaState(db);
        if (state.minReaderVersion > codeVersion) {
          throw new SchemaTooNewError(state.minReaderVersion, codeVersion);
        }
        let { schemaVersion, minReaderVersion } = state;
        const applied: number[] = [];
        for (const m of migrations) {
          if (m.version <= schemaVersion || m.version > codeVersion) continue;
          db.exec(m.sql);
          applied.push(m.version);
          schemaVersion = m.version;
          minReaderVersion = Math.max(minReaderVersion, m.minReaderVersion ?? 0);
        }
        const hasLog = db.prepare("SELECT 1 FROM sqlite_master WHERE type='table' AND name='migration_log'").get();
        if (hasLog) {
          const ins = db.prepare(
            "INSERT INTO migration_log(version, at, backup_path, digest, table_counts_json) VALUES (?, ?, ?, ?, ?)",
          );
          for (const v of applied.filter((v) => v >= 3)) {
            ins.run(v, clock().toISOString(), backup?.path ?? null, backup?.digest ?? null, backup ? JSON.stringify(backup.tableCounts) : null);
          }
        }
        const put = db.prepare(
          "INSERT INTO schema_meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
        );
        put.run("schema_version", schemaVersion);
        put.run("min_reader_version", minReaderVersion);
        hook("ddl-applied", {});
        return { schemaVersion, minReaderVersion };
      })
      // IMMEDIATE takes the write lock up front so two instances cannot both migrate.
      .immediate();

  if (!pending) return run(null);

  const hold = opts.test?.skipHold ? null : acquireHold(db);
  try {
    const backup = pre.schemaVersion >= 1 ? backupBeforeMigrate(db, { now: clock, test: opts.test }) : null;
    const state = run(backup);
    hook("committed", {});
    opts.log?.info(
      backup
        ? `autarch: schema ${pre.schemaVersion} → ${state.schemaVersion}; backup ${backup.path} verified (integrity ok, digest ${backup.digest.slice(0, 12)}, ${backup.tables} tables, ${backup.rows} rows)`
        : `autarch: schema ${pre.schemaVersion} → ${state.schemaVersion}; no backup (fresh database)`,
    );
    return state;
  } finally {
    releaseHold(db, hold);
  }
}
