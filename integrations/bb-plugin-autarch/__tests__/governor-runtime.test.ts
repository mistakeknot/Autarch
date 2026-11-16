import { it, expect, vi } from "vitest";
import Database from "better-sqlite3";
import { applyOperator, homeAnswer, fileProtectedCard, loadConfig, timerActive, wireGovernor, prepareInboxes, resolveStorageRoot, readOverrides, type Exec } from "../governor-runtime.js";
import { DEFAULTS } from "../governor-policy.js";
import type { PluginsLike } from "../tasks.js";

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
it("fails preflight for missing roots or zero writable recipients", async () => {
  const threads = [{ id: "coordinator", title: "X | coordinator", providerId: "claude-code" }];
  const disk = { stat: vi.fn(async () => ({ isDirectory: () => true })), access: vi.fn(async () => { throw Error("not writable"); }), appendFile: vi.fn(async () => {}) };
  await expect(prepareInboxes({ ...DEFAULTS, storage_root: "/fixture" }, threads, 0, 1, 1.2, "pace", 0, disk)).rejects.toThrow("No writable");
  expect(disk.appendFile).not.toHaveBeenCalled();
  disk.stat = vi.fn(async () => { throw Error("root missing"); });
  await expect(prepareInboxes({ ...DEFAULTS, storage_root: "/fixture" }, threads, 0, 1, 1.2, "pace", 0, disk)).rejects.toThrow("root missing");
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
it("registers an abortable service and leaves the database untouched while off", async () => {
  let start!: (signal: AbortSignal) => Promise<void>;
  const handle = { ready: vi.fn(() => true), store: vi.fn(() => { throw Error("unexpected store access"); }) };
  const bb = { sdk: { threads: { list: async () => [] } }, background: { service: (name: string, opts: { start: typeof start }) => { expect(name).toBe("quota-governor"); start = opts.start; } }, realtime: { publish: vi.fn() }, log: { warn: vi.fn() } };
  const view = wireGovernor(bb, handle, async () => ({ quota_governor_mode: "off" }));
  const controller = new AbortController(); const run = start(controller.signal);
  await Promise.resolve(); await Promise.resolve(); controller.abort(); await run;
  expect(view().mode).toBe("off"); expect(handle.store).not.toHaveBeenCalled();
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
