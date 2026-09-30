import Database from "better-sqlite3";
import { describe, expect, it, vi } from "vitest";
import { wireStore } from "../server.js";

// A fake SDK: only what wireStore touches. Real-bb activation belongs to Task 1.11.
function fakeBb(db: Database.Database) {
  const disposers: (() => void)[] = [];
  const services: string[] = [];
  const bb = {
    log: { info: vi.fn(), warn: vi.fn(), error: vi.fn() },
    storage: { database: () => db },
    onDispose: (fn: () => void) => disposers.push(fn),
    background: { service: (name: string) => services.push(name) },
  };
  return { bb: bb as never, disposers, services, log: bb.log };
}

describe("wireStore", () => {
  it("opens the store on the host database, registers the export service and disposes", () => {
    const f = fakeBb(new Database(":memory:"));
    const handle = wireStore(f.bb);
    expect(handle.ready()).toBe(true);
    expect(handle.store().storeId).toMatch(/^[0-9a-f]{32}$/);
    expect(f.services).toEqual(["home-export"]);
    f.disposers.forEach((d) => d());
  });

  it("does not throw when the database is unusable; it reports not-ready", () => {
    const db = new Database(":memory:");
    db.close();
    const f = fakeBb(db);
    const handle = wireStore(f.bb);
    expect(handle.ready()).toBe(false);
    expect(handle.error()).toBeTruthy();
    f.disposers.forEach((d) => d());
  });
});
