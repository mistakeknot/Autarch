// Production adapters. Tests inject filesystem, process and plugin interfaces.
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import * as fs from "node:fs/promises";
import { homedir, hostname } from "node:os";
import { join } from "node:path";
import type Database from "better-sqlite3";
import { z } from "zod";
import { Governor, GovernorStore, atomicJson, setHostLimit, type AtomicFS, type Mode, type View } from "./governor.js";
import { DEFAULTS, editOperator, findVizier, recipients, type Account, type Config, type Thread } from "./governor-policy.js";
import type { PluginsLike } from "./tasks.js";
import type { StoreHandle } from "./store.js";

export type Exec = (file: string, args: string[], options?: { timeout?: number; env?: NodeJS.ProcessEnv; signal?: AbortSignal }) => Promise<{ stdout: string }>;
const realExec: Exec = (file, args, options) => promisify(execFile)(file, args, { timeout: 120000, maxBuffer: 8 * 1024 * 1024, ...options });
async function readJson(path: string): Promise<unknown> { try { return JSON.parse(await fs.readFile(path, "utf8")); } catch (e) { if ((e as NodeJS.ErrnoException).code === "ENOENT" || e instanceof SyntaxError) return null; throw e; } }
export async function loadConfig(env: NodeJS.ProcessEnv = process.env, read = readJson): Promise<Config> {
  const home = homedir();
  const base = { ...structuredClone(DEFAULTS), bb_host: env.QUOTA_GOVERNOR_HOST ?? "", state_dir: join(home, ".local/state/quota-governor"), operator_toml: join(home, ".config/aleph-orchestration/operator.toml"), storage_root: join(env.BB_DATA_DIR ?? join(home, ".bb"), "thread-storage") };
  const overrides = env.QUOTA_GOVERNOR_CONFIG ? await read(env.QUOTA_GOVERNOR_CONFIG) : null;
  return Object.assign(base, overrides ?? {});
}
export async function timerActive(runFile: Exec): Promise<boolean> {
  try { const r = await runFile("systemctl", ["--user", "is-active", "quota-governor.timer"], { timeout: 10000 }); return r.stdout.trim() !== "inactive"; }
  catch (e) {
    const err = e as { code?: number; stdout?: string };
    if ((err.code === 3 && err.stdout?.trim() === "inactive") || (err.code === 4 && err.stdout?.trim() === "unknown")) return false;
    throw Error("Cannot establish legacy timer state; live refused");
  }
}
export function homeAnswer(db: Database.Database, key: string): "a" | "b" | null {
  const row = db.prepare("SELECT p.option_id FROM cards c JOIN picks p ON p.decision_id = 'card-' || c.task_id || '-g1' WHERE c.card_key = ?").get(key) as { option_id: string } | undefined;
  return row?.option_id === "a" || row?.option_id === "b" ? row.option_id : null;
}
export async function fileProtectedCard(cfg: Config, level: number, pace: number | null, renewal: number, vizier: string, runFile: Exec, disk: AtomicFS): Promise<string> {
  const lv = cfg.levels[level];
  const ask = {
    schema: "home-ask/v2", project: "Sylveste",
    project_root: process.env.QUOTA_GOVERNOR_PROJECT_ROOT ?? join(homedir(), "projects", "Sylveste"),
    ask_key: `quota-governor-cut-protected-L${level}${renewal ? `-r${renewal}` : ""}`,
    question: `What this is: the quota governor wants to move to level ${level} (${lv.name}) to stop weekly quota running out before reset (pooled pace ${pace == null ? "unknown" : pace.toFixed(2)}). This level lowers protected work caps.\nWhat you would get: pace returns toward the sustainable rate sooner; protected work slows until it recovers.\nWhy now: the governor holds one level lower until you answer; it never cuts protected work without this card (bead mk-z0lht.1).\n\nCut protected work?\nA) Yes, cut protected work at this level\nB) No, hold at the lower level and accept the overrun\nRecommendation: B unless run-out is within two days.`,
    options: [{ id: "a", kind: "ruling-only", label: "A) Yes, cut protected work at this level" }, { id: "b", kind: "ruling-only", label: "B) No, hold at the lower level" }], recommendation: "b",
  };
  const path = join(cfg.state_dir, `card-ask-L${level}.json`);
  await atomicJson(path, ask, disk);
  const r = await runFile("autarch", ["needs-mk", "file", "--project", process.env.QUOTA_GOVERNOR_CARD_PROJECT ?? "MSQ", "--title", `Quota: cut protected work at level ${level} (${lv.name})?`, "--ask-file", path, "--vizier-owned", "--json"], { timeout: 120000, env: { ...process.env, BB_THREAD_ID: vizier } });
  return z.object({ card_key: z.string().min(1) }).parse(JSON.parse(r.stdout)).card_key;
}
export interface OperatorFS extends AtomicFS {
  readFile(path: string, encoding: "utf8"): Promise<string>;
  copyFile(from: string, to: string): Promise<unknown>;
  readdir(path: string): Promise<string[]>; unlink(path: string): Promise<unknown>;
}
export async function applyOperator(cfg: Config, level: number, force: boolean, runFile: Exec, disk: OperatorFS): Promise<void> {
  const path = cfg.operator_toml, old = await disk.readFile(path, "utf8"), next = editOperator(old, cfg.levels[level].operator);
  if (old === next && !force) return;
  const activate = async () => {
    const r = await runFile("orch", ["--json", "render"], { timeout: 120000 });
    const gen = z.object({ generation: z.string().min(1) }).parse(JSON.parse(r.stdout)).generation;
    await runFile("orch", ["apply", gen], { timeout: 120000 });
  };
  if (old !== next) {
    await disk.mkdir(cfg.state_dir, { recursive: true });
    await disk.copyFile(path, join(cfg.state_dir, `operator.toml.bak-${new Date().toISOString().replace(/[:.]/g, "")}`));
    await disk.writeFile(`${path}.governor.tmp`, next, { mode: 0o600 }); await disk.rename(`${path}.governor.tmp`, path);
  }
  try { await activate(); }
  catch (e) {
    if (old !== next) {
      await disk.writeFile(`${path}.governor.tmp`, old, { mode: 0o600 }); await disk.rename(`${path}.governor.tmp`, path);
      try { await activate(); } catch { /* Outer transition rollback retries, then records pending. */ }
    }
    throw Error(`orch step failed (${String(e)}); operator.toml restored`);
  }
  const backups = (await disk.readdir(cfg.state_dir)).filter(f => f.startsWith("operator.toml.bak-")).sort();
  for (const f of backups.slice(0, -10)) await disk.unlink(join(cfg.state_dir, f));
}
export async function notifyInboxes(cfg: Config, threads: Thread[], old: number, next: number, pace: number | null, why: string, now: number): Promise<void> {
  const lv = cfg.levels[next], p = (pace ?? -1).toFixed(2);
  const msg = next > old
    ? `THROTTLE (quota governor, level ${next} ${lv.name}, pooled weekly pace ${p}): no new executors, child threads, review rounds, research or background slices; finish only in-flight steps; no FYI or status replies. Merge and root-run asks already sent continue. Protected work (the one-shots and protected paths) keeps its caps unless a card says otherwise.`
    : `THROTTLE LIFTED to level ${next} ${lv.name} (quota governor, pooled weekly pace ${p}): ${next === 0 ? "the open caps are back; resume normal work." : `caps back to level ${next}; resume normal work within them.`}`;
  const append = async (id: string, text: string) => {
    if (!/^[A-Za-z0-9_-]+$/.test(id)) throw Error("Invalid thread id");
    const dir = join(cfg.storage_root, id);
    try { if (!(await fs.stat(dir)).isDirectory()) return; } catch (e) { if ((e as NodeJS.ErrnoException).code === "ENOENT") return; throw e; }
    await fs.appendFile(join(dir, "inbox.md"), `${new Date(now).toISOString().slice(0, 16)}Z quota-governor ${text}\n`);
  };
  for (const id of recipients(threads, cfg)) await append(id, msg);
  const vz = findVizier(threads, cfg);
  if (vz) await append(vz, `quota level ${old} ${cfg.levels[old].name} -> ${next} ${lv.name} (pace ${p}; ${why})`);
}
export interface GovernorHost {
  sdk: { threads: { list(input: { limit: number; offset: number; includeHidden: true }): Promise<unknown[]> } };
  background: { service(name: string, opts: { start(signal: AbortSignal): Promise<void> }): unknown };
  realtime: { publish(channel: string, data: Record<string, unknown>): unknown };
  log: { warn(message: string): unknown };
}
export function wireGovernor(bb: GovernorHost, handle: Pick<StoreHandle, "ready" | "store">, settings: () => Promise<{ quota_governor_mode: string }>): () => View {
  let governor: Governor | null = null;
  let last: View = { mode: "off" };
  bb.background.service("quota-governor", {
    async start(signal) {
      const exec: Exec = (file, args, opts) => realExec(file, args, { ...opts, signal });
      const plugins = (bb.sdk as { plugins?: PluginsLike }).plugins;
      const threads = async (): Promise<Thread[]> => {
        const out: Thread[] = [];
        for (let offset = 0; ; ) {
          if (signal.aborted) throw Error("Governor stopped");
          const page = await bb.sdk.threads.list({ limit: 100, offset, includeHidden: true });
          if (!page.length) return out;
          out.push(...z.array(z.object({ id: z.string(), title: z.string().nullable().optional(), providerId: z.string().optional(), pinnedAt: z.unknown().optional(), archivedAt: z.unknown().optional(), deletedAt: z.unknown().optional() })).parse(page)); offset += page.length;
        }
      };
      while (!signal.aborted) {
        try {
          const mode = (await settings()).quota_governor_mode;
          if (!["off", "shadow", "live"].includes(mode)) throw Error("Invalid quota governor mode");
          last = { ...last, mode: mode as Mode, error: undefined };
          if (mode !== "off" && handle.ready()) {
            if (!plugins) throw Error("Plugin RPC unavailable");
            if (!governor) {
              const db = handle.store().db;
              governor = new Governor(new GovernorStore(db), {
                now: Date.now, hostname, config: () => loadConfig(), mode: async () => (await settings()).quota_governor_mode as Mode,
                timerActive: () => timerActive(exec),
                accounts: async () => {
                  const numeric = z.number().finite().nullable().optional();
                  const schema = z.array(z.object({ id: z.string(), provider: z.string(), enabled: z.boolean(), status: z.string(), observedAt: numeric, heldUntil: numeric, sevenDayUtilization: numeric, sevenDayResetAt: numeric, limitWindows: z.array(z.object({ windowMinutes: numeric, utilization: numeric, resetAt: numeric, observedAt: numeric }).passthrough()).nullable().optional() }).passthrough());
                  const r = await plugins.callRpc({ pluginId: "account-pool", method: "account.list", input: null, outputSchema: schema, signal });
                  return schema.parse(r) as Account[];
                },
                readJson,
                inferLevel: async cfg => { try { const text = await fs.readFile(cfg.operator_toml, "utf8"); for (const lv of [...cfg.levels].reverse()) if (editOperator(text, lv.operator) === text) return lv.level; } catch { /* Script's first-run fallback. */ } return 0; },
                answer: async key => homeAnswer(db, key),
                fileCard: async (cfg, level, pace, rn) => { const vz = findVizier(await threads(), cfg); if (!vz) throw Error("Vizier thread not uniquely found"); return fileProtectedCard(cfg, level, pace, rn, vz, exec, fs); },
                apply: async (cfg, level, force) => { await applyOperator(cfg, level, force, exec, fs); await setHostLimit(plugins, cfg.bb_host, cfg.levels[level].bb_limit, signal); },
                notify: async (cfg, old, next, pace, why) => notifyInboxes(cfg, await threads(), old, next, pace, why, Date.now()),
                writeThrottle: (cfg, throttle) => atomicJson(join(cfg.state_dir, "throttle.json"), throttle, fs),
                publish: view => { last = view; bb.realtime.publish("quota-governor-changed", {}); },
              });
            }
            await governor.tick();
          } else bb.realtime.publish("quota-governor-changed", {});
        } catch (e) { last = { ...last, error: String(e) }; bb.log.warn(`quota-governor: ${String(e)}`); bb.realtime.publish("quota-governor-changed", {}); }
        if (signal.aborted) break;
        await new Promise<void>(resolve => {
          const done = () => { clearTimeout(timer); signal.removeEventListener("abort", done); resolve(); };
          const timer = setTimeout(done, 10 * 60 * 1000); signal.addEventListener("abort", done, { once: true });
          if (signal.aborted) done();
        });
      }
    },
  });
  return () => last;
}
