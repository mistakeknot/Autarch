// Drives the a9853e2 (schema 2) Home through its real wiring: wireHome, the registered `home` CLI and the RPC
// handlers, over a store handle on a real database file. Test-only; the v2 sources come from git (v2build.ts).
import Database from "better-sqlite3";
import { vi } from "vitest";
import type { ServeClient } from "../serve.js";
import type { Env } from "./service-helpers.js";
import { loadV2Home } from "./v2build.js";
import { FakeSdk } from "./wakes-helpers.js";

type CliResult = { exitCode: number; stdout?: string; stderr?: string };

export function fakeBb() {
  const disposers: (() => void)[] = [];
  let cli: { run(argv: string[], ctx: object): unknown } | undefined;
  const bb = {
    log: { info: vi.fn(), warn: vi.fn(), error: vi.fn() },
    onDispose: (fn: () => void) => disposers.push(fn),
    background: { service: () => undefined },
    events: { on: () => undefined },
    agents: { configure: () => undefined },
    cli: { register: (r: typeof cli) => (cli = r) },
    sdk: { threads: {} },
    realtime: { publish: () => undefined },
  };
  return { bb: bb as never, disposers, cli: () => cli! };
}

export interface V2Home {
  /** The registered `bb home` CLI, as bb would call it. */
  run: (argv: string[], ctx?: { threadId?: string }) => Promise<CliResult>;
  handlers: Record<string, (i: never) => Promise<unknown>>;
  ready: () => boolean;
  error: () => string | null;
  sdk: FakeSdk;
  close: () => void;
}

/** Start the v2 Home on env.file. When the store refuses (SchemaTooNewError) ready() is false and nothing runs. */
export async function startV2Home(env: Env): Promise<V2Home> {
  const v2 = await loadV2Home();
  const db = new Database(env.file);
  const handle = v2.createStoreHandle(() => db, { log: { info() {}, warn() {}, error() {} } as never });
  const f = fakeBb();
  const sdk = new FakeSdk();
  const serve = {
    projects: async () => env.projects,
    health: async () => ({ build: { commit: "a9853e2" } }),
    healthy: async () => true,
  } as unknown as ServeClient;
  const home = v2.wireHome(f.bb, handle, { serveAddr: "127.0.0.1:1", serveTokenFile: "/nope", serveProjectDirs: [], autarchBin: "autarch" }, { serve, sdk });
  return {
    run: (argv, ctx = {}) => Promise.resolve(f.cli().run(argv, ctx) as CliResult),
    handlers: home.handlers as never,
    ready: () => handle.ready(),
    error: () => handle.error(),
    sdk,
    close() {
      f.disposers.forEach((d) => d());
      handle.dispose();
      if (handle.ready()) handle.store().close();
      else {
        try {
          db.close();
        } catch {
          /* already closed */
        }
      }
    },
  };
}

export interface V2Fixture {
  /** An open decide ask filed by thr-a and mentioned by thr-b and thr-c. */
  decide: string;
  mycroft: string;
  steps: string;
  /** Owned by thr-own, with a progress report from thr-own. */
  machine: string;
  /** The operator picked it; the ruling file is still pending (the project root was read-only at pick time). */
  picked: string;
  /** The vizier picked it (ruling file written). */
  delegated: string;
  withdrawn: string;
  vizier: string;
}

/**
 * A populated v2 database, created only through the a9853e2 build's own entry points: the registered `home`
 * CLI and the RPC handlers. The project root is left read-only (0555) so the picked ask's ruling file stays pending.
 */
export async function populateV2(env: Env, home: V2Home): Promise<V2Fixture> {
  const { chmodSync } = await import("node:fs");
  const { join } = await import("node:path");
  const { ask } = await import("./service-helpers.js");
  const vizier = "thr-viz";
  const fileAsk = async (req: Record<string, unknown>, threadId?: string): Promise<{ id: string; mentioned: boolean }> => {
    const r = await home.run(["ask", "--request", JSON.stringify(req)], threadId ? { threadId } : {});
    if (r.exitCode !== 0) throw new Error(`ask failed (${r.exitCode}): ${r.stderr}`);
    return JSON.parse(r.stdout!) as { id: string; mentioned: boolean };
  };
  const revisionOf = async (id: string): Promise<string> => {
    const l = (await home.handlers.listAsks(null as never)) as { owed: { id: string; revision: string }[] };
    const row = l.owed.find((o) => o.id === id);
    if (!row) throw new Error(`${id} is not owed`);
    return row.revision;
  };

  // Delegated first, while the root is writable, so its ruling file is written.
  const delegated = (await fileAsk(ask(env, { thread: "thr-d", subject: "autarch/delegated: routine", question: "Which routine order?" }), "thr-d")).id;
  const s = (await home.handlers.setDelegation({ vizierThreadId: vizier, projects: ["Autarch"], dailyCap: 5 } as never)) as { ok: boolean; item: string };
  if (!s.ok) throw new Error("setDelegation refused");
  await home.handlers.markSeen({ item: s.item } as never);
  const ruled = await home.run(["rule", delegated, "project", "--reason", "reversible and routine"], { threadId: vizier });
  if (ruled.exitCode !== 0) throw new Error(`rule failed: ${ruled.stderr}`);

  chmodSync(env.roots.Autarch!, 0o555);
  chmodSync(join(env.roots.Autarch!, "docs", "decisions"), 0o555);
  const picked = (await fileAsk(ask(env, { thread: "thr-p", subject: "autarch/picked: pending ruling", question: "Which picked option?" }), "thr-p")).id;
  const pk = (await home.handlers.pick({ decision_id: picked, option_id: "day", revision: await revisionOf(picked), pick_id: "pick-fixture-1" } as never)) as { ok: boolean };
  if (!pk.ok) throw new Error("pick refused");

  const decide = (await fileAsk(ask(env, { thread: "thr-a" }), "thr-a")).id;
  for (const t of ["thr-b", "thr-c"]) {
    const m = await fileAsk(ask(env, { thread: t }), t);
    if (!m.mentioned || m.id !== decide) throw new Error("mention did not fold into the first ask");
  }
  const my = ask(env, { asker: "mycroft", subject: "autarch/mycroft: pull", question: "Mycroft asks: which?", options: [{ id: "a", label: "A" }, { id: "b", label: "B" }] });
  delete my.thread;
  const mycroft = (await fileAsk(my)).id;
  const steps = (await fileAsk(ask(env, { kind: "steps", options: undefined, steps: ["do a", "do b"], thread: "thr-s", subject: "autarch/steps: runbook", question: "steps for the runbook" }), "thr-s")).id;
  const machineAsk = (thread: string, subject: string, machine: Record<string, unknown>) => ask(env, { kind: "machine", options: undefined, machine: { class: "ci", detail: "runner is down", ...machine }, thread, subject, question: subject });
  const machine = (await fileAsk(machineAsk("thr-m", "ci: runner down", { owner_thread: "thr-own" }), "thr-m")).id;
  const prog = await home.run(["progress", machine, "--note", "restarting the runner"], { threadId: "thr-own" });
  if (prog.exitCode !== 0) throw new Error(`progress failed: ${prog.stderr}`);
  const withdrawn = (await fileAsk(machineAsk("thr-w", "ci: withdrawn blocker", { class: "disk", detail: "disk full", owner_thread: "thr-own" }), "thr-w")).id;
  const w = await home.run(["withdraw", withdrawn], { threadId: "thr-w" });
  if (w.exitCode !== 0) throw new Error(`withdraw failed: ${w.stderr}`);
  return { decide, mycroft, steps, machine, picked, delegated, withdrawn, vizier };
}
