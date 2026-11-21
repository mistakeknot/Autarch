import { it, expect, vi, afterEach } from "vitest";
import Database from "better-sqlite3";
import { applyOperator, homeAnswer, fileProtectedCard, loadConfig, timerActive, wireGovernor, prepareInboxes, resolveStorageRoot, readOverrides, type Exec } from "../governor-runtime.js";
import { GovernorStore } from "../governor.js";
import { DEFAULTS } from "../governor-policy.js";
import type { PluginsLike } from "../tasks.js";
import { homeMethods } from "../contract.js";
const { readFile } = vi.hoisted(() => ({ readFile: vi.fn(async (_path: unknown, ..._rest: unknown[]): Promise<string> => { throw Object.assign(Error("missing"), { code: "ENOENT" }); }) }));
vi.mock("node:fs/promises", async importOriginal => ({ ...await importOriginal<typeof import("node:fs/promises")>(), readFile }));
afterEach(() => { vi.unstubAllEnvs(); readFile.mockReset(); readFile.mockRejectedValue(Object.assign(Error("missing"), { code: "ENOENT" })); });

it("normalizes future optional quotaGovernor fields at the RPC contract boundary", () => {
  const input = { mode: "shadow", error: undefined, state: { level: 1, pending: undefined }, decision: { held: undefined }, script: { windows: [undefined, { unused: undefined, utilization: .5 }] } };
  const result = homeMethods.quotaGovernor.output.parse(input);
  expect(result).toStrictEqual({ mode: "shadow", state: { level: 1 }, decision: {}, script: { windows: [null, { utilization: .5 }] } });
  expect(JSON.parse(JSON.stringify(result))).toStrictEqual(result);
  expect(Object.hasOwn(input, "error")).toBe(true);
  expect(homeMethods.quotaGovernor.output.safeParse(7).success).toBe(false);
});

it("reads full retained history through the quotaGovernor option while off without creating tables", () => {
  const db = new Database(":memory:");
  const bb = { sdk: { threads: { list: async () => [] } }, background: { service: vi.fn() }, realtime: { publish: vi.fn() }, log: { warn: vi.fn() } };
  const view = wireGovernor(bb, { ready: () => true, store: () => ({ db }) }, async () => ({ quota_governor_mode: "off" }));
  expect(homeMethods.quotaGovernor.input.parse(null)).toBeNull();
  expect(homeMethods.quotaGovernor.input.parse({ history: "full" })).toEqual({ history: "full" });
  expect(homeMethods.quotaGovernor.input.safeParse({ history: "other" }).success).toBe(false);
  expect(view({ history: "full" }).history).toEqual([]);
  expect(db.prepare("SELECT name FROM sqlite_master WHERE type='table'").all()).toEqual([]);
  const store = new GovernorStore(db);
  const row = { time: Date.now() - 72 * 3600000, mode: "shadow" as const, level: 0, new_level: 1, pace: 1.2, note: "older", script_level: 0 };
  store.record(row); store.record({ ...row, mode: "live", note: "live" });
  expect(view().history).toBeUndefined();
  expect(view({ history: "full" }).history?.map(r => r.note)).toEqual(["older", "live"]);
  db.close();
});

it("publishes a JSON-clean full shadow decision through wireGovernor with a ready store", async () => {
  const db = new Database(":memory:"); let start!: (signal: AbortSignal) => Promise<void>;
  const controller = new AbortController();
  const now = Date.now();
  const account = { id: "account", provider: "claude", enabled: true, status: "ready", observedAt: now, sevenDayUtilization: .6, sevenDayResetAt: now + 84 * 3600000 };
  const rpc = vi.fn(async () => [account]); const plugins: PluginsLike = { callRpc: async args => args.outputSchema.parse(await rpc()) };
  const published = vi.fn(() => controller.abort());
  const bb = { sdk: { threads: { list: async () => [] }, plugins }, background: { service: (_name: string, opts: { start: typeof start }) => { start = opts.start; } }, realtime: { publish: published }, log: { warn: vi.fn() } };
  const handle = { ready: () => true, store: vi.fn(() => ({ db })) };
  const view = wireGovernor(bb, handle, async () => ({ quota_governor_mode: "shadow" }), {
    config: async () => ({ ...DEFAULTS, bb_host: "governed", pace_mode: "cumulative", state_dir: "/fixture/state" }), hostname: () => "governed",
    roots: () => { throw Error("shadow must not discover storage"); },
  });
  await start(controller.signal);
  expect(handle.store).toHaveBeenCalledTimes(1); expect(rpc).toHaveBeenCalledTimes(1);
  expect(published).toHaveBeenCalledWith("quota-governor-changed", {});
  expect(view().mode).toBe("shadow"); expect(view().error).toBeUndefined();
  expect(view().decision?.new_level).toBe(1); expect(view().state?.level).toBe(1);
  expect(view().decision?.pace).toBeCloseTo(1.2); expect(view().last_run).toBeTypeOf("number");
  expect(view().state).toStrictEqual(JSON.parse((db.prepare("SELECT json FROM quota_governor_state WHERE key='shadow'").get() as { json: string }).json));
  const { history: _history, ...status } = view();
  expect(status).toStrictEqual(JSON.parse((db.prepare("SELECT json FROM quota_governor_state WHERE key='view'").get() as { json: string }).json));
  expect(view().history).toHaveLength(1);
  expect(JSON.parse(JSON.stringify(view()))).toStrictEqual(view());
  expect(homeMethods.quotaGovernor.output.parse(view())).toStrictEqual(view());
  db.close();
});

it("reads a Citadel pick by the card task's decision id", async () => {
  const db = new Database(":memory:");
  db.exec(`CREATE TABLE cards(task_id TEXT,card_key TEXT); CREATE TABLE picks(decision_id TEXT,option_id TEXT,"by" TEXT,surface TEXT); INSERT INTO cards VALUES('task','CARD'); INSERT INTO picks VALUES('card-task-g1','a','mk','home');`);
  expect(homeAnswer(db, "CARD")).toBe("a"); db.prepare("UPDATE picks SET option_id='b'").run(); expect(homeAnswer(db, "CARD")).toBe("b");
  expect(homeAnswer(db, "missing")).toBeNull(); db.close();
});
it("rejects delegated, automatic and CLI picks for protected work", () => {
  const db = new Database(":memory:");
  db.exec(`CREATE TABLE cards(task_id TEXT,card_key TEXT); CREATE TABLE picks(decision_id TEXT,option_id TEXT,"by" TEXT,surface TEXT); INSERT INTO cards VALUES('task','CARD'); INSERT INTO picks VALUES('card-task-g1','a','mk','home');`);
  for (const by of ["vizier", "auto", "agent", ""]) {
    db.prepare(`UPDATE picks SET "by"=?`).run(by); expect(homeAnswer(db, "CARD")).toBeNull();
  }
  db.prepare(`UPDATE picks SET "by"='mk',surface='cli'`).run(); expect(homeAnswer(db, "CARD")).toBeNull();
  db.prepare(`UPDATE picks SET surface='overlay'`).run(); expect(homeAnswer(db, "CARD")).toBe("a"); db.close();
});
it("files with a title and the vizier caller", async () => {
  let bin = "", argv: string[] = [], options: Parameters<Exec>[2];
  const exec: Exec = async (file, args, opts) => { bin = file; argv = args; options = opts; return { stdout: '{"card_key":"CARD"}' }; };
  const fs = { mkdir: vi.fn(async () => {}), writeFile: vi.fn(async () => {}), rename: vi.fn(async () => {}) };
  await fileProtectedCard({ ...DEFAULTS, state_dir: "/fake", card_project: "fixture" }, 2, 1.8, 1, "vizier", exec, fs, {});
  expect(bin).toBe("autarch"); expect(argv).toContain("--title"); expect(argv[argv.indexOf("--title") + 1]).toContain("level 2"); expect(options?.env?.BB_THREAD_ID).toBe("vizier");
});
it("refuses an unconfigured card destination before any write or command", async () => {
  const exec = vi.fn(async () => ({ stdout: "{}" }));
  const fs = { mkdir: vi.fn(async () => {}), writeFile: vi.fn(async () => {}), rename: vi.fn(async () => {}) };
  await expect(fileProtectedCard(DEFAULTS, 2, 1.8, 0, "vizier", exec, fs, {})).rejects.toThrow("card project");
  expect(exec).not.toHaveBeenCalled(); expect(fs.writeFile).not.toHaveBeenCalled();
});
it("resolves discovered machine thread storage and fails ambiguous roots", async () => {
  expect(resolveStorageRoot(DEFAULTS, () => ["/fixture/machine/thread-storage"]).storage_root).toBe("/fixture/machine/thread-storage");
  expect(() => resolveStorageRoot(DEFAULTS, () => [])).toThrow("thread-storage");
  expect(() => resolveStorageRoot(DEFAULTS, () => ["/a", "/b"])).toThrow("thread-storage");
  const roots = vi.fn(() => { throw Error("unexpected discovery"); });
  expect((await loadConfig({}, async () => null)).storage_root).toBe("");
  expect(resolveStorageRoot({ ...DEFAULTS, storage_root: "/configured" }, roots).storage_root).toBe("/configured"); expect(roots).not.toHaveBeenCalled();
});
it("skips missing recipients and preflights a writable target without appending", async () => {
  const threads = [{ id: "coordinator", title: "X | coordinator", providerId: "claude-code" }, { id: "missing", title: "X steward", providerId: "claude-code" }];
  const disk = { stat: vi.fn(async (p: string) => { if (p.endsWith("missing")) throw Error("recipient missing"); return { isDirectory: () => true }; }), access: vi.fn(async () => {}), appendFile: vi.fn(async () => {}) };
  const plan = await prepareInboxes({ ...DEFAULTS, storage_root: "/fixture" }, threads, 0, 1, 1.2, "pace", 0, disk);
  expect(plan.skipped.join(" ")).toContain("missing"); expect(disk.appendFile).not.toHaveBeenCalled();
  await plan.send(); expect(disk.appendFile).toHaveBeenCalledTimes(1);
});
it("allows zero writable recipients but fails preflight for a missing root", async () => {
  const threads = [{ id: "coordinator", title: "X | coordinator", providerId: "claude-code" }];
  const disk = { stat: vi.fn(async () => ({ isDirectory: () => true })), access: vi.fn(async () => { throw Error("not writable"); }), appendFile: vi.fn(async () => {}) };
  const plan = await prepareInboxes({ ...DEFAULTS, storage_root: "/fixture" }, threads, 0, 1, 1.2, "pace", 0, disk);
  expect(plan.skipped.join(" ")).toContain("No writable"); expect(await plan.send()).toEqual([]);
  expect(disk.appendFile).not.toHaveBeenCalled();
  disk.stat = vi.fn(async () => { throw Error("root missing"); });
  await expect(prepareInboxes({ ...DEFAULTS, storage_root: "/fixture" }, threads, 0, 1, 1.2, "pace", 0, disk)).rejects.toThrow("root missing");
});
it("allows an empty recipient list and skips malformed ids without touching their paths", async () => {
  const disk = { stat: vi.fn(async () => ({ isDirectory: () => true })), access: vi.fn(async () => {}), appendFile: vi.fn(async () => {}) };
  const cfg = { ...DEFAULTS, storage_root: "/fixture" };
  const empty = await prepareInboxes(cfg, [], 0, 1, 1.2, "pace", 0, disk);
  expect(empty.skipped.join(" ")).toContain("No writable"); await empty.send(); expect(disk.appendFile).not.toHaveBeenCalled();
  const plan = await prepareInboxes(cfg, ["../invalid", "valid"].map(id => ({ id, title: "X | coordinator", providerId: "claude-code" })), 0, 1, 1.2, "pace", 0, disk);
  expect(plan.skipped.join(" ")).toContain("Invalid thread id"); await plan.send(); expect(disk.appendFile).toHaveBeenCalledTimes(1);
  expect(JSON.stringify(disk.stat.mock.calls)).not.toContain("invalid");
});
it.each([
  ["setting-host", "setting-host", true],
  ["setting-host", "other-host", false],
  ["", "env-host", true],
] as const)("wires settings host %s through the default loader to local host %s", async (settingHost, localHost, active) => {
  vi.stubEnv("QUOTA_GOVERNOR_CONFIG", "/fixture/config"); vi.stubEnv("QUOTA_GOVERNOR_HOST", "env-host");
  readFile.mockImplementation(async path => {
    if (path === "/fixture/config") return JSON.stringify({ bb_host: "file-host", state_dir: "/fixture/state" });
    if (path === "/fixture/state/state.json") return '{"level":0}';
    throw Object.assign(Error("missing"), { code: "ENOENT" });
  });
  const db = new Database(":memory:"); const controller = new AbortController(); let start!: (signal: AbortSignal) => Promise<void>;
  const call = vi.fn(async () => []);
  const plugins: PluginsLike = { callRpc: async args => args.outputSchema.parse(await call()) };
  const handle = { ready: () => true, store: vi.fn(() => ({ db })) };
  const bb = { sdk: { threads: { list: async () => [] }, plugins }, background: { service: (_name: string, opts: { start: typeof start }) => { start = opts.start; } }, realtime: { publish: () => controller.abort() }, log: { warn: vi.fn() } };
  const settings = vi.fn(async () => ({ quota_governor_mode: "shadow", quota_governor_host: settingHost }));
  const view = wireGovernor(bb, handle, settings, { hostname: () => localHost, roots: () => { throw Error("unexpected storage discovery"); } });
  await start(controller.signal);
  expect(readFile.mock.calls.some(c => c[0] === "/fixture/config")).toBe(true);
  expect(call).toHaveBeenCalledTimes(active ? 1 : 0);
  if (active) { expect(view().error).toBeUndefined(); expect(view().last_run).toBeTypeOf("number"); }
  else { expect(view().error).toBe("Inactive on this host"); expect(handle.store).not.toHaveBeenCalled(); }
  db.close();
});
it("checks directory writability when a recipient has no inbox yet", async () => {
  const disk = { stat: async () => ({ isDirectory: () => true }), access: vi.fn(async (path: string) => { if (path.endsWith("inbox.md")) throw Object.assign(Error("new inbox"), { code: "ENOENT" }); }), appendFile: vi.fn(async () => {}) };
  const plan = await prepareInboxes({ ...DEFAULTS, storage_root: "/fixture" }, [{ id: "c", title: "X | coordinator", providerId: "claude-code" }], 0, 1, 1.2, "pace", 0, disk);
  expect(disk.access.mock.calls.map(a => a[0])).toEqual(["/fixture/c/inbox.md", "/fixture/c"]);
  expect(disk.appendFile).not.toHaveBeenCalled(); await plan.send(); expect(disk.appendFile).toHaveBeenCalledTimes(1);
});
it("reports a partial append and continues to later recipients after commit", async () => {
  const threads = ["a", "b", "c"].map(id => ({ id, title: "X | coordinator", providerId: "claude-code" }));
  const disk = { stat: async () => ({ isDirectory: () => true }), access: async () => {}, appendFile: vi.fn(async (p: string) => { if (p.includes("/b/")) throw Error("append denied"); }) };
  const plan = await prepareInboxes({ ...DEFAULTS, storage_root: "/fixture" }, threads, 0, 1, 1.2, "pace", 0, disk);
  expect((await plan.send()).join(" ")).toContain("append denied"); expect(disk.appendFile).toHaveBeenCalledTimes(3);
});
it("keeps script notice and card wording with only the private ruling reference replaced", async () => {
  const disk = { stat: async () => ({ isDirectory: () => true }), access: async () => {}, appendFile: vi.fn(async (_path: string, _text: string) => {}) };
  await (await prepareInboxes({ ...DEFAULTS, storage_root: "/fixture" }, [{ id: "c", title: "X | coordinator", providerId: "claude-code" }], 0, 1, 1.25, "pace", 0, disk)).send();
  expect(disk.appendFile.mock.calls[0][1]).toContain("Protected work (the one-shots, the Undersketch protected path) keeps its caps unless a card says otherwise.");
  let body = "";
  const fs = { mkdir: async () => {}, writeFile: async (_p: string, s: string) => { body = s; }, rename: async () => {} };
  await fileProtectedCard({ ...DEFAULTS, state_dir: "/fixture", card_project: "fixture" }, 2, 1.8, 0, "vizier", async () => ({ stdout: '{"card_key":"CARD"}' }), fs, {});
  const ask = JSON.parse(body);
  expect(ask.question).toContain("the Babelrooms and Yaoivers one-shots, the Undersketch protected path");
  expect(ask.question).toContain("pace comes back toward the sustainable rate sooner");
  expect(ask.question).toContain("unless the run-out is within two days.");
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
it("refuses a service still running after its timer stops", async () => {
  const run: Exec = async (_file, args) => {
    if (args.at(-1) === "quota-governor.service") return { stdout: "active\n" };
    throw Object.assign(Error("inactive"), { code: 3, stdout: "inactive\n" });
  };
  expect(await timerActive(run)).toBe(true);
});
it("rejects malformed overrides and logs/shows config errors without touching the store", async () => {
  await expect(readOverrides("fixture", async () => "{broken JSON")).rejects.toThrow(SyntaxError);
  await expect(readOverrides("fixture", async () => "null")).rejects.toThrow("object");
  await expect(loadConfig({ QUOTA_GOVERNOR_CONFIG: "fixture" }, async () => { throw new SyntaxError("bad JSON"); })).rejects.toThrow("bad JSON");
  await expect(loadConfig({ QUOTA_GOVERNOR_CONFIG: "fixture" }, async () => [])).rejects.toThrow("object");
  let start!: (signal: AbortSignal) => Promise<void>;
  const controller = new AbortController();
  const handle = { ready: () => true, store: vi.fn(() => { throw Error("unexpected store access"); }) };
  const bb = { sdk: { threads: { list: async () => [] } }, background: { service: (_name: string, opts: { start: typeof start }) => { start = opts.start; } }, realtime: { publish: () => controller.abort() }, log: { warn: vi.fn() } };
  const view = wireGovernor(bb, handle, async () => ({ quota_governor_mode: "live" }), { config: async () => { throw new SyntaxError("bad JSON"); } });
  await start(controller.signal); expect(view().error).toContain("bad JSON"); expect(bb.log.warn).toHaveBeenCalledWith(expect.stringContaining("bad JSON")); expect(handle.store).not.toHaveBeenCalled();
});
it("merges runtime config overrides without embedding a host", async () => {
  expect((await loadConfig({ QUOTA_GOVERNOR_HOST: "governed" }, async () => null)).bb_host).toBe("governed");
  expect((await loadConfig({ QUOTA_GOVERNOR_CONFIG: "/fake/config" }, async () => ({ good_readings: 3, bb_host: "configured" }))).good_readings).toBe(3);
});
it.each([
  [{ quota_governor_host: "setting-host", quota_governor_card_project: "setting-project" }, { QUOTA_GOVERNOR_HOST: "env-host", QUOTA_GOVERNOR_CARD_PROJECT: "env-project" }, "setting-host", "setting-project"],
  [{ quota_governor_host: " ", quota_governor_card_project: "" }, { QUOTA_GOVERNOR_HOST: "env-host", QUOTA_GOVERNOR_CARD_PROJECT: "env-project" }, "env-host", "env-project"],
  [{}, {}, "file-host", "file-project"],
  [{}, { QUOTA_GOVERNOR_HOST: "", QUOTA_GOVERNOR_CARD_PROJECT: " " }, "file-host", "file-project"],
  [{ quota_governor_host: "setting-host" }, { QUOTA_GOVERNOR_CARD_PROJECT: "env-project" }, "setting-host", "env-project"],
] as const)("resolves plugin settings, environment and file in order: %j", async (settings, env, host, project) => {
  const cfg = await loadConfig({ ...env, QUOTA_GOVERNOR_CONFIG: "fixture" }, async () => ({ bb_host: "file-host", card_project: "file-project" }), settings);
  expect([cfg.bb_host, cfg.card_project]).toEqual([host, project]);
});
it("does not let the filer override an already resolved plugin card destination", async () => {
  let project = "";
  const disk = { mkdir: async () => {}, writeFile: async () => {}, rename: async () => {} };
  await fileProtectedCard({ ...DEFAULTS, state_dir: "/fixture", card_project: "setting-project" }, 2, 1.8, 0, "vizier", async (_file, args) => { project = args[args.indexOf("--project") + 1]; return { stdout: '{"card_key":"CARD"}' }; }, disk, { QUOTA_GOVERNOR_CARD_PROJECT: "env-project" });
  expect(project).toBe("setting-project");
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
it.each(["off", "shadow", "error"])("returns a JSON-clean wired %s view, including while the store is not ready", async scenario => {
  let start!: (signal: AbortSignal) => Promise<void>; const controller = new AbortController();
  const bb = { sdk: { threads: { list: async () => [] } }, background: { service: (_name: string, opts: { start: typeof start }) => { start = opts.start; } }, realtime: { publish: () => controller.abort() }, log: { warn: vi.fn() } };
  const handle = { ready: () => scenario === "error", store: () => { throw Error("unexpected store access"); } };
  const view = wireGovernor(bb, handle, async () => ({ quota_governor_mode: scenario === "error" ? "shadow" : scenario }), { config: async () => { throw Error("Config unavailable"); } });
  expect(JSON.parse(JSON.stringify(view()))).toStrictEqual(view());
  await start(controller.signal);
  expect(JSON.parse(JSON.stringify(view()))).toStrictEqual(view());
  if (scenario === "error") expect(view().error).toContain("Config unavailable");
  else expect(Object.hasOwn(view(), "error")).toBe(false);
});
it.each(["off", "other host", "shadow"])("%s does not discover notification storage", async scenario => {
  const db = new Database(":memory:"); let start!: (signal: AbortSignal) => Promise<void>;
  const controller = new AbortController(); const roots = vi.fn(() => { throw Error("unexpected storage discovery"); });
  const plugins: PluginsLike = { callRpc: async args => args.outputSchema.parse([]) };
  const handle = { ready: () => true, store: vi.fn(() => ({ db })) };
  const bb = { sdk: { threads: { list: async () => [] }, plugins }, background: { service: (_name: string, opts: { start: typeof start }) => { start = opts.start; } }, realtime: { publish: () => controller.abort() }, log: { warn: vi.fn() } };
  const view = wireGovernor(bb, handle, async () => ({ quota_governor_mode: scenario === "other host" ? "live" : scenario }), {
    config: async () => ({ ...DEFAULTS, bb_host: "governed", state_dir: "/fixture/state" }), roots, hostname: () => scenario === "other host" ? "other" : "governed",
  });
  await start(controller.signal); expect(roots).not.toHaveBeenCalled(); expect(view().error ?? "").not.toContain("thread-storage");
  if (scenario === "other host") expect(view().error).toBe("Inactive on this host");
  db.close();
});
