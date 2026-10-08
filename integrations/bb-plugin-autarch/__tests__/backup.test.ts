// Task 2.3 test 6: quiesce and a verified backup before migration (plan 1.3.8).
import Database from "better-sqlite3";
import { spawn, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { chmodSync, existsSync, readdirSync, readFileSync, writeFileSync, openSync, writeSync, closeSync } from "node:fs";
import { join, isAbsolute } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { BackupNotVerifiedError, contentDigest, QuiesceRequiredError } from "../backup.js";
import { MIGRATIONS, migrate, readSchemaState } from "../migrations.js";
import { Store } from "../store.js";
import { decision, pickOf, tmpDir } from "./helpers.js";
import { loadV2 } from "./v2build.js";

let t: ReturnType<typeof tmpDir>;
let file: string;
beforeEach(() => {
  t = tmpDir();
  file = join(t.dir, "data.db");
});
afterEach(() => t.cleanup());

const sha = (f: string) => createHash("sha256").update(readFileSync(f)).digest("hex");
const backups = () => readdirSync(t.dir).filter((f) => /^home-v2-backup-\d{8}T\d{6}Z\.db$/.test(f));
const metaRows = () => {
  const r = new Database(file, { readonly: true });
  try {
    return r.prepare("SELECT key, value FROM schema_meta ORDER BY key").all();
  } finally {
    r.close();
  }
};
const hasTable = (name: string) => {
  const r = new Database(file, { readonly: true });
  try {
    return !!r.prepare("SELECT 1 FROM sqlite_master WHERE type='table' AND name=?").get(name);
  } finally {
    r.close();
  }
};

/** A populated database at schema 2, closed cleanly. */
function populateV2(): void {
  const db = new Database(file);
  db.pragma("journal_mode = WAL");
  migrate(db, { codeVersion: 2, migrations: MIGRATIONS.slice(0, 2) });
  const ins = db.prepare(
    `INSERT INTO decisions(id, request_id, identity, revision, semantic_key, subject, kind, project, asker, thread, body_json, filed_at, updated_at)
     VALUES (?, ?, 'i', 'rev-1', 's', 'subj', 'decide', 'autarch', 'a', 't', '{}', 'x', 'x')`,
  );
  ins.run("d1", "r1");
  ins.run("d2", "r2");
  db.prepare("INSERT INTO notes(id, at, text) VALUES ('n1','t','keep')").run();
  db.close();
}
const preDigest = () => {
  const r = new Database(file, { readonly: true });
  try {
    return contentDigest(r);
  } finally {
    r.close();
  }
};
const quick = { busyTimeoutMs: 50 } as const;

describe("backup before migrate", () => {
  it("success: verified backup at the logged path, digest equals the pre-migration content", () => {
    populateV2();
    const before = preDigest();
    const logs: string[] = [];
    const s = new Store(new Database(file), { ...quick, log: { info: (m) => logs.push(m), warn: () => {} } });
    const files = backups();
    expect(files).toHaveLength(1);
    const path = join(t.dir, files[0]!);
    const b = new Database(path, { readonly: true });
    expect(b.pragma("integrity_check", { simple: true })).toBe("ok");
    expect(readSchemaState(b).schemaVersion).toBe(2);
    expect(contentDigest(b).digest).toBe(before.digest);
    b.close();
    const row = s.db.prepare("SELECT * FROM migration_log WHERE version = 3").get() as Record<string, string>;
    expect(isAbsolute(row.backup_path!)).toBe(true);
    expect(row.backup_path).toBe(path);
    expect(row.digest).toBe(before.digest);
    expect(JSON.parse(row.table_counts_json!)).toMatchObject({ decisions: 2, notes: 1 });
    const line = logs.find((l) => l.startsWith("autarch: schema 2 → 4;"));
    expect(line).toContain(`backup ${path} verified`);
    expect(line).toContain(`digest ${before.digest.slice(0, 12)}`);
    expect(line).toMatch(/integrity ok.*\d+ tables, \d+ rows/);
    expect(readSchemaState(s.db)).toEqual({ schemaVersion: 4, minReaderVersion: 3 });
    // the hold is released: another connection can read and write
    const other = new Database(file, { timeout: 50 });
    other.prepare("UPDATE notes SET text = 'x' WHERE id = 'n1'").run();
    other.close();
    expect(String(s.db.pragma("journal_mode", { simple: true }))).toBe("wal");
    s.close();
  });

  it("existing v2 reader in the same process: QuiesceRequiredError, nothing written, v2 still works", async () => {
    populateV2();
    const v2 = await loadV2();
    const old = new v2.Store(new Database(file));
    old.setting("store_id");
    const shaBefore = sha(file);
    const metaBefore = metaRows();
    const db3 = new Database(file);
    expect(() => new Store(db3, quick)).toThrow(QuiesceRequiredError);
    db3.close();
    expect(sha(file)).toBe(shaBefore);
    expect(metaRows()).toEqual(metaBefore);
    expect(backups()).toEqual([]);
    expect(hasTable("cards")).toBe(false);
    const d = decision();
    expect(old.insertDecision(d)).toMatchObject({ inserted: true });
    expect(old.recordPick(pickOf(d.id), [{ id: "o", kind: "notify" }])).toMatchObject({ ok: true });
    old.close();
  });

  it("existing v2 reader in a child process: QuiesceRequiredError, nothing written", async () => {
    populateV2();
    const child = spawn(process.execPath, ["--import", "tsx", join(import.meta.dirname, "v2-reader-child.ts"), file], {
      cwd: join(import.meta.dirname, ".."),
      stdio: ["pipe", "pipe", "inherit"],
    });
    try {
      await new Promise<void>((res, rej) => {
        child.stdout.on("data", (b: Buffer) => b.toString().includes("ready") && res());
        child.on("exit", (c) => rej(new Error(`child exited ${c}`)));
      });
      const shaBefore = sha(file);
      const metaBefore = metaRows();
      const db3 = new Database(file);
      expect(() => new Store(db3, quick)).toThrow(QuiesceRequiredError);
      db3.close();
      expect(sha(file)).toBe(shaBefore);
      expect(metaRows()).toEqual(metaBefore);
      expect(backups()).toEqual([]);
    } finally {
      child.stdin.end();
      await new Promise((r) => child.on("exit", r));
    }
  });

  it("writers are excluded during the hold; the migration then succeeds and the backup equals the pre-migration content", () => {
    populateV2();
    const before = preDigest();
    const seen: string[] = [];
    const s = new Store(new Database(file), {
      ...quick,
      migrate: {
        test: {
          hook: (step) => {
            if (step !== "backup-written") return;
            const w = new Database(file, { timeout: 50 });
            for (const sql of [
              "UPDATE settings_kv SET value = 'tampered' WHERE key = 'store_id'",
              "DELETE FROM decisions WHERE id = 'd1'; INSERT INTO decisions(id, request_id, identity, revision, semantic_key, subject, kind, project, asker, thread, body_json, filed_at, updated_at) VALUES ('d1b','r1b','i','rev-1','s','subj','decide','autarch','a','t','{}','x','x')",
            ]) {
              try {
                w.exec(sql);
                seen.push("WROTE");
              } catch (e) {
                seen.push((e as { code?: string }).code ?? String(e));
              }
            }
            w.close();
          },
        },
      },
    });
    expect(seen).toEqual(["SQLITE_BUSY", "SQLITE_BUSY"]);
    const b = new Database(join(t.dir, backups()[0]!), { readonly: true });
    expect(contentDigest(b).digest).toBe(before.digest);
    b.close();
    expect(s.db.prepare("SELECT COUNT(*) c FROM decisions WHERE id = 'd1'").get()).toEqual({ c: 1 });
    s.close();
  });

  for (const [name, sql] of [
    ["an UPDATE of one settings_kv value", "UPDATE settings_kv SET value = 'tampered' WHERE key = 'store_id'"],
    [
      "a delete and insert that keeps the count",
      "DELETE FROM decisions WHERE id = 'd1'; INSERT INTO decisions(id, request_id, identity, revision, semantic_key, subject, kind, project, asker, thread, body_json, filed_at, updated_at) VALUES ('d1b','r1b','i','rev-1','s','subj','decide','autarch','a','t','{}','x','x')",
    ],
  ] as const) {
    it(`the digest catches what counts miss (skip-hold switch): ${name}`, () => {
      populateV2();
      const metaBefore = metaRows();
      const db = new Database(file);
      let err: unknown;
      try {
        new Store(db, {
          ...quick,
          migrate: {
            test: {
              skipHold: true,
              hook: (step) => {
                if (step !== "backup-written") return;
                const w = new Database(file, { timeout: 50 });
                w.exec(sql); // succeeds: nothing holds the lock
                w.close();
              },
            },
          },
        });
      } catch (e) {
        err = e;
      }
      db.close();
      expect(err).toBeInstanceOf(BackupNotVerifiedError);
      const e = err as BackupNotVerifiedError;
      expect(e.check).toBe("digest");
      expect(e.message).toContain("digest");
      expect(e.message).toContain(e.backupPath!);
      expect(e.backupPath).toBe(join(t.dir, backups()[0]!));
      expect(hasTable("cards")).toBe(false);
      expect(metaRows()).toEqual(metaBefore);
    });
  }

  describe("refusals leave the DB at v2 and name the path and the failed check", () => {
    const expectV2 = (metaBefore: unknown) => {
      expect(hasTable("cards")).toBe(false);
      expect(hasTable("migration_log")).toBe(false);
      expect(metaRows()).toEqual(metaBefore);
    };
    const refuse = (opts: Parameters<typeof migrate>[1], check: string) => {
      populateV2();
      const metaBefore = metaRows();
      const db = new Database(file);
      db.pragma("journal_mode = WAL");
      let err: unknown;
      try {
        migrate(db, opts);
      } catch (e) {
        err = e;
      }
      db.close();
      expect(err).toBeInstanceOf(BackupNotVerifiedError);
      const e = err as BackupNotVerifiedError;
      expect(e.check).toBe(check);
      expect(e.message).toContain(check);
      expect(e.message).toContain(e.backupPath!);
      expectV2(metaBefore);
      return e;
    };
    const FIXED = new Date("2026-10-01T12:00:00Z");

    it("VACUUM INTO fails: the target exists", () => {
      populateV2();
      const path = join(t.dir, "home-v2-backup-20261001T120000Z.db");
      writeFileSync(path, "occupied");
      const metaBefore = metaRows();
      const db = new Database(file);
      db.pragma("journal_mode = WAL");
      let err: unknown;
      try {
        migrate(db, { test: {}, now: () => FIXED });
      } catch (e) {
        err = e;
      }
      db.close();
      expect(err).toBeInstanceOf(BackupNotVerifiedError);
      expect((err as BackupNotVerifiedError).check).toBe("VACUUM INTO");
      expect((err as Error).message).toContain(path);
      expect(readFileSync(path, "utf8")).toBe("occupied");
      expectV2(metaBefore);
    });

    it.skipIf(process.getuid?.() === 0)("VACUUM INTO fails: the directory is not writable", () => {
      populateV2();
      const metaBefore = metaRows();
      const db = new Database(file);
      db.pragma("journal_mode = DELETE");
      chmodSync(t.dir, 0o555);
      let err: unknown;
      try {
        migrate(db, { now: () => FIXED });
      } catch (e) {
        err = e;
      }
      chmodSync(t.dir, 0o755);
      db.close();
      expect(err).toBeInstanceOf(BackupNotVerifiedError);
      expect((err as BackupNotVerifiedError).check).toBe("VACUUM INTO");
      expect((err as Error).message).toContain("home-v2-backup-20261001T120000Z.db");
      expectV2(metaBefore);
    });

    it("integrity_check is not ok (a corrupted page)", () => {
      refuse(
        {
          test: {
            hook: (step, ctx) => {
              if (step !== "backup-written") return;
              const fd = openSync(ctx.backupPath!, "r+");
              writeSync(fd, Buffer.alloc(4096, 0xff), 0, 4096, 4096 * 2);
              closeSync(fd);
            },
          },
        },
        "integrity_check",
      );
    });

    it("schema_version differs", () => {
      refuse(
        {
          test: {
            hook: (step, ctx) => {
              if (step !== "backup-written") return;
              const b = new Database(ctx.backupPath!);
              b.prepare("UPDATE schema_meta SET value = 1 WHERE key = 'schema_version'").run();
              b.close();
            },
          },
        },
        "schema_version",
      );
    });
  });

  it("a crash (SIGKILL) during the hold leaves v2 with no DDL, or v3 with its migration_log row; the next open succeeds", () => {
    const CHILD = join(import.meta.dirname, "backup-crash-child.ts");
    const outcomes: Record<string, number> = {};
    for (const step of ["backup-written", "verified", "ddl-applied", "committed"]) {
      t.cleanup();
      t = tmpDir();
      file = join(t.dir, "data.db");
      populateV2();
      const r = spawnSync(process.execPath, ["--import", "tsx", CHILD, file, step], {
        encoding: "utf8",
        cwd: join(import.meta.dirname, ".."),
      });
      expect(r.signal, r.stderr).toBe("SIGKILL");
      const probe = new Database(file);
      probe.pragma("busy_timeout = 2000");
      const v = readSchemaState(probe).schemaVersion;
      if (v === 2) {
        expect(probe.prepare("SELECT 1 FROM sqlite_master WHERE name IN ('cards','migration_log')").all()).toEqual([]);
      } else {
        expect(v).toBe(4);
        expect(probe.prepare("SELECT version FROM migration_log ORDER BY version").all()).toEqual([{ version: 3 }, { version: 4 }]);
      }
      outcomes[step] = v;
      probe.close();
      const s = new Store(new Database(file), { ...quick, migrate: { now: () => new Date(Date.now() + 120_000) } });
      expect(readSchemaState(s.db).schemaVersion).toBe(4);
      expect(s.db.prepare("SELECT COUNT(*) c FROM migration_log").get()).toEqual({ c: 2 });
      s.close();
    }
    expect(outcomes).toEqual({ "backup-written": 2, verified: 2, "ddl-applied": 2, committed: 4 });
  });

  it("a fresh database migrates with no backup, but still refuses while another connection is open", () => {
    const logs: string[] = [];
    const s = new Store(new Database(file), { ...quick, log: { info: (m) => logs.push(m), warn: () => {} } });
    expect(backups()).toEqual([]);
    expect(s.db.prepare("SELECT backup_path, digest FROM migration_log WHERE version = 3").get()).toEqual({
      backup_path: null,
      digest: null,
    });
    expect(logs.some((l) => l.startsWith("autarch: schema 0 → 4; no backup"))).toBe(true);
    s.close();

    const file2 = join(t.dir, "fresh2.db");
    const other = new Database(file2);
    other.pragma("journal_mode = WAL");
    other.prepare("SELECT COUNT(*) FROM sqlite_master").get();
    const db = new Database(file2);
    expect(() => new Store(db, quick)).toThrow(QuiesceRequiredError);
    db.close();
    expect(readSchemaState(other).schemaVersion).toBe(0);
    other.close();
    expect(existsSync(file2)).toBe(true);
  });
});
