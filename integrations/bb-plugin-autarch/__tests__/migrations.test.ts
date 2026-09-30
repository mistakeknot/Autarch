import Database from "better-sqlite3";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { join } from "node:path";
import { CODE_VERSION, MIGRATIONS, migrate, readSchemaState, SchemaTooNewError, type Migration } from "../migrations.js";
import { createStoreHandle } from "../store.js";
import { decision, openStore, pickOf, tmpDir } from "./helpers.js";

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
    expect(migrate(db)).toEqual({ schemaVersion: CODE_VERSION, minReaderVersion: 0 });
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
    const bad: Migration = { version: 2, sql: "ALTER TABLE decisions ADD COLUMN ok TEXT; THIS IS NOT SQL;" };
    expect(() => migrate(db, { codeVersion: 2, migrations: [MIGRATIONS[0]!, bad] })).toThrow();
    expect(readSchemaState(db).schemaVersion).toBe(1);
    expect(() => db.prepare("SELECT ok FROM decisions").all()).toThrow();
  });

  it("failed activation after migration: version N keeps filing, picking and reconciling on the migrated file", () => {
    const n = openStore(file);
    // candidate N+1 migrates the same file, then its factory throws
    const cand = new Database(file);
    migrate(cand, { codeVersion: 2, migrations: [MIGRATIONS[0]!, V2] });
    cand.close();
    expect(readSchemaState(n.db).schemaVersion).toBe(2);

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
