// Quiesce and a verified backup before a migration (plan 1.3.8, conditions 1-2).
//
// Home never migrates a database that another connection can still write, and never
// migrates without a written, verified copy of the old one. The hold is SQLite's own
// exclusive lock on the plugin's own connection; the backup is `VACUUM INTO`, a SQL
// statement. Nothing here execs, spawns or shells out. The only file operations are an
// O_RDONLY open plus fsync of the backup file and its directory (the write-scope allowlist
// entry for this module, Task 2.7).
import Database from "better-sqlite3";
import { createHash } from "node:crypto";
import { closeSync, existsSync, fsyncSync, openSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

/** Stable machine-readable markers: they are in the error message and in the store log line, and scripts/home-upgrade-v3.bash greps for them. */
export const REFUSED_QUIESCE = "[home-refused:quiesce-required]";
export const REFUSED_BACKUP = "[home-refused:backup-not-verified]";

export class QuiesceRequiredError extends Error {
  readonly code = "quiesce-required";
  constructor(detail = "") {
    super(
      `${REFUSED_QUIESCE} another connection holds data.db: disable the autarch plugin, stop every reader, then enable${detail ? ` (${detail})` : ""}`,
    );
  }
}

export class BackupNotVerifiedError extends Error {
  readonly code = "backup-not-verified";
  constructor(
    readonly backupPath: string | null,
    readonly check: string,
    detail: string,
  ) {
    super(`${REFUSED_BACKUP} pre-migration backup not verified: ${check} failed${backupPath ? ` for ${backupPath}` : ""}: ${detail}`);
  }
}

/** Test-only seams. Production code never passes these (asserted by the Task 2.7 import-graph test). */
export interface BackupTestSeams {
  /** Called at "backup-written", "verified", "ddl-applied" and "committed". */
  hook?: (step: string, ctx: { backupPath?: string }) => void;
  /** Skip the exclusive hold, so a test can write between the backup and its verification. */
  skipHold?: boolean;
}

export interface DigestResult {
  digest: string;
  tables: number;
  rows: number;
  tableCounts: Record<string, number>;
}

const errCode = (e: unknown) => (e as { code?: string })?.code ?? "";
const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));
const isBusy = (e: unknown) => /^SQLITE_(BUSY|LOCKED)/.test(errCode(e)) || /database is locked/i.test(errMsg(e));

/**
 * SHA-256 over every table in `sqlite_master` (type table, excluding sqlite_%), in name order:
 * the table name, its CREATE SQL, and every row as quote() of each column in declared order,
 * rows ordered by rowid (by primary key for a WITHOUT ROWID table). Counts alone would pass an
 * UPDATE, or a delete and insert that keeps the count.
 */
export function contentDigest(db: Database.Database): DigestResult {
  const h = createHash("sha256");
  const feed = (s: string) => h.update(`${Buffer.byteLength(s)}:${s}\n`);
  const tableCounts: Record<string, number> = {};
  let rows = 0;
  const run = db.transaction(() => {
    const tables = db
      .prepare("SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
      .all() as { name: string; sql: string }[];
    for (const t of tables) {
      feed(t.name);
      feed(t.sql);
      const info = db.pragma(`table_info("${t.name.replace(/"/g, '""')}")`) as { name: string; pk: number }[];
      const q = (c: string) => `"${c.replace(/"/g, '""')}"`;
      const withoutRowid = /\bWITHOUT\s+ROWID\b/i.test(t.sql);
      const order = withoutRowid
        ? info.filter((c) => c.pk > 0).sort((a, b) => a.pk - b.pk).map((c) => q(c.name)).join(", ")
        : "rowid";
      const select = info.map((c) => `quote(${q(c.name)})`).join(", ");
      let n = 0;
      for (const r of db.prepare(`SELECT ${select} FROM ${q(t.name)} ORDER BY ${order}`).raw().iterate() as Iterable<string[]>) {
        for (const v of r) feed(v);
        feed("␞"); // row boundary
        n++;
      }
      tableCounts[t.name] = n;
      rows += n;
    }
    return tables.length;
  });
  const tables = run();
  return { digest: h.digest("hex"), tables, rows, tableCounts };
}

export interface Hold {
  priorMode: string;
}

/** True for a connection that has no file to lock (an in-memory database). */
const isMemory = (db: Database.Database) => db.memory || db.name === "" || db.name === ":memory:";

/**
 * Step 1 and 2 of 1.3.8: exclusive locking mode, leave WAL (refused while any other connection
 * has the database open), then take the lock and keep it. Throws QuiesceRequiredError with
 * nothing written when another connection is open.
 */
export function acquireHold(db: Database.Database): Hold | null {
  if (isMemory(db)) return null;
  const priorMode = String(db.pragma("journal_mode", { simple: true })).toLowerCase();
  try {
    db.pragma("locking_mode = EXCLUSIVE");
    const mode = String(db.pragma("journal_mode = DELETE", { simple: true })).toLowerCase();
    if (mode !== "delete") throw Object.assign(new Error(`journal_mode stayed ${mode}`), { code: "SQLITE_BUSY" });
    db.exec("BEGIN EXCLUSIVE; COMMIT");
  } catch (e) {
    try {
      db.pragma("locking_mode = NORMAL");
      db.prepare("SELECT 1").get();
    } catch {
      /* the connection is being abandoned */
    }
    if (isBusy(e)) throw new QuiesceRequiredError(errMsg(e));
    throw e;
  }
  return { priorMode };
}

/**
 * Step 7: normal locking, one read to drop the lock, then back to WAL. The order matters: SQLite
 * cannot leave EXCLUSIVE locking once WAL was entered under it, so the lock mode is relaxed (and
 * the lock released) while the journal is still DELETE. Plan 1.3.8 step 7 lists WAL first; probed
 * 2026-10-01 that order leaves the lock held for the life of the connection.
 */
export function releaseHold(db: Database.Database, hold: Hold | null): void {
  if (!hold) return;
  db.pragma("locking_mode = NORMAL");
  db.prepare("SELECT 1 FROM sqlite_master LIMIT 1").get();
  if (hold.priorMode === "wal") db.pragma("journal_mode = WAL");
}

function fsyncPath(path: string): void {
  const fd = openSync(path, "r"); // O_RDONLY
  try {
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
}

const stamp = (d: Date) => d.toISOString().replace(/[-:]/g, "").replace(/\.\d+Z$/, "Z");

export interface BackupResult extends DigestResult {
  path: string;
  schemaVersion: number;
}

export interface BackupOptions {
  now?: () => Date;
  test?: BackupTestSeams;
}

function schemaValue(db: Database.Database, key: string): number | undefined {
  const r = db.prepare("SELECT value FROM schema_meta WHERE key = ?").get(key) as { value: number } | undefined;
  return r?.value;
}

/**
 * Steps 3-6 of 1.3.8. Writes `<dir>/home-v2-backup-<UTC stamp>.db` with VACUUM INTO, fsyncs it and
 * its directory, then verifies integrity, schema_version, min_reader_version <= 2 and the full
 * content digest against the live database. Any failure throws BackupNotVerifiedError, with no
 * retry; the partial file stays in place.
 */
export function backupBeforeMigrate(db: Database.Database, opts: BackupOptions = {}): BackupResult {
  if (isMemory(db)) throw new BackupNotVerifiedError(null, "backup target", "an in-memory database has no directory to back up into");
  const hook = opts.test?.hook ?? (() => {});
  const live = schemaValue(db, "schema_version") ?? 0;
  const dir = dirname(resolve(db.name));
  const path = join(dir, `home-v2-backup-${stamp((opts.now ?? (() => new Date()))())}.db`);
  if (existsSync(path)) throw new BackupNotVerifiedError(path, "VACUUM INTO", "target already exists");
  try {
    db.prepare("VACUUM INTO ?").run(path);
    fsyncPath(path);
    fsyncPath(dir);
  } catch (e) {
    throw new BackupNotVerifiedError(path, "VACUUM INTO", errMsg(e));
  }
  hook("backup-written", { backupPath: path });

  let b: Database.Database | null = null;
  let backupDigest: DigestResult;
  try {
    try {
      b = new Database(path, { readonly: true });
      const ic = b.pragma("integrity_check", { simple: true });
      if (ic !== "ok") throw new BackupNotVerifiedError(path, "integrity_check", String(ic));
    } catch (e) {
      if (e instanceof BackupNotVerifiedError) throw e;
      throw new BackupNotVerifiedError(path, "integrity_check", errMsg(e));
    }
    const sv = schemaValue(b, "schema_version");
    if (sv !== live) throw new BackupNotVerifiedError(path, "schema_version", `backup has ${sv}, live database has ${live}`);
    const mr = schemaValue(b, "min_reader_version") ?? 0;
    if (mr > 2) throw new BackupNotVerifiedError(path, "min_reader_version", `backup demands reader ${mr}, restore needs <= 2`);
    backupDigest = contentDigest(b);
  } finally {
    b?.close();
  }
  const liveDigest = contentDigest(db);
  if (liveDigest.digest !== backupDigest.digest) {
    throw new BackupNotVerifiedError(
      path,
      "digest",
      `content digest differs (backup ${backupDigest.digest.slice(0, 12)}, live ${liveDigest.digest.slice(0, 12)})`,
    );
  }
  hook("verified", { backupPath: path });
  return { path, schemaVersion: live, ...liveDigest };
}
