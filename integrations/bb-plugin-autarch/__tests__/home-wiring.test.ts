import Database from "better-sqlite3";
import { describe, expect, it, vi } from "vitest";
import { createStoreHandle } from "../store.js";
import { wireHome, type HomeConfig } from "../server.js";
import type { ServeClient } from "../serve.js";

const cfg: HomeConfig = { serveAddr: "127.0.0.1:8110", serveTokenFile: "/nope", serveProjectDirs: [], autarchBin: "autarch" };

function fakeBb() {
  const events = new Map<string, ((p: unknown) => unknown)[]>();
  const services: string[] = [];
  const disposers: (() => void)[] = [];
  let configure: ((ctx: { thread: { id: string }; project: { name: string } }) => { tools: []; skills: []; instructions?: string }) | undefined;
  let cli: { run(argv: string[], ctx: object): unknown } | undefined;
  const bb = {
    log: { info: vi.fn(), warn: vi.fn(), error: vi.fn() },
    onDispose: (fn: () => void) => disposers.push(fn),
    background: { service: (name: string) => services.push(name) },
    events: { on: (name: string, fn: (p: unknown) => unknown) => events.set(name, [...(events.get(name) ?? []), fn]) },
    agents: { configure: (fn: typeof configure) => (configure = fn) },
    cli: { register: (r: typeof cli) => (cli = r) },
    sdk: { threads: {} },
  };
  return { bb: bb as never, events, services, disposers, configure: () => configure, cli: () => cli };
}

const serve = { projects: async () => [], health: async () => ({ build: { commit: "abc" } }), healthy: async () => true } as unknown as ServeClient;

describe("wireHome", () => {
  it("registers the CLI, the feed, the event listeners and the services", () => {
    const db = new Database(":memory:");
    const handle = createStoreHandle(() => db, {});
    const f = fakeBb();
    const home = wireHome(f.bb, handle, cfg, { serve });
    expect([...f.events.keys()].sort()).toEqual(["message.cancelled", "message.dispatched", "thread.archived", "thread.deleted", "turn.failed"]);
    expect(f.services.sort()).toEqual(["home-feed-refresh", "home-serve", "home-wakes"]);
    expect(f.configure()).toBeTypeOf("function");
    expect(f.cli()).toBeDefined();
    expect(Object.keys(home.handlers).sort()).toEqual(["catchup", "dismiss", "health", "listAsks", "listRecent", "markAllSeen", "markSeen", "override", "pick", "resend", "setDelegation", "stats"]);
    f.disposers.forEach((d) => d());
  });

  it("the RPC handlers work against the store and record picks as mk", async () => {
    const db = new Database(":memory:");
    const handle = createStoreHandle(() => db, {});
    const f = fakeBb();
    const home = wireHome(f.bb, handle, cfg, { serve });
    expect(await home.handlers.listAsks(null)).toMatchObject({ owed: [], uncertain: [], delegation: { suspended: false }, machineOwners: {} });
    const h = (await home.handlers.health(null)) as { ready: boolean; build: unknown; source_sha256: unknown; projects: unknown };
    expect(h).toMatchObject({ ready: true, build: { commit: "abc" }, projects: [] });
    expect(await home.handlers.catchup(null)).toMatchObject({ items: [] });
    expect(await home.handlers.markAllSeen({ ids: ["failed:1"] })).toMatchObject({ marked: [] });
    expect(await home.handlers.setDelegation({ vizierThreadId: "thr-v", projects: ["Autarch"], dailyCap: 3 })).toMatchObject({ ok: true });
    f.disposers.forEach((d) => d());
  });

  it("a turn.failed for a thread with no asks records nothing; message events reach the loop without throwing", async () => {
    const db = new Database(":memory:");
    const handle = createStoreHandle(() => db, {});
    const f = fakeBb();
    wireHome(f.bb, handle, cfg, { serve });
    for (const fn of f.events.get("turn.failed")!) await fn({ threadId: "t", requestId: "r" });
    for (const fn of f.events.get("message.dispatched")!) await fn({ entry: { id: "q1" } });
    for (const fn of f.events.get("thread.deleted")!) await fn({ thread: { id: "t" } });
    expect((db.prepare("SELECT COUNT(*) AS n FROM events WHERE type = 'turn-failed'").get() as { n: number }).n).toBe(0);
    f.disposers.forEach((d) => d());
  });

  it("while the store is not ready the handlers refuse and configure returns nothing", async () => {
    const db = new Database(":memory:");
    db.close();
    const handle = createStoreHandle(() => db, {});
    const f = fakeBb();
    const home = wireHome(f.bb, handle, cfg, { serve });
    await expect(home.handlers.listAsks(null)).rejects.toThrow(/not ready/);
    expect(f.configure()!({ thread: { id: "t" }, project: { name: "Autarch" } })).toEqual({ tools: [], skills: [] });
    f.disposers.forEach((d) => d());
  });
});
