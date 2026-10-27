import Database from "better-sqlite3";
import { lstatSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { MIGRATIONS, migrate } from "../migrations.js";
import { setFsUuidResolverForTest } from "../ruling.js";

let base: string;
beforeEach(() => {
  base = mkdtempSync(join(tmpdir(), "fsid-mig-"));
});
afterEach(() => {
  setFsUuidResolverForTest(null);
  rmSync(base, { recursive: true, force: true });
});

function v4db() {
  const db = new Database(join(base, "d" + Math.random() + ".db"));
  migrate(db, { codeVersion: 4, migrations: MIGRATIONS.filter((m) => m.version <= 4), test: { skipHold: true } });
  return db;
}
function row(db: Database.Database, id: string, root: string | null, ino: string | null, dev = "1") {
  db.prepare(
    `INSERT INTO decisions(id, request_id, identity, revision, semantic_key, subject, kind, project, project_root, root_dev, root_ino, asker, thread, body_json, delegable, filed_at, updated_at)
     VALUES (?, ?, 'i', 'r', 'k', 's', 'decide', 'p', ?, ?, ?, 'a', 't', '{}', 1, 'x', 'x')`,
  ).run(id, `req-${id}`, root, dev, ino);
}

describe("v5 adds decisions.root_fsid and leaves legacy rows unpinned (mk-mpa66)", () => {
  it("adds a nullable column, fills nothing (even where the path still holds the pinned inode), and touches no other column", () => {
    const good = join(base, "good");
    mkdirSync(good);
    const st = lstatSync(good, { bigint: true });
    const db = v4db();
    row(db, "good", good, String(st.ino), String(st.dev));
    row(db, "noroot", null, null);
    const before = db.prepare("SELECT id, project_root, root_dev, root_ino, body_json, updated_at FROM decisions ORDER BY id").all();
    setFsUuidResolverForTest(() => "U1");
    migrate(db);
    expect(db.prepare("SELECT id, root_fsid FROM decisions ORDER BY id").all()).toEqual([
      { id: "good", root_fsid: null },
      { id: "noroot", root_fsid: null },
    ]);
    expect(db.prepare("SELECT id, project_root, root_dev, root_ino, body_json, updated_at FROM decisions ORDER BY id").all()).toEqual(before);
  });
});
