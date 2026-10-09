import Database from "better-sqlite3";
import { join } from "node:path";
import { describe, expect, it, vi } from "vitest";
import { createStoreHandle } from "../store.js";
import { wireHome, type HomeConfig } from "../server.js";
import type { ServeClient } from "../serve.js";
import { ask, makeEnv } from "./service-helpers.js";
import { FakeSdk } from "./wakes-helpers.js";
import { sourceSha256 } from "../scripts/source-hash.mjs";

const cfg: HomeConfig = { serveAddr: "127.0.0.1:8110", serveTokenFile: "/nope", serveProjectDirs: [], autarchBin: "autarch" };

function fakeBb(threads: object = {}) {
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
    sdk: { threads },
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
    expect(f.services.sort()).toEqual(["home-card-comments", "home-delegation-check", "home-feed-refresh", "home-move-poll", "home-move-reports", "home-queue", "home-wakes"]);
    expect(f.configure()).toBeTypeOf("function");
    expect(f.cli()).toBeDefined();
    expect(Object.keys(home.handlers).sort()).toEqual(["catchup", "checkMove", "claimMove", "conversation", "conversationUnread", "dismiss", "health", "listAsks", "listRecent", "markAllSeen", "markConversationSeen", "markSeen", "moves", "note", "override", "pick", "queue", "resend", "revokeApproval", "rootRun", "setBinding", "setDelegation", "setViewing", "skipMove", "stats", "waiting"]);
    f.disposers.forEach((d) => d());
  });

  it("setBinding is an RPC only: no CLI verb, so the vizier's rule path cannot reach it (Q5)", async () => {
    const db = new Database(":memory:");
    const handle = createStoreHandle(() => db, {});
    const f = fakeBb();
    const home = wireHome(f.bb, handle, cfg, { serve });
    expect(typeof home.handlers.setBinding).toBe("function");
    for (const verb of ["setBinding", "set-binding", "binding", "bind", "confirm-binding"]) {
      const r = await f.cli()!.run([verb, "--tasks-project-id", "tp1", "--state", "confirmed"], { threadId: "thr-v" });
      expect(JSON.stringify(r)).not.toMatch(/confirmed binding|ok":true/);
    }
    expect(db.prepare("SELECT COUNT(*) AS n FROM project_bindings").get()).toEqual({ n: 0 });
    // The rule verb exists, and it is not the binding write.
    const r = await f.cli()!.run(["rule", "dec", "a", "--reason", "x"], { threadId: "thr-v" });
    expect(JSON.stringify(r)).toMatch(/vizier|not found|usage|unknown/i);
    expect(db.prepare("SELECT COUNT(*) AS n FROM project_bindings").get()).toEqual({ n: 0 });
    f.disposers.forEach((d) => d());
  });

  it("rootRun refuses an unknown card and a missing tasks client, and writes nothing", async () => {
    const db = new Database(":memory:");
    const handle = createStoreHandle(() => db, {});
    const f = fakeBb();
    const home = wireHome(f.bb, handle, cfg, { serve });
    expect(await home.handlers.rootRun({ task_id: "01J0000000000000000000000A" })).toEqual({ ok: false, error: "unknown card" });
    db.prepare("INSERT INTO cards(task_id, state) VALUES ('01J0000000000000000000000A', 'open')").run();
    expect(await home.handlers.rootRun({ task_id: "01J0000000000000000000000A" })).toEqual({ ok: false, error: "tasks unavailable" });
    expect(db.prepare("SELECT COUNT(*) AS n FROM approvals").get()).toEqual({ n: 0 });
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
    // Computed over the plugin root at load (Task 1.10), the value the harness preflight compares.
    expect(h.source_sha256).toMatch(/^[0-9a-f]{64}$/);
    expect(h.source_sha256).toBe(sourceSha256(join(import.meta.dirname, "..")));
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

  it("takes an injected wake sdk in place of bb.sdk.threads", async () => {
    const env = makeEnv();
    try {
      const handle = createStoreHandle(() => new Database(env.file), {});
      const f = fakeBb();
      const sdk = new FakeSdk();
      const own = { projects: async () => env.projects, health: async () => ({}), healthy: async () => true } as unknown as ServeClient;
      const home = wireHome(f.bb, handle, cfg, { serve: own, sdk });
      await new Promise((r) => setTimeout(r, 20));
      // `bb home ask` is retired, so a pre-card ask is filed straight into the store.
      const filed = await env.open().file(ask(env), { threadId: "thr-a" });
      if (!filed.ok) throw new Error(filed.error);
      const id = filed.decision_id;
      const listed = (await home.handlers.listAsks(null)).owed[0];
      const r = await home.handlers.pick({ decision_id: id, option_id: "project", revision: listed.revision, pick_id: "p1" });
      expect(r.status).toBe(201);
      await vi.waitFor(() => expect(sdk.sent.length).toBe(1));
      expect(sdk.sent[0]!.threadId).toBe("thr-a");
      f.disposers.forEach((d) => d());
    } finally {
      env.cleanup();
    }
  });

  it("a pick from the overlay is recorded with surface overlay; the default stays home", async () => {
    const env = makeEnv();
    try {
      const handle = createStoreHandle(() => new Database(env.file), {});
      const f = fakeBb();
      const own = { projects: async () => env.projects, health: async () => ({}), healthy: async () => true } as unknown as ServeClient;
      const home = wireHome(f.bb, handle, cfg, { serve: own, sdk: new FakeSdk() });
      await new Promise((r) => setTimeout(r, 20));
      const filed = await env.open().file(ask(env), { threadId: "thr-a" });
      if (!filed.ok) throw new Error(filed.error);
      const listed = (await home.handlers.listAsks(null)).owed[0];
      const r = await home.handlers.pick({ decision_id: filed.decision_id, option_id: "project", revision: listed.revision, pick_id: "po", surface: "overlay" });
      expect(r.status).toBe(201);
      const row = new Database(env.file, { readonly: true }).prepare("SELECT surface FROM picks WHERE pick_id = 'po'").get();
      expect(row).toEqual({ surface: "overlay" });
      f.disposers.forEach((d) => d());
    } finally {
      env.cleanup();
    }
  });

  it("every RPC result is a JSON value: bb rejects undefined members (found by the real-bb run)", async () => {
    const env = makeEnv();
    try {
      const handle = createStoreHandle(() => new Database(env.file), {});
      const f = fakeBb();
      const own = { projects: async () => env.projects, health: async () => ({}), healthy: async () => true } as unknown as ServeClient;
      const home = wireHome(f.bb, handle, cfg, { serve: own, sdk: new FakeSdk() });
      await new Promise((r) => setTimeout(r, 20));
      const filed = await env.open().file(ask(env), { threadId: "thr-a" });
      if (!filed.ok) throw new Error(filed.error);
      const undefinedAt = (v: unknown, path: string): string | null => {
        if (v === undefined) return path;
        if (Array.isArray(v)) return v.map((x, i) => undefinedAt(x, `${path}[${i}]`)).find((x) => x) ?? null;
        if (v && typeof v === "object") return Object.entries(v).map(([k, x]) => undefinedAt(x, `${path}.${k}`)).find((x) => x) ?? null;
        return null;
      };
      expect(undefinedAt(await home.handlers.listAsks(null), "$")).toBeNull();
      expect(undefinedAt(await home.handlers.catchup(null), "$")).toBeNull();
      expect(undefinedAt(await home.handlers.listRecent({ project: "Autarch", limit: 50 }), "$")).toBeNull();
      f.disposers.forEach((d) => d());
    } finally {
      env.cleanup();
    }
  });

  it("the vizier fallback reads every thread even when the host caps a page below the requested size", async () => {
    const row = (id: string, over: Record<string, unknown> = {}) => ({  id, title: "work", pinnedAt: null, archivedAt: null, deletedAt: null, ...over });
    const all = Array.from({ length: 130 }, (_, i) => row(`thr_${i}`));
    const run = async (extra: ReturnType<typeof row>[]) => {
      const rows = [...all, ...extra];
      const threads = {
        // a host that never returns more than 40 rows per page
        list: async (o: { limit: number; offset: number }) => rows.slice(o.offset, o.offset + Math.min(o.limit, 40)),
        get: async ({ threadId }: { threadId: string }) => rows.find((r) => r.id === threadId) ?? null,
      };
      const db = new Database(":memory:");
      const handle = createStoreHandle(() => db, {});
      const f = fakeBb(threads);
      wireHome(f.bb, handle, cfg, { serve });
      const r = await f.cli()!.run(["handoff", "thr_5"], { threadId: "thr_vizier" });
      f.disposers.forEach((d) => d());
      return JSON.stringify(r);
    };
    // the one pinned vizier thread sits past the first pages: found, so its handoff goes through
    expect(await run([row("thr_vizier", { title: "Masaq' | vizier", pinnedAt: 1 })])).toContain('\\"from\\":\\"thr_vizier\\",\\"to\\":\\"thr_5\\"');
    // a second pinned match hiding past the first pages makes the fallback refuse
    const two = await run([row("thr_vizier", { title: "Masaq' | vizier", pinnedAt: 1 }), row("thr_other", { title: "Masaq' | vizier two", pinnedAt: 2 })]);
    expect(two).toContain("2 pinned, unarchived threads match");
  });
});
