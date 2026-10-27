import Database from "better-sqlite3";
import { lstatSync, mkdirSync, mkdtempSync, rmSync, symlinkSync } from "node:fs";
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
function row(db: Database.Database, id: string, root: string | null, ino: string | null) {
  db.prepare(
    `INSERT INTO decisions(id, request_id, identity, revision, semantic_key, subject, kind, project, project_root, root_dev, root_ino, asker, thread, body_json, delegable, filed_at, updated_at)
     VALUES (?, ?, 'i', 'r', 'k', 's', 'decide', 'p', ?, '1', ?, 'a', 't', '{}', 1, 'x', 'x')`,
  ).run(id, `req-${id}`, root, ino);
}

describe("v5 backfill of decisions.root_fsid (mk-mpa66)", () => {
  it("adds the column and fills it only where the path still holds the pinned inode as a directory on a UUID filesystem; touches no other column", () => {
    const good = join(base, "good");
    const replaced = join(base, "replaced");
    const real = join(base, "real");
    mkdirSync(good);
    mkdirSync(replaced);
    mkdirSync(real);
    symlinkSync(real, join(base, "link"));
    const ino = (p: string) => String(lstatSync(p, { bigint: true }).ino);
    const db = v4db();
    row(db, "good", good, ino(good));
    row(db, "replaced", replaced, "999999999");
    row(db, "link", join(base, "link"), ino(real));
    row(db, "gone", join(base, "gone"), "5");
    row(db, "noroot", null, null);
    const before = db.prepare("SELECT id, project_root, root_dev, root_ino, body_json, updated_at FROM decisions ORDER BY id").all();
    setFsUuidResolverForTest(() => "U1");
    migrate(db);
    expect(db.prepare("SELECT id, root_fsid FROM decisions ORDER BY id").all()).toEqual([
      { id: "gone", root_fsid: null },
      { id: "good", root_fsid: "U1" },
      { id: "link", root_fsid: null },
      { id: "noroot", root_fsid: null },
      { id: "replaced", root_fsid: null },
    ]);
    expect(db.prepare("SELECT id, project_root, root_dev, root_ino, body_json, updated_at FROM decisions ORDER BY id").all()).toEqual(before);
  });

  it("leaves root_fsid null when the filesystem has no UUID", () => {
    const good = join(base, "good");
    mkdirSync(good);
    const db = v4db();
    row(db, "good", good, String(lstatSync(good, { bigint: true }).ino));
    setFsUuidResolverForTest(() => null);
    migrate(db);
    expect(db.prepare("SELECT root_fsid FROM decisions").get()).toEqual({ root_fsid: null });
  });
});
