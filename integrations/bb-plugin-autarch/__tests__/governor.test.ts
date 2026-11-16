import { describe, it, expect, vi } from "vitest";
import Database from "better-sqlite3";
import { DEFAULTS, choose, applyFloor, readFloor, measurePace, recipients, editOperator } from "../governor-policy.js";
import { Governor, GovernorStore, setHostLimit, atomicJson, type GovernorIO } from "../governor.js";
import type { PluginsLike } from "../tasks.js";

const now = 1_000_000_000_000, H = 3600000;
const account = (u = .6) => ({ id: "a", provider: "claude", enabled: true, status: "ready", observedAt: now, sevenDayUtilization: u, sevenDayResetAt: now + 84 * H });
function rig() {
  const db = new Database(":memory:");
  const store = new GovernorStore(db);
  const io: GovernorIO = {
    now: () => now, hostname: () => "governed", config: async () => ({ ...DEFAULTS, bb_host: "governed", pace_mode: "cumulative" }),
    mode: async () => "off", timerActive: vi.fn(async () => false), accounts: vi.fn(async () => [account()]),
    readJson: vi.fn(async () => null), inferLevel: async () => 0, answer: vi.fn(async () => null), fileCard: vi.fn(async () => "CARD"),
    apply: vi.fn(async () => {}), prepareNotify: vi.fn(async () => ({ skipped: [], send: async () => [] })), writeThrottle: vi.fn(async () => {}), publish: vi.fn(),
  };
  return { db, store, io, g: new Governor(store, io) };
}
describe("governor policy", () => {
  it("tightens immediately and loosens only after two good readings", () => {
    expect(choose(0, 1.5, 0, DEFAULTS).slice(0, 2)).toEqual([2, 0]);
    expect(choose(1, .9, 0, DEFAULTS).slice(0, 2)).toEqual([1, 1]);
    expect(choose(1, .9, 1, DEFAULTS).slice(0, 2)).toEqual([0, 0]);
    expect(choose(1, null, 1, DEFAULTS).slice(0, 2)).toEqual([1, 1]);
  });
  it("holds at the floor without banking good readings", () => {
    const f = { level: 1, until: new Date(now + H).toISOString(), source: "operator" };
    expect(applyFloor(1, 0, 0, "low", f).slice(0, 2)).toEqual([1, 0]);
    expect(applyFloor(1, 1, 1, "low", f).slice(0, 2)).toEqual([1, 0]);
    expect(readFloor(f, now + H, DEFAULTS)).toBeNull();
    expect(readFloor({ ...f, level: true }, now, DEFAULTS)).toBeNull();
  });
  it("reads Python ISO dates, basic dates, week dates and colonless offsets", () => {
    const time = Date.parse("2026-11-19T00:00:00Z");
    for (const until of ["2026-11-20", "20261120", "2026-11-20T00:00:00+0000", "20261120T000000+00", "2026-W47-5", "2026W475", "2026-11-20 00:00:00.123456+00:00"]) {
      expect(readFloor({ level: 1, until }, time, DEFAULTS), until).not.toBeNull();
    }
    for (const until of ["2026-02-30", "2026-11-20T24:00", "2026-W54-1", "2026-11", "soon"]) {
      expect(readFloor({ level: 1, until }, time, DEFAULTS), until).toBeNull();
    }
  });
  it("samples only current weekly windows and requires two hours", () => {
    const a = account();
    expect(measurePace([a], now, DEFAULTS, {})[0]).toBeNull();
    expect(measurePace([a], now, DEFAULTS, { a: [[now - 2.9 * H, .54, a.sevenDayResetAt]] })[0]).toBeGreaterThan(1.35);
    expect(measurePace([a], now, DEFAULTS, { a: [[now - 2.9 * H, .54, a.sevenDayResetAt - 168 * H]] })[0]).toBeNull();
    expect(measurePace([{ ...a, enabled: false }], now, DEFAULTS, {})[0]).toBeNull();
    expect(measurePace([{ ...a, observedAt: now - H }], now, DEFAULTS, {})[0]).toBeNull();
  });
  it("selects only live Claude coordinators and stewards", () => {
    expect(recipients([
      { id: "yes", title: "X | coordinator", providerId: "claude-code" },
      { id: "old", title: "X steward", providerId: "claude-code", archivedAt: 1 },
      { id: "codex", title: "X steward", providerId: "codex" },
      { id: "vizier", title: "Masaq' | vizier steward", providerId: "claude-code" },
    ], DEFAULTS)).toEqual(["yes"]);
  });
  it("refuses incomplete operator files", () => expect(() => editOperator("[executors]\nhost_ceiling = 4", DEFAULTS.levels[1].operator)).toThrow());
});
describe("governor service", () => {
  it("off and other hosts perform no reads or writes", async () => {
    const { g, io, db } = rig(); await g.tick();
    expect(io.accounts).not.toHaveBeenCalled(); expect(io.timerActive).not.toHaveBeenCalled();
    io.mode = async () => "live"; io.hostname = () => "other"; await g.tick();
    expect(io.accounts).not.toHaveBeenCalled(); expect(io.writeThrottle).not.toHaveBeenCalled(); db.close();
  });
  it("refuses live when the legacy timer is active", async () => {
    const { g, io, db } = rig(); io.mode = async () => "live"; io.timerActive = async () => true;
    await g.tick(); expect(g.view().error).toContain("timer");
    expect(io.accounts).not.toHaveBeenCalled(); expect(io.writeThrottle).not.toHaveBeenCalled(); db.close();
  });
  it("shadow computes transitions and stores samples without external writes", async () => {
    const { g, io, store, db } = rig(); io.mode = async () => "shadow"; await g.tick();
    expect(g.view().decision?.new_level).toBe(1); expect(store.samples(now, DEFAULTS).a).toHaveLength(1);
    expect(io.apply).not.toHaveBeenCalled(); expect(io.prepareNotify).not.toHaveBeenCalled(); expect(io.fileCard).not.toHaveBeenCalled(); expect(io.writeThrottle).not.toHaveBeenCalled(); db.close();
  });
  it("migrates live state once, holds protected work, and accepts a Home pick", async () => {
    const { g, io, db } = rig(); io.mode = async () => "live";
    io.readJson = async p => p.endsWith("state.json") ? { level: 1, card: { key: "CARD", level: 2 } } : null;
    io.accounts = async () => [account(.9)]; await g.tick();
    expect(io.apply).not.toHaveBeenCalled(); expect(g.view().decision?.held).toContain("CARD");
    io.answer = async () => "a"; await g.tick(); expect(io.apply).toHaveBeenCalledWith(expect.anything(), 2, false);
    expect(g.view().state?.level).toBe(2); expect(io.writeThrottle).toHaveBeenCalled(); db.close();
  });
  it("failed transitions roll back and preserve pending reconciliation", async () => {
    const { g, io, db } = rig(); io.mode = async () => "live"; io.apply = vi.fn(async () => { throw Error("down"); });
    const send = vi.fn(async () => []); io.prepareNotify = async () => ({ skipped: [], send });
    await g.tick(); expect(g.view().state?.level).toBe(0); expect(g.view().state?.pending).toBe(true);
    expect(send).not.toHaveBeenCalled();
    db.close();
  });
  it("does not apply or roll back policy when notification preflight fails on repeated ticks", async () => {
    const { g, io, db } = rig(); io.mode = async () => "live";
    io.prepareNotify = async () => { throw Error("storage root missing"); };
    await g.tick(); await g.tick(); expect(g.view().state?.level).toBe(0); expect(g.view().error).toContain("storage root missing");
    expect(io.apply).not.toHaveBeenCalled(); expect(g.view().decision?.applied).not.toContain("applied level 1"); db.close();
  });
  it.each(["read", "write", "both"])("still sends committed notices after a throttle %s failure", async failure => {
    const { g, io, store, db } = rig(); io.mode = async () => "live";
    const send = vi.fn(async () => { expect(store.get<{ level: number }>("live")?.level).toBe(1); return []; });
    io.prepareNotify = async () => ({ skipped: [], send });
    if (failure !== "write") io.readJson = async path => { if (path.endsWith("throttle.json")) throw Error("throttle read denied"); return null; };
    if (failure !== "read") io.writeThrottle = async () => { throw Error("throttle write denied"); };
    await g.tick(); expect(send).toHaveBeenCalledTimes(1); expect(g.view().state?.level).toBe(1);
    expect(io.apply).toHaveBeenCalledTimes(1); expect(g.view().error).toContain(failure === "write" ? "throttle write denied" : "throttle read denied");
    if (failure === "both") expect(g.view().error).toContain("throttle write denied");
    expect(store.get<{ error?: string }>("view")?.error).toBe(g.view().error); db.close();
  });
  it("applies a transition with no reachable recipients", async () => {
    const { g, io, db } = rig(); io.mode = async () => "live";
    io.prepareNotify = async () => ({ skipped: ["No writable notification recipients"], send: async () => [] });
    await g.tick(); expect(g.view().state?.level).toBe(1); expect(io.apply).toHaveBeenCalledTimes(1);
    expect(g.view().warnings).toContain("No writable notification recipients"); db.close();
  });
  it("commits before notices and never rolls back a committed level after a partial append", async () => {
    const { g, io, store, db } = rig(); io.mode = async () => "live";
    const sent: number[] = [];
    io.prepareNotify = async () => ({ skipped: ["Skipped recipient missing"], send: async () => {
      expect(store.get<{ level: number }>("live")?.level).toBe(1);
      sent.push(1); throw Error("second append failed");
    } });
    await g.tick(); expect(sent).toEqual([1]); expect(g.view().state?.level).toBe(1);
    expect(io.apply).toHaveBeenCalledTimes(1); expect(g.view().warnings).toContain("Skipped recipient missing");
    expect(g.view().warnings?.join(" ")).toContain("second append failed"); db.close();
  });
  it("holds protected work and reports an unconfigured card project", async () => {
    const { g, io, store, db } = rig(); io.mode = async () => "live"; store.put("live", { level: 1 });
    io.accounts = async () => [account(.9)]; io.fileCard = async () => { throw Error("Governor card project is required"); };
    await g.tick(); expect(g.view().state?.level).toBe(1); expect(g.view().error).toContain("card project"); expect(io.apply).not.toHaveBeenCalled(); db.close();
  });
  it("declines last twelve hours without extending on each poll, then renew", async () => {
    const { g, io, store, db } = rig(); io.mode = async () => "live"; io.accounts = async () => [account(.9)];
    const first = now / 1000 - 3600;
    store.put("live", { level: 1, card: { key: "CARD", level: 2, answered: "b", declined_at: first } });
    io.answer = async () => "b"; await g.tick();
    expect(g.view().state?.card?.declined_at).toBe(first); expect(io.fileCard).not.toHaveBeenCalled();
    io.now = () => now + 12 * H; io.accounts = async () => [{ ...account(.9), observedAt: now + 12 * H }];
    await g.tick(); expect(io.fileCard).toHaveBeenCalledWith(expect.anything(), 2, expect.anything(), 1); db.close();
  });
  it("pool failures preserve throttle age and don't write an apparently fresh decision", async () => {
    const { g, io, db } = rig(); io.mode = async () => "live"; io.accounts = async () => { throw Error("pool down"); };
    await g.tick(); expect(io.writeThrottle).not.toHaveBeenCalled(); expect(g.view().error).toContain("pool down"); db.close();
  });
  it("sample history excludes old observations even if collected now", () => {
    const { store, db } = rig();
    store.sample([{ ...account(), observedAt: now - 12 * H }], now, DEFAULTS);
    expect(store.samples(now, DEFAULTS)).toEqual({});
    store.sample([account()], now, DEFAULTS); expect(store.samples(now, DEFAULTS).a).toHaveLength(1);
    expect(store.samples(now + 4 * H, DEFAULTS)).toEqual({}); db.close();
  });
  it("preserves global and other host limits through RPC", async () => {
    const callRpc = vi.fn(async (a: any) => a.method === "getConfiguration" ? { globalLimit: 30, hostOverrides: [{ hostId: "other", limit: 5 }], hosts: [{ id: "mine", name: "governed" }] } : {});
    const plugins: PluginsLike = { callRpc: async args => args.outputSchema.parse(await callRpc(args)) };
    await setHostLimit(plugins, "governed", 12);
    expect(callRpc.mock.calls[1][0].input).toEqual({ globalLimit: 30, hostOverrides: [{ hostId: "other", limit: 5 }, { hostId: "mine", limit: 12 }] });
  });
  it("writes throttle atomically through the filesystem interface", async () => {
    const calls: unknown[] = []; const fs = { mkdir: async (...a: unknown[]) => { calls.push(["mkdir", ...a]); }, writeFile: async (...a: unknown[]) => { calls.push(["write", ...a]); }, rename: async (...a: unknown[]) => { calls.push(["rename", ...a]); } };
    await atomicJson("/fake/throttle.json", { level: 1 }, fs);
    expect(calls[1]).toEqual(["write", "/fake/throttle.json.tmp", expect.stringContaining('"level": 1'), expect.anything()]);
    expect(calls[2]).toEqual(["rename", "/fake/throttle.json.tmp", "/fake/throttle.json"]);
  });
});
