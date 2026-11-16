import { describe, it, expect, vi } from "vitest";
import Database from "better-sqlite3";
import { DEFAULTS, choose, applyFloor, readFloor, measurePace, recipients, editOperator } from "../governor-policy.js";
import { Governor, GovernorStore, setHostLimit, atomicJson, type GovernorIO } from "../governor.js";

const now = 1_000_000_000_000, H = 3600000;
const account = (u = .6) => ({ id: "a", provider: "claude", enabled: true, status: "ready", observedAt: now, sevenDayUtilization: u, sevenDayResetAt: now + 84 * H });
function rig() {
  const db = new Database(":memory:");
  const store = new GovernorStore(db);
  const io: GovernorIO = {
    now: () => now, hostname: () => "governed", config: async () => ({ ...DEFAULTS, bb_host: "governed", pace_mode: "cumulative" }),
    mode: async () => "off", timerActive: vi.fn(async () => false), accounts: vi.fn(async () => [account()]),
    readJson: vi.fn(async () => null), inferLevel: async () => 0, answer: vi.fn(async () => null), fileCard: vi.fn(async () => "CARD"),
    apply: vi.fn(async () => {}), notify: vi.fn(async () => {}), writeThrottle: vi.fn(async () => {}), publish: vi.fn(),
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
    expect(io.apply).not.toHaveBeenCalled(); expect(io.notify).not.toHaveBeenCalled(); expect(io.fileCard).not.toHaveBeenCalled(); expect(io.writeThrottle).not.toHaveBeenCalled(); db.close();
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
    await g.tick(); expect(g.view().state?.level).toBe(0); expect(g.view().state?.pending).toBe(true);
    expect(io.notify).not.toHaveBeenCalled(); db.close();
  });
  it("preserves global and other host limits through RPC", async () => {
    const callRpc = vi.fn(async (a: any) => a.method === "getConfiguration" ? { globalLimit: 30, hostOverrides: [{ hostId: "other", limit: 5 }], hosts: [{ id: "mine", name: "governed" }] } : {});
    await setHostLimit({ callRpc }, "governed", 12);
    expect(callRpc.mock.calls[1][0].input).toEqual({ globalLimit: 30, hostOverrides: [{ hostId: "other", limit: 5 }, { hostId: "mine", limit: 12 }] });
  });
  it("writes throttle atomically through the filesystem interface", async () => {
    const calls: unknown[] = []; const fs = { mkdir: async (...a: unknown[]) => { calls.push(["mkdir", ...a]); }, writeFile: async (...a: unknown[]) => { calls.push(["write", ...a]); }, rename: async (...a: unknown[]) => { calls.push(["rename", ...a]); } };
    await atomicJson("/fake/throttle.json", { level: 1 }, fs);
    expect(calls[1]).toEqual(["write", "/fake/throttle.json.tmp", expect.stringContaining('"level": 1'), expect.anything()]);
    expect(calls[2]).toEqual(["rename", "/fake/throttle.json.tmp", "/fake/throttle.json"]);
  });
});
