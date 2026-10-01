// Review finding 4: a refused migration (quiesce or unverified backup) fails activation instead of looping.
// Plan 1.3.8 (the plugin does not start) and 8.9 item 1 (the refusal is a thrown factory error, so bb keeps v2).
import Database from "better-sqlite3";
import { chmodSync, closeSync, openSync, readdirSync, writeSync } from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { BackupNotVerifiedError } from "../backup.js";
import { createStoreHandle } from "../store.js";
import { MIGRATIONS, migrate } from "../migrations.js";
import { wireStore } from "../server.js";
import { tmpDir } from "./helpers.js";
import { loadV2 } from "./v2build.js";

let t: ReturnType<typeof tmpDir>;
let file: string;
beforeEach(() => {
  t = tmpDir();
  file = join(t.dir, "data.db");
});
afterEach(() => {
  vi.useRealTimers();
  try {
    chmodSync(t.dir, 0o755);
  } catch {
    /* gone */
  }
  t.cleanup();
});

function populateV2(mode: "WAL" | "DELETE") {
  const db = new Database(file);
  db.pragma("journal_mode = WAL");
  migrate(db, { codeVersion: 2, migrations: MIGRATIONS.slice(0, 2) });
  db.prepare("INSERT INTO notes(id, at, text) VALUES ('n1','t','keep')").run();
  db.pragma(`journal_mode = ${mode}`);
  db.close();
}
function fakeBb(open: () => Database.Database) {
  const disposers: (() => void)[] = [];
  const services: string[] = [];
  const log = { info: vi.fn(), warn: vi.fn(), error: vi.fn() };
  const bb = { log, storage: { database: open }, onDispose: (fn: () => void) => disposers.push(fn), background: { service: (n: string) => services.push(n) } };
  return { bb: bb as never, disposers, services, log };
}
const backups = () => readdirSync(t.dir).filter((f) => f.startsWith("home-v2-backup-"));

describe("wireStore on a refused migration", () => {
  it("v2 still holds the file: activation throws the quiesce refusal, the plugin does not start, no retry, v2 keeps working", async () => {
    populateV2("WAL");
    const v2 = await loadV2();
    const old = new v2.Store(new Database(file));
    old.setting("store_id");
    vi.useFakeTimers();
    let opens = 0;
    const f = fakeBb(() => {
      opens++;
      return new Database(file, { timeout: 50 });
    });
    expect(() => wireStore(f.bb)).toThrow(/\[home-refused:quiesce-required\] another connection holds data\.db/);
    expect(f.services).toEqual([]);
    f.disposers.forEach((d) => d());
    vi.advanceTimersByTime(10 * 60_000);
    expect(opens).toBe(1);
    expect(backups()).toEqual([]);
    expect(f.log.warn.mock.calls.map((c) => String(c[0])).join("\n")).toContain("[home-refused:quiesce-required]");
    expect(old.setting("store_id")).toBeTruthy();
    old.close();
  });

  it("an unverifiable backup: the handle records the refusal, one attempt only, one kept backup copy and not a growing pile", () => {
    populateV2("WAL");
    vi.useFakeTimers({ now: new Date("2026-10-01T12:00:00Z") });
    let opens = 0;
    const warns: string[] = [];
    const handle = createStoreHandle(
      () => {
        opens++;
        return new Database(file, { timeout: 50 });
      },
      {
        closeOnFailure: true,
        log: { info() {}, warn: (m) => warns.push(m) },
        migrate: {
          test: {
            hook: (step, ctx) => {
              if (step !== "backup-written") return;
              const fd = openSync(ctx.backupPath!, "r+");
              writeSync(fd, Buffer.alloc(4096, 0xff), 0, 4096, 4096 * 2);
              closeSync(fd);
            },
          },
        },
      },
    );
    expect(handle.ready()).toBe(false);
    expect(handle.refusal()).toBeInstanceOf(BackupNotVerifiedError);
    expect(handle.refusal()!.message).toMatch(/^\[home-refused:backup-not-verified\] pre-migration backup not verified: integrity_check failed/);
    vi.advanceTimersByTime(10 * 60_000);
    expect(opens).toBe(1);
    expect(backups().length).toBe(1);
    expect(warns.join("\n")).toContain("[home-refused:backup-not-verified]");
    expect(warns.join("\n")).not.toContain("retrying");
    handle.dispose();
  });

  it("a plain locked or busy database still degrades and retries (no throw)", () => {
    const db = new Database(":memory:");
    db.close();
    const f = fakeBb(() => db);
    const h = wireStore(f.bb);
    expect(h.ready()).toBe(false);
    expect(h.refusal()).toBeNull();
    f.disposers.forEach((d) => d());
  });
});
