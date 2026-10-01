// Task 2.8a test 1 (A10): the forward-only fence. Once v3 has migrated a v2 database, the a9853e2 build refuses it
// and writes nothing; the verified backup the migration took is still a database the a9853e2 build runs on.
import { chmodSync, copyFileSync, existsSync, statSync } from "node:fs";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { makeEnv, type Env } from "./service-helpers.js";
import { populateV2, startV2Home, type V2Fixture } from "./v2-home.js";

let env: Env;
let fx: V2Fixture;
const sha = (f: string) => createHash("sha256").update(readFileSync(f)).digest("hex");
const walSize = (f: string) => (existsSync(`${f}-wal`) ? statSync(`${f}-wal`).size : 0);

beforeEach(async () => {
  env = makeEnv(["Autarch"]);
  const v2 = await startV2Home(env);
  fx = await populateV2(env, v2);
  v2.close();
  env.clock.t = Date.now() + 120_000;
});
afterEach(() => {
  for (const p of [env.roots.Autarch!, join(env.roots.Autarch!, "docs", "decisions")]) {
    try {
      chmodSync(p, 0o755);
    } catch {
      /* gone */
    }
  }
  env.cleanup();
});

/** Migrate under v3 and close cleanly (wal_checkpoint(TRUNCATE) runs in Store.close). */
function migrateAndClose(): { backup: string } {
  const svc = env.open();
  const row = svc.store.db.prepare("SELECT backup_path FROM migration_log").get() as { backup_path: string };
  svc.store.close();
  return { backup: row.backup_path };
}

describe("forward-only fence", () => {
  it("the a9853e2 build refuses a v3 database: not ready, schema-too-new, and the file is unchanged", async () => {
    migrateAndClose();
    const before = sha(env.file);
    expect(walSize(env.file)).toBe(0);
    const v2 = await startV2Home(env);
    expect(v2.ready()).toBe(false);
    expect(v2.error()).toMatch(/reader version 3|schema-too-new/);
    // Its commands do not work against the refused store.
    await expect(v2.run(["list"], { threadId: "thr-a" })).rejects.toThrow(/home store not ready.*reader version 3/);
    v2.close();
    expect(sha(env.file)).toBe(before);
    expect(walSize(env.file)).toBe(0);
  });

  it("a refused start still leaves the database usable by v3, with every legacy ask intact", async () => {
    migrateAndClose();
    const v2 = await startV2Home(env);
    expect(v2.ready()).toBe(false);
    v2.close();
    const svc = env.open();
    const ids = (svc.store.db.prepare("SELECT id FROM decisions WHERE source='home'").all() as { id: string }[]).map((r) => r.id);
    for (const id of [fx.decide, fx.mycroft, fx.steps, fx.machine, fx.picked, fx.delegated, fx.withdrawn]) expect(ids).toContain(id);
    svc.store.close();
  });

  it("the migration's verified backup is a v2 database the a9853e2 build runs on, with the same asks", async () => {
    const { backup } = migrateAndClose();
    expect(existsSync(backup)).toBe(true);
    const restored = join(env.file + ".restored");
    copyFileSync(backup, restored);
    const v2 = await startV2Home({ ...env, file: restored } as Env);
    expect(v2.ready()).toBe(true);
    const owed = (await v2.handlers.listAsks(null as never)) as { owed: { id: string }[] };
    expect(owed.owed.map((o) => o.id)).toContain(fx.decide);
    v2.close();
  });

  it("a database that was never v2 (fresh v3) is refused the same way", async () => {
    const fresh = makeEnv(["Autarch"]);
    try {
      fresh.open().store.close();
      const before = sha(fresh.file);
      const v2 = await startV2Home(fresh);
      expect(v2.ready()).toBe(false);
      v2.close();
      expect(sha(fresh.file)).toBe(before);
    } finally {
      fresh.cleanup();
    }
  });
});
