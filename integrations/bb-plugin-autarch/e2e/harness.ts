// The scenario harness (Task 1.10). Fake mode drives the real plugin wiring with a fake bb;
// real-bb mode is checked but its driver arrives with Task 1.11 (it needs an isolated bb server).
//   tsx e2e/harness.ts --mode fake --run-id <uuid> --out <file.jsonl> [--scenarios a,b]
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
  if (flags.has("owned-server")) {
    const app = process.env.HOME_E2E_BB_APP;
    if (!app || !existsSync(app)) usage("--owned-server needs HOME_E2E_BB_APP naming the bb app to launch");
  }
  if (flags.has("owned-server")) usage("--owned-server: UNVERIFIABLE on this host (Task 2.12 part B): the installed bb-app 0.44.0+aleph.0.5.1 failed to start on a fresh data dir inside bwrap --unshare-net (server DrizzleError: cannot rollback - no transaction is active, during initDb migrate); the launcher is not implemented and there is no looser mode");
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
  const { Real, readRealEnv } = await import("./real.js");
  const build = JSON.parse(readFileSync(flags.get("build")!, "utf8"));
  if (build.commit !== commit) usage(`build file commit ${build.commit} is not HEAD ${commit}`);
  const real = new Real(readRealEnv(process.env), runId!, build);
  const recorded: string[] = [];
  let loaded: unknown;
  try {
    loaded = await real.setup();
  } catch (e) {
    process.stderr.write(`ABORT ${e instanceof Error ? e.message : String(e)}\n`);
    await real.cleanup([]);
    process.exit(1);
  }
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
    appendFileSync(out!, `${JSON.stringify({ scenario: name, mode, run_id: runId, commit, tree, dirty, loaded, ...line })}\n`);
    process.stderr.write(`${line.pass ? "PASS" : "FAIL"} ${name} (${((Date.now() - started) / 1000).toFixed(1)} s)${line.pass ? "" : `: ${line.error}`}\n`);
  }
  const cleanup = await real.cleanup(recorded);
  writeFileSync(`${out}.cleanup.json`, JSON.stringify({ run_id: runId, threads: recorded, ...cleanup }, null, 2) + "\n");
  process.stderr.write(`cleanup: archived ${cleanup.archived.length}, failed ${cleanup.failed.length}\n`);
  process.exit(failed || cleanup.failed.length ? 1 : 0);
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
