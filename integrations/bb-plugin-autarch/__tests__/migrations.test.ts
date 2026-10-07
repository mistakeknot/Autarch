import Database from "better-sqlite3";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { createHash } from "node:crypto";
import { existsSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { CODE_VERSION, MIGRATIONS, migrate, readSchemaState, SchemaTooNewError, type Migration } from "../migrations.js";
import { createStoreHandle, Store } from "../store.js";
import { QuiesceRequiredError } from "../backup.js";
import { decision, openStore, pickOf, tmpDir } from "./helpers.js";
import { loadV2 } from "./v2build.js";

let t: ReturnType<typeof tmpDir>;
let file: string;
beforeEach(() => {
  t = tmpDir();
  file = join(t.dir, "data.db");
});
afterEach(() => t.cleanup());

const V2: Migration = { version: 2, sql: "ALTER TABLE decisions ADD COLUMN triage TEXT; CREATE INDEX decisions_triage ON decisions(triage);" };

describe("staged migrations", () => {
  it("a fresh database gets the current schema and a store_id", () => {
    const db = new Database(file);
    expect(migrate(db)).toEqual({ schemaVersion: CODE_VERSION, minReaderVersion: 3 });
    const id = (db.prepare("SELECT value FROM settings_kv WHERE key='store_id'").get() as { value: string }).value;
    expect(id).toMatch(/^[0-9a-f]{32}$/);
    // idempotent, and keeps the store_id
    migrate(db);
    expect((db.prepare("SELECT value FROM settings_kv WHERE key='store_id'").get() as { value: string }).value).toBe(id);
  });

  it("a database at an older schema version migrates and keeps its data", () => {
    const db = new Database(file);
    migrate(db, { codeVersion: 1, migrations: [MIGRATIONS[0]!] });
    db.prepare("INSERT INTO notes(id, at, text) VALUES ('n','t','keep me')").run();
    expect(readSchemaState(db).schemaVersion).toBe(1);
    expect(migrate(db, { codeVersion: 2, migrations: [MIGRATIONS[0]!, V2] })).toEqual({ schemaVersion: 2, minReaderVersion: 0 });
    expect(db.prepare("SELECT text FROM notes").get()).toEqual({ text: "keep me" });
    expect(db.prepare("SELECT triage FROM decisions").all()).toEqual([]);
  });

  it("the shipped v1 to v2 step adds the approvals tables and keeps data", () => {
    const db = new Database(file);
    migrate(db, { codeVersion: 1, migrations: [MIGRATIONS[0]!] });
    db.prepare("INSERT INTO notes(id, at, text) VALUES ('n','t','keep me')").run();
    expect(migrate(db, { codeVersion: 2 })).toEqual({ schemaVersion: 2, minReaderVersion: 0 });
    expect(db.prepare("SELECT text FROM notes").get()).toEqual({ text: "keep me" });
    expect(db.prepare("SELECT COUNT(*) n FROM approvals").get()).toEqual({ n: 0 });
    expect(db.prepare("SELECT COUNT(*) n FROM approval_events").get()).toEqual({ n: 0 });
  });

  it("refuses a database whose min_reader_version exceeds the code, and the handle stays not-ready", () => {
    const db = new Database(file);
    migrate(db);
    db.prepare("UPDATE schema_meta SET value = 99 WHERE key = 'min_reader_version'").run();
    expect(() => migrate(db)).toThrow(SchemaTooNewError);
    db.close();
    const handle = createStoreHandle(() => new Database(file), { closeOnFailure: true });
    expect(handle.ready()).toBe(false);
    expect(handle.error()).toMatch(/requires reader version 99/);
    handle.dispose();
  });

  it("a failed migration rolls back completely", () => {
    const db = new Database(file);
    migrate(db);
    const bad: Migration = { version: 5, sql: "ALTER TABLE decisions ADD COLUMN ok TEXT; THIS IS NOT SQL;" };
    expect(() => migrate(db, { codeVersion: 5, migrations: [...MIGRATIONS, bad] })).toThrow();
    expect(readSchemaState(db).schemaVersion).toBe(CODE_VERSION);
    expect(() => db.prepare("SELECT ok FROM decisions").all()).toThrow();
  });

  it("a candidate that cannot quiesce is refused, and version N keeps filing, picking and reconciling on its unchanged file", () => {
    const n = openStore(file);
    // candidate N+1 tries to migrate the same file while N is open: refused, nothing written
    const cand = new Database(file);
    expect(() => migrate(cand, { codeVersion: 5, migrations: [...MIGRATIONS, { ...V2, version: 5 }] })).toThrow(QuiesceRequiredError);
    cand.close();
    expect(readSchemaState(n.db).schemaVersion).toBe(CODE_VERSION);

    const d = decision();
    expect(n.insertDecision(d)).toMatchObject({ inserted: true });
    expect(n.recordPick(pickOf(d.id), [{ id: "o", kind: "notify" }])).toMatchObject({ ok: true });
    expect(n.claim("o", 0)).toEqual({ ok: true, attempt: 1 });
    n.updateAttempt("o", 1, "delivered", { evidence: "e" });
    expect(n.transition("o", "sending", "done", 1)).toEqual({ ok: true });

    // and a fresh version-N instance still starts on it
    const again = openStore(file);
    expect(again.obligation("o")?.state).toBe("done");
  });
});

// ---- v3 schema (Task 2.3 tests 1-5, 7) ----------------------------------------

const DEC_COLS = "id, request_id, identity, revision, semantic_key, subject, kind, project, asker, thread, body_json, filed_at, updated_at";
function insDecision(db: Database.Database, id: string, extra: Record<string, unknown> = {}): void {
  const base: Record<string, unknown> = {
    id, request_id: `req-${id}`, identity: "i", revision: "rev-1", semantic_key: "s", subject: "subj", kind: "decide",
    project: "autarch", asker: "a", thread: "t", body_json: "{}", filed_at: "x", updated_at: "x", ...extra,
  };
  const cols = Object.keys(base);
  db.prepare(`INSERT INTO decisions(${cols.join(",")}) VALUES (${cols.map((c) => "@" + c).join(",")})`).run(base);
}
const CARD = { source: "card", task_id: "T1", generation: 1, tasks_project_id: "P1", card_fp: "fp1" };
const v3db = () => {
  const db = new Database(file);
  db.pragma("journal_mode = WAL");
  migrate(db);
  return db;
};
const abort = (fn: () => unknown) => expect(fn).toThrow(/constraint|abort|immutable|never carries|fixed at insert|append-only|frozen|insert-only|UNIQUE/i);

describe("v3 migration", () => {
  it("1. a populated v2 database migrates with every row and column preserved", async () => {
    const v2 = await loadV2();
    const old = openV2(v2);
    const d = decision();
    old.insertDecision(d);
    old.insertDecision(decision({ kind: "decide" }));
    old.recordPick(pickOf(d.id), [{ id: "o1", kind: "notify", recipient: "x" }, { id: "o2", kind: "ruling-file" }]);
    old.markSeen("acct", "item");
    old.setSetting("k", "v");
    old.close();
    const raw = new Database(file, { readonly: true });
    const tables = (raw.prepare("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name").all() as { name: string }[]).map((r) => r.name);
    const dump = (db: Database.Database, name: string, cols?: string[]) =>
      db.prepare(`SELECT ${cols ? cols.map((c) => `"${c}"`).join(",") : "*"} FROM "${name}" ORDER BY rowid`).all();
    const before = new Map(tables.map((n) => [n, dump(raw, n)]));
    const colsOf = new Map(tables.map((n) => [n, (raw.pragma(`table_info("${n}")`) as { name: string }[]).map((c) => c.name)]));
    expect(before.get("decisions")).toHaveLength(2);
    raw.close();

    const s = new Store(new Database(file), { busyTimeoutMs: 50 });
    for (const n of tables) {
      if (n === "schema_meta") continue;
      expect(dump(s.db, n, colsOf.get(n)), n).toEqual(before.get(n));
    }
    expect(s.db.prepare("SELECT source, task_id, generation, tasks_project_id, card_fp FROM decisions").all()).toEqual([
      { source: "home", task_id: null, generation: null, tasks_project_id: null, card_fp: null },
      { source: "home", task_id: null, generation: null, tasks_project_id: null, card_fp: null },
    ]);
    for (const t3 of ["cards", "card_requests", "decision_blocks", "card_writes", "project_bindings", "migration_log"]) {
      expect(s.db.prepare(`SELECT COUNT(*) c FROM ${t3}`).get(), t3).toBeDefined();
    }
    s.close();
  });

  it("2. (task_id, generation) is unique where task_id is set", () => {
    const db = v3db();
    insDecision(db, "g1", CARD);
    abort(() => insDecision(db, "g1dup", CARD));
    insDecision(db, "g2", { ...CARD, generation: 2 });
    insDecision(db, "h1");
    insDecision(db, "h2"); // NULL task_id rows do not collide
  });

  it("3. decisions_ask_immutable rejects body_json and card_fp updates, allows updated_at and resolved_at", () => {
    const db = v3db();
    insDecision(db, "g1", CARD);
    insDecision(db, "h1");
    abort(() => db.prepare("UPDATE decisions SET body_json = '{\"x\":1}' WHERE id = 'g1'").run());
    abort(() => db.prepare("UPDATE decisions SET card_fp = 'other' WHERE id = 'g1'").run());
    abort(() => db.prepare("UPDATE decisions SET card_fp = 'new' WHERE id = 'h1'").run()); // NULL -> value
    for (const col of ["revision", "identity", "subject", "semantic_key"]) {
      abort(() => db.prepare(`UPDATE decisions SET ${col} = 'changed' WHERE id = 'g1'`).run());
    }
    expect(db.prepare("UPDATE decisions SET updated_at = 'y', resolved_at = 'y', withdrawn_at = 'y' WHERE id = 'g1'").run().changes).toBe(1);
  });

  it("4. decisions_card_link and decisions_home_insert", () => {
    const db = v3db();
    insDecision(db, "card1", CARD);
    insDecision(db, "home1");
    // refused: source flips, both ways
    abort(() => db.prepare("UPDATE decisions SET source = 'home' WHERE id = 'card1'").run());
    abort(() => db.prepare("UPDATE decisions SET source = 'card' WHERE id = 'home1'").run());
    // refused: each link column, NULL -> value and value -> other
    for (const [col, val] of [["task_id", "T9"], ["generation", 9], ["tasks_project_id", "P9"]] as const) {
      abort(() => db.prepare(`UPDATE decisions SET ${col} = ? WHERE id = 'home1'`).run(val));
      abort(() => db.prepare(`UPDATE decisions SET ${col} = ? WHERE id = 'card1'`).run(val));
      abort(() => db.prepare(`UPDATE decisions SET ${col} = NULL WHERE id = 'card1'`).run());
    }
    // refused: a 'home' row inserted with any link column
    abort(() => insDecision(db, "bad1", { task_id: "T2" }));
    abort(() => insDecision(db, "bad2", { generation: 1 }));
    abort(() => insDecision(db, "bad3", { tasks_project_id: "P1" }));
    abort(() => insDecision(db, "bad4", { source: "other" }));
    // allowed: a card insert with all link columns, and updates of other columns
    insDecision(db, "card2", { ...CARD, task_id: "T2" });
    expect(db.prepare("UPDATE decisions SET updated_at = 'z', delegable = 0 WHERE id = 'card1'").run().changes).toBe(1);
    // a no-op write of a link column is not a change
    expect(db.prepare("UPDATE decisions SET source = 'card' WHERE id = 'card1'").run().changes).toBe(1);
  });

  it("5. card_requests, decision_blocks and cards_routing_frozen triggers", () => {
    const db = v3db();
    insDecision(db, "card1", CARD);
    db.prepare("INSERT INTO card_requests(request_key, task_id, identity, registered_at) VALUES ('k','T1','id1','t')").run();
    abort(() => db.prepare("UPDATE card_requests SET identity = 'x' WHERE request_key = 'k'").run());
    abort(() => db.prepare("DELETE FROM card_requests WHERE request_key = 'k'").run());
    db.prepare("INSERT INTO decision_blocks(decision_id, ref) VALUES ('card1','bead:mk-1')").run();
    abort(() => db.prepare("INSERT INTO decision_blocks(decision_id, ref) VALUES ('card1','bead:mk-1')").run()); // PK
    abort(() => db.prepare("UPDATE decision_blocks SET ref = 'bead:mk-2' WHERE decision_id = 'card1'").run());
    abort(() => db.prepare("DELETE FROM decision_blocks WHERE decision_id = 'card1'").run());

    db.prepare("INSERT INTO cards(task_id, state) VALUES ('T1','observed')").run();
    const up = (sql: string) => db.prepare(sql).run();
    up("UPDATE cards SET routing_mode = 'thread', routed_thread = 'thr_a' WHERE task_id = 'T1'"); // NULL -> value
    abort(() => up("UPDATE cards SET routing_mode = 'pull' WHERE task_id = 'T1'"));
    abort(() => up("UPDATE cards SET routed_thread = 'thr_b' WHERE task_id = 'T1'"));
    abort(() => up("UPDATE cards SET routing_mode = NULL WHERE task_id = 'T1'"));
    abort(() => up("UPDATE cards SET routed_thread = NULL WHERE task_id = 'T1'"));
    expect(up("UPDATE cards SET state = 'open', asking_thread = 'thr_c' WHERE task_id = 'T1'").changes).toBe(1);
    abort(() => up("UPDATE cards SET state = 'bogus' WHERE task_id = 'T1'"));
    abort(() => up("INSERT INTO cards(task_id, state, routing_mode) VALUES ('T2','open','sideways')"));
    // card_writes and project_bindings accept their documented values only
    db.prepare("INSERT INTO card_writes(id, task_id, decision_id, kind, payload, state, next_try_at, updated_at) VALUES ('w1','T1','card1','comment','{}','pending','t','t')").run();
    abort(() => db.prepare("INSERT INTO card_writes(id, task_id, decision_id, kind, payload, state, next_try_at, updated_at) VALUES ('w2','T1','card1','comment','{}','pending','t','t')").run()); // UNIQUE(decision_id, kind)
    abort(() => db.prepare("INSERT INTO card_writes(id, task_id, decision_id, kind, payload, state, next_try_at, updated_at) VALUES ('w3','T1','card1','nope','{}','pending','t','t')").run());
    db.prepare("INSERT INTO project_bindings(tasks_project_id, home_project, state, suggested_at) VALUES ('P1','autarch','suggested','t')").run();
    abort(() => db.prepare("INSERT INTO project_bindings(tasks_project_id, home_project, state) VALUES ('P2','autarch','trusted')").run());
  });

  it("7. reader fence: a v2 migrate on a v3 database throws SchemaTooNewError and writes nothing", async () => {
    const v2 = await loadV2();
    const s = new Store(new Database(file), { busyTimeoutMs: 50 });
    s.setSetting("k", "v");
    s.close(); // wal_checkpoint(TRUNCATE) on a clean stop
    const sha = () => createHash("sha256").update(readFileSync(file)).digest("hex");
    const meta = () => {
      const r = new Database(file, { readonly: true });
      try {
        return r.prepare("SELECT key, value FROM schema_meta ORDER BY key").all();
      } finally {
        r.close();
      }
    };
    const shaBefore = sha();
    const metaBefore = meta();
    expect(metaBefore).toContainEqual({ key: "min_reader_version", value: 3 });
    const db = new Database(file);
    expect(() => migrate(db, { codeVersion: 2, migrations: v2.MIGRATIONS })).toThrow(SchemaTooNewError);
    db.close();
    expect(sha()).toBe(shaBefore);
    expect(meta()).toEqual(metaBefore);
    const wal = `${file}-wal`;
    expect(!existsSync(wal) || statSync(wal).size === 0).toBe(true);
  });
});

function openV2(v2: Awaited<ReturnType<typeof loadV2>>) {
  return new v2.Store(new Database(file));
}

describe("v4 migration (moves)", () => {
  const v3Only = MIGRATIONS.filter((m) => m.version <= 3);

  it("migrates a real v3 database: backup verified, moves table added, no min_reader bump, data kept", () => {
    const old = new Database(file);
    migrate(old, { codeVersion: 3, migrations: v3Only });
    old.prepare("INSERT INTO cards(task_id, state) VALUES ('T1','open')").run();
    old.close();

    const logs: string[] = [];
    const db = new Database(file);
    expect(migrate(db, { log: { info: (m) => logs.push(m) } })).toEqual({ schemaVersion: 4, minReaderVersion: 3 });
    expect(logs.some((l) => l.startsWith("autarch: schema 3 → 4; backup ") && l.includes("verified"))).toBe(true);
    expect(db.prepare("SELECT COUNT(*) c FROM cards").get()).toEqual({ c: 1 });
    expect(db.prepare("SELECT COUNT(*) c FROM moves").get()).toEqual({ c: 0 });
    expect(db.prepare("SELECT version FROM migration_log ORDER BY version").all()).toEqual([{ version: 3 }, { version: 4 }]);
    db.close();
  });

  it("a v3 reader still opens a v4 database, and a database that demands reader 5 is refused", () => {
    const db = new Database(file);
    migrate(db);
    expect(() => migrate(db, { codeVersion: 3, migrations: v3Only })).not.toThrow();
    db.prepare("UPDATE schema_meta SET value = 5 WHERE key = 'min_reader_version'").run();
    expect(() => migrate(db)).toThrow(SchemaTooNewError);
    db.close();
  });

  it("moves rejects an unknown kind or state, and a duplicate (task, generation)", () => {
    const db = new Database(file);
    migrate(db);
    const ins = (kind: string, state: string, gen = 1) =>
      db.prepare("INSERT INTO moves(task_id, generation, kind, payload_json, state, opened_by, opened_at) VALUES ('T1', ?, ?, '{}', ?, 'card', 't')").run(gen, kind, state);
    expect(() => ins("nope", "open")).toThrow();
    expect(() => ins("pr", "done")).toThrow();
    ins("pr", "open");
    expect(() => ins("pr", "open")).toThrow();
    ins("pr", "claimed", 2);
    db.close();
  });
});
