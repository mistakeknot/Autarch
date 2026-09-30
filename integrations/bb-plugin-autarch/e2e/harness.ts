// The scenario harness (Task 1.10). Fake mode drives the real plugin wiring with a fake bb;
// real-bb mode is checked but its driver arrives with Task 1.11 (it needs an isolated bb server).
//   tsx e2e/harness.ts --mode fake --run-id <uuid> --out <file.jsonl> [--scenarios a,b]
import { execFileSync } from "node:child_process";
import { appendFileSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { scenarios } from "./scenarios/index.js";

const here = dirname(fileURLToPath(import.meta.url));
const flags = new Map<string, string>();
const argv = process.argv.slice(2);
for (let i = 0; i < argv.length; i += 2) flags.set(argv[i]!.replace(/^--/, ""), argv[i + 1] ?? "");
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
if (mode === "real-bb") {
  process.stderr.write("e2e: real-bb mode needs an isolated bb server (Task 1.11); its driver is not part of Task 1.10\n");
  process.exit(2);
}

const git = (...a: string[]) => execFileSync("git", a, { cwd: here, encoding: "utf8" }).trim();
const top = git("rev-parse", "--show-toplevel");
// Evidence must not land inside the tree it describes: it would make the tree dirty.
const rel = relative(top, resolve(out!));
if (!rel.startsWith("..") && !rel.startsWith(sep)) usage("--out must be outside the git worktree");

const commit = git("rev-parse", "HEAD");
const tree = git("rev-parse", "HEAD^{tree}");
const dirty = git("status", "--porcelain").length > 0;

const scratch = mkdtempSync(join(tmpdir(), "autarch-e2e-build-"));
const autarch = join(scratch, "autarch");
execFileSync("go", ["build", "-buildvcs=false", "-o", autarch, "./cmd/autarch"], { cwd: top, stdio: "inherit" });

const wanted = flags.get("scenarios")?.split(",") ?? Object.keys(scenarios).sort();
writeFileSync(out!, "");
let failed = 0;
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
