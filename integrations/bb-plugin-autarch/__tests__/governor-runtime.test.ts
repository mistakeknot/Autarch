import { it, expect, vi } from "vitest";
import Database from "better-sqlite3";
import { applyOperator, homeAnswer, fileProtectedCard, loadConfig, timerActive, wireGovernor, type Exec } from "../governor-runtime.js";
import { DEFAULTS } from "../governor-policy.js";

it("reads a Citadel pick by the card task's decision id", async () => {
  const db = new Database(":memory:");
  db.exec("CREATE TABLE cards(task_id TEXT,card_key TEXT); CREATE TABLE picks(decision_id TEXT,option_id TEXT); INSERT INTO cards VALUES('task','CARD'); INSERT INTO picks VALUES('card-task-g1','a');");
  expect(homeAnswer(db, "CARD")).toBe("a"); db.prepare("UPDATE picks SET option_id='b'").run(); expect(homeAnswer(db, "CARD")).toBe("b");
  expect(homeAnswer(db, "missing")).toBeNull(); db.close();
});
it("files with a title and the vizier caller", async () => {
  let bin = "", argv: string[] = [], options: Parameters<Exec>[2];
  const exec: Exec = async (file, args, opts) => { bin = file; argv = args; options = opts; return { stdout: '{"card_key":"CARD"}' }; };
  const fs = { mkdir: vi.fn(async () => {}), writeFile: vi.fn(async () => {}), rename: vi.fn(async () => {}) };
  await fileProtectedCard({ ...DEFAULTS, state_dir: "/fake" }, 2, 1.8, 1, "vizier", exec, fs);
  expect(bin).toBe("autarch"); expect(argv).toContain("--title"); expect(argv[argv.indexOf("--title") + 1]).toContain("level 2"); expect(options?.env?.BB_THREAD_ID).toBe("vizier");
});
it("restores and reactivates operator policy after a failed apply", async () => {
  const old = '[executors]\nper_coordinator_cap = 1\nhost_ceiling = 4\n[executors.priority]\ntop_rung = 2\nhost_ceiling = 6\n[executors.priority.top_rung_overrides]\n"Shadow Work |" = 1\n"Undersketch |" = 5\n';
  const writes: string[] = [];
  const fs = { readFile: async () => old, mkdir: async () => {}, copyFile: async () => {}, writeFile: async (_p: string, t: string) => { writes.push(t); }, rename: async () => {}, readdir: async () => [], unlink: async () => {} };
  let applies = 0;
  const exec = vi.fn(async (_bin: string, args: string[]) => { if (args[0] === "apply" && ++applies === 1) throw Error("reloader down"); return { stdout: '{"generation":"g"}' }; });
  await expect(applyOperator({ ...DEFAULTS, operator_toml: "/fake/operator", state_dir: "/fake" }, 0, false, exec, fs)).rejects.toThrow("restored");
  expect(writes.at(-1)).toBe(old); expect(applies).toBe(2);
});
it("fails closed for an indeterminate timer state", async () => {
  await expect(timerActive(async () => { throw Object.assign(Error("unavailable"), { code: 1 }); })).rejects.toThrow();
  expect(await timerActive(async () => { throw Object.assign(Error("inactive"), { code: 3, stdout: "inactive\n" }); })).toBe(false);
  expect(await timerActive(async () => ({ stdout: "active\n" }))).toBe(true);
});
it("merges runtime config overrides without embedding a host", async () => {
  expect((await loadConfig({ QUOTA_GOVERNOR_HOST: "governed" }, async () => null)).bb_host).toBe("governed");
  expect((await loadConfig({ QUOTA_GOVERNOR_CONFIG: "/fake/config" }, async () => ({ good_readings: 3, bb_host: "configured" }))).good_readings).toBe(3);
});
it("registers an abortable service and leaves the database untouched while off", async () => {
  let start!: (signal: AbortSignal) => Promise<void>;
  const handle = { ready: vi.fn(() => true), store: vi.fn(() => { throw Error("unexpected store access"); }) };
  const bb = { sdk: { threads: { list: async () => [] } }, background: { service: (name: string, opts: { start: typeof start }) => { expect(name).toBe("quota-governor"); start = opts.start; } }, realtime: { publish: vi.fn() }, log: { warn: vi.fn() } };
  const view = wireGovernor(bb, handle, async () => ({ quota_governor_mode: "off" }));
  const controller = new AbortController(); const run = start(controller.signal);
  await Promise.resolve(); await Promise.resolve(); controller.abort(); await run;
  expect(view().mode).toBe("off"); expect(handle.store).not.toHaveBeenCalled();
});
