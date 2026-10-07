// The scenario harness (Task 1.10). Fake mode drives the real plugin wiring with a fake bb;
// real-bb mode is checked but its driver arrives with Task 1.11 (it needs an isolated bb server).
//   tsx e2e/harness.ts --mode fake --run-id <uuid> --out <file.jsonl> [--scenarios a,b]
//   tsx e2e/harness.ts --mode real-bb --owned-server <bb-app copy> --build <file> --install --run-id <uuid> --out <file>  (inside bwrap --unshare-net)
import { resolveBin, rigExecSync } from "./rigexec.js";
import { appendFileSync, existsSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { scenarios } from "./scenarios/index.js";

const here = dirname(fileURLToPath(import.meta.url));
const flags = new Map<string, string>();
const argv = process.argv.slice(2);
const BOOLEAN = new Set(["--install"]);
for (let i = 0; i < argv.length; i++) {
  if (BOOLEAN.has(argv[i]!)) flags.set(argv[i]!.slice(2), "true");
  else flags.set(argv[i]!.replace(/^--/, ""), argv[++i] ?? "");
}
const usage = (m: string): never => {
  process.stderr.write(`e2e: ${m}\nusage: e2e --mode fake|real-bb --run-id <uuid> --out <file> [--scenarios a,b]\n`);
  process.exit(2);
};
const mode = flags.get("mode");
const runId = flags.get("run-id");
const out = flags.get("out");
if (mode !== "fake" && mode !== "real-bb") usage("--mode must be fake or real-bb");
if (!runId) usage("--run-id is required");
if (!out) usage("--out is required");
if (flags.has("owned-server") && mode !== "real-bb") usage("--owned-server is for --mode real-bb");
if (mode === "real-bb" && (!flags.get("build") || !flags.get("install"))) usage("real-bb needs --build <build file> and --install");

if (mode === "real-bb") {
  // Before any spawn: the isolation checks of Task 2.12 (finding r5-3). Outside the bwrap namespace, or with
  // a LISTEN socket already in it, the run is refused.
  const { requireNetns } = await import("./ownership.js");
  try {
    requireNetns(process.env);
  } catch (e) {
    usage(e instanceof Error ? e.message : String(e));
  }
  // The operator-launched server mode is gone: every real-bb run needs a server this harness started and proved (A7).
  if (!flags.get("owned-server")) usage("real-bb needs --owned-server <bb-app copy> (also HOME_E2E_BB_APP): the harness launches and proves its own server");
}

const git = (...a: string[]) => rigExecSync("git", a, { cwd: here }).stdout.trim();
const top = git("rev-parse", "--show-toplevel");
// Evidence must not land inside the tree it describes: it would make the tree dirty.
const rel = relative(top, resolve(out!));
if (!rel.startsWith("..") && !rel.startsWith(sep)) usage("--out must be outside the git worktree");

const commit = git("rev-parse", "HEAD");
const tree = git("rev-parse", "HEAD^{tree}");
const dirty = git("status", "--porcelain").length > 0;

const wanted =
  flags.get("scenarios")?.split(",") ?? (mode === "real-bb" ? ["filer-from-thread", "poller-refresh", "comment-arrives-later-real", "cross-project-panel", "answer-instruction", "queued-then-archived", "vizier-chat", "upgrade-quiesce"] : Object.keys(scenarios).sort());
writeFileSync(out!, "");
let failed = 0;
if (mode === "real-bb") {
  const { Real } = await import("./real.js");
  const { launchOwned } = await import("./launcher.js");
  const build = JSON.parse(readFileSync(flags.get("build")!, "utf8"));
  if (build.commit !== commit) usage(`build file commit ${build.commit} is not HEAD ${commit}`);
  const appArg = flags.get("owned-server")!;
  if (process.env.HOME_E2E_BB_APP && resolve(process.env.HOME_E2E_BB_APP) !== resolve(appArg)) usage("--owned-server and HOME_E2E_BB_APP name different apps");
  let owned: Awaited<ReturnType<typeof launchOwned>>;
  try {
    owned = await launchOwned({ app: appArg, netnsIsolated: true, toolchainSeed: process.env.HOME_E2E_TOOLCHAIN_SEED });
  } catch (e) {
    process.stderr.write(`ABORT ${e instanceof Error ? e.message : String(e)}\n`);
    process.exit(1);
  }
  const t = owned.target;
  const real = new Real({ url: t.url, hostPort: t.hostPort, data: t.dataDir, home: t.home, cli: owned.bbCli }, runId!, build, t);
  const recorded: string[] = [];
  let loaded: unknown;
  try {
    loaded = await real.setup();
  } catch (e) {
    process.stderr.write(`ABORT ${e instanceof Error ? e.message : String(e)}\n`);
    await real.cleanup([]);
    await owned.stop();
    process.exit(1);
  }
  const lines: Record<string, unknown>[] = [];
  for (const name of wanted) {
    const run = real.scenarios[name];
    if (!run) usage(`unknown real-bb scenario ${name}`);
    const started = Date.now();
    let line: Record<string, unknown>;
    try {
      line = { pass: true, evidence: await run!() };
    } catch (e) {
      failed++;
      line = { pass: false, error: e instanceof Error ? e.message : String(e), evidence: {} };
    }
    recorded.push(...real.threads.filter((t) => !recorded.includes(t)));
    lines.push({ scenario: name, mode, run_id: runId, commit, tree, dirty, loaded, ...line });
    process.stderr.write(`${line.pass ? "PASS" : "FAIL"} ${name} (${((Date.now() - started) / 1000).toFixed(1)} s)${line.pass ? "" : `: ${line.error}`}\n`);
  }
  const cleanup = await real.cleanup(recorded);
  const halt = await owned.stop();
  process.stderr.write(`stop: stopped=${halt.stopped} killed=${halt.killed.length} exit=${halt.stop_exit}\n`);
  // Records are written after teardown so each one carries the rig proof, the stop result and its own archived threads.
  for (const l of lines) {
    const ev = (l.evidence ?? {}) as Record<string, unknown>;
    const threads = Array.isArray(ev.threads) ? (ev.threads as string[]) : [];
    l.evidence = { ...ev, rig: { ...owned.rig, stopped: halt.stopped, killed: halt.killed }, cleanup: { archived: threads.filter((x) => cleanup.archived.includes(x)) } };
    appendFileSync(out!, `${JSON.stringify(l)}\n`);
  }
  writeFileSync(`${out}.cleanup.json`, JSON.stringify({ run_id: runId, threads: recorded, ...cleanup }, null, 2) + "\n");
  process.stderr.write(`cleanup: archived ${cleanup.archived.length}, failed ${cleanup.failed.length}\n`);
  process.exit(failed || cleanup.failed.length || !halt.stopped ? 1 : 0);
}

const scratch = mkdtempSync(join(tmpdir(), "autarch-e2e-build-"));
const autarch = join(scratch, "autarch");
const goBin = resolveBin("go", process.env.PATH ?? "");
if (!goBin) usage("go is not on PATH");
const built = rigExecSync(goBin!, ["build", "-buildvcs=false", "-o", autarch, "./cmd/autarch"], {
  cwd: top,
  env: { GOCACHE: process.env.GOCACHE ?? join(scratch, "gocache"), GOMODCACHE: process.env.GOMODCACHE ?? join(process.env.GOPATH ?? join(process.env.HOME ?? "", "go"), "pkg", "mod"), GOFLAGS: "-mod=mod" },
});
if (built.code !== 0) usage(`go build failed: ${built.stderr}`);
for (const name of wanted) {
  const run = scenarios[name];
  if (!run) usage(`unknown scenario ${name}`);
  const started = Date.now();
  let line: Record<string, unknown>;
  try {
    const evidence = await run!({ autarch });
    line = { pass: true, evidence };
  } catch (e) {
    failed++;
    line = { pass: false, error: e instanceof Error ? e.message : String(e), evidence: {} };
  }
  appendFileSync(out!, `${JSON.stringify({ scenario: name, mode, run_id: runId, commit, tree, dirty, loaded: null, ...line })}\n`);
  process.stderr.write(`${line.pass ? "PASS" : "FAIL"} ${name} (${((Date.now() - started) / 1000).toFixed(1)} s)${line.pass ? "" : `: ${line.error}`}\n`);
}
process.exit(failed ? 1 : 0);
