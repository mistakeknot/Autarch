import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { tmpDir } from "./helpers.js";

const CHECK = join(import.meta.dirname, "..", "scripts", "check-e2e.mjs");
const FIX = join(import.meta.dirname, "fixtures", "e2e");
const FAKE = "answer-instruction,blocks-notices,card-blocks-only-edit,card-edited-after-pick,card-edited-before-pick,card-ingest,card-invalidated-old-tab-pick,card-unlabelled,comment-arrives-later,crash-after-pick,delegated-override,duplicate-request-reversed,edit-after-pick-failed-write-restart,label-recreated,override-unlabel-crash,pick-retry,project-mismatch-delegation,queued-then-archived,root-run-display,root-run-injection,ruling-file-blocked,serve-recovers,two-writers";
const REAL = "answer-instruction,comment-arrives-later-real,cross-project-panel,filer-from-thread,poller-refresh,queued-then-archived,upgrade-quiesce,vizier-chat";
const PLUGIN = "c".repeat(64);
const SERVE = "d".repeat(64);
const RUN = "run-1234";

const t = tmpDir();
let repo = "";
let commit = "";
let tree = "";
let oldCommit = "";
let oldTree = "";
const git = (...a: string[]) => execFileSync("git", ["-C", repo, ...a], { encoding: "utf8" }).trim();

beforeAll(() => {
  repo = join(t.dir, "repo");
  mkdirSync(repo);
  git("init", "-q");
  git("config", "user.email", "t@t");
  git("config", "user.name", "t");
  writeFileSync(join(repo, "a"), "1");
  git("add", "a");
  git("commit", "-qm", "one");
  oldCommit = git("rev-parse", "HEAD");
  oldTree = git("rev-parse", "HEAD^{tree}");
  writeFileSync(join(repo, "a"), "2");
  git("commit", "-qam", "two");
  commit = git("rev-parse", "HEAD");
  tree = git("rev-parse", "HEAD^{tree}");
});
afterAll(() => t.cleanup());

function materialise(dir: string, name: string): string {
  const text = readFileSync(join(FIX, dir, `${name}.jsonl`), "utf8")
    .replaceAll("__COMMIT__", commit)
    .replaceAll("__TREE__", tree)
    .replaceAll("__RUN_ID__", RUN)
    .replaceAll("__STALE_RUN__", "run-stale")
    .replaceAll("__PLUGIN__", PLUGIN)
    .replaceAll("__SERVE__", SERVE);
  const out = join(t.dir, `${dir}-${name}.jsonl`);
  writeFileSync(out, text);
  return out;
}

function run(file: string, mode: "fake" | "real-bb", over: { commit?: string; build?: string; noBuild?: boolean; ownership?: boolean } = {}) {
  const build = mode === "real-bb" ? join(t.dir, "build.json") : undefined;
  if (build) writeFileSync(build, JSON.stringify({ commit, plugin_dir: "/x", source_sha256: PLUGIN, autarch_path: "/y", autarch_sha256: SERVE }));
  const args = [CHECK, file, "--mode", mode, "--run-id", RUN, "--product-commit", over.commit ?? commit, "--scenarios", mode === "fake" ? FAKE : REAL, "--repo", repo];
  if (build && !over.noBuild) args.push("--build", over.build ?? build);
  if (over.ownership) args.push("--require-ownership");
  return spawnSync("node", args, { encoding: "utf8" });
}

describe("check-e2e", () => {
  it("accepts a complete fake run", () => {
    const r = run(materialise("accept", "fake"), "fake");
    expect(r.stderr).toBe("");
    expect(r.status).toBe(0);
  });

  it("accepts a complete real-bb run that matches the build file", () => {
    const r = run(materialise("accept", "real-bb"), "real-bb");
    expect(r.stderr).toBe("");
    expect(r.status).toBe(0);
  });

  const rejects = readdirSync(join(FIX, "reject")).map((f) => f.replace(/\.jsonl$/, ""));
  it("has a rejected fixture for each required failure", () => {
    for (const n of ["empty", "partial", "duplicated", "stale-commit", "stale-tree", "dirty", "mixed-run-id", "wrong-mode", "missing-evidence", "same-commit-other-run", "malformed-types", "wake-pending", "short-hash", "wake-count-2", "old-plugin-source", "old-app-source", "old-serve"])
      expect(rejects).toContain(n);
  });
  for (const n of ["empty", "partial", "duplicated", "stale-commit", "stale-tree", "dirty", "mixed-run-id", "wrong-mode", "missing-evidence", "same-commit-other-run", "malformed-types", "wake-pending", "short-hash", "wake-count-2"]) {
    it(`rejects ${n}`, () => {
      const r = run(materialise("reject", n), "fake");
      expect(r.status).toBe(1);
      expect(r.stderr).toMatch(/check-e2e:/);
    });
  }
  for (const [n, field] of [["old-plugin-source", "plugin_source"], ["old-app-source", "app_source"], ["old-serve", "serve_sha256"]] as const) {
    it(`rejects a real-bb run with ${n}`, () => {
      const r = run(materialise("reject", n), "real-bb");
      expect(r.status).toBe(1);
      expect(r.stderr).toContain(`loaded.${field}`);
    });
  }

  it("rejects a passing run when the product commit is a different commit", () => {
    const r = run(materialise("accept", "fake"), "fake", { commit: oldCommit });
    expect(r.status).toBe(1);
    expect(r.stderr).toMatch(/commit .* is not the product commit/);
    expect(oldTree).not.toBe(tree);
  });

  it("rejects a real-bb run given no --build identity evidence", () => {
    const r = run(materialise("accept", "real-bb"), "real-bb", { noBuild: true });
    expect(r.status).toBe(1);
    expect(r.stderr).toContain("--build");
  });

  it("rejects a real-bb run whose build file records a different commit", () => {
    const b = join(t.dir, "stale-build.json");
    writeFileSync(b, JSON.stringify({ commit: oldCommit, source_sha256: PLUGIN, autarch_sha256: SERVE }));
    const r = run(materialise("accept", "real-bb"), "real-bb", { build: b });
    expect(r.status).toBe(1);
    expect(r.stderr).toContain("build file commit");
  });

  it("rejects a real-bb filer-from-thread run whose absent BB_THREAD_ID did not exit 2", () => {
    const f = materialise("accept", "real-bb");
    writeFileSync(f, readFileSync(f, "utf8").replace('"env_absent_exit":2', '"env_absent_exit":0'));
    const r = run(f, "real-bb");
    expect(r.status).toBe(1);
    expect(r.stderr).toContain("env_absent_exit");
  });

  it("rejects an upgrade-quiesce record that does not show bb keeping the previous v2 instance", () => {
    const f = materialise("accept", "real-bb");
    writeFileSync(f, readFileSync(f, "utf8").replace(/,"previous_instance_kept":true/, "").replace(/"v2_instance_hosted_by":"[^"]*"/, '"v2_instance_hosted_by":"standalone-store-process"'));
    const r = run(f, "real-bb");
    expect(r.status).toBe(1);
    expect(r.stderr).toContain("previous_instance_kept");
  });

  it("rejects an old scenario name that Task 2.11 retired", () => {
    const f = materialise("accept", "fake");
    writeFileSync(f, readFileSync(f, "utf8").replace('"scenario":"card-ingest"', '"scenario":"supersede"'));
    const r = run(f, "fake");
    expect(r.status).toBe(1);
    expect(r.stderr).toContain("unknown scenario");
  });

  it("maps every retired scenario name in the plan's Task 2.11 table", async () => {
    const { RETIRED } = await import(CHECK);
    const plan = readFileSync(join(import.meta.dirname, "..", "..", "..", "docs", "plans", "2026-09-30-home-on-bb-tasks-plan.md"), "utf8");
    const row = (n: string) => plan.split("\n").find((l) => l.includes("`" + n + "`") && l.includes("|"));
    for (const [old, now] of Object.entries(RETIRED)) {
      const l = row(old);
      expect(l, `plan row for ${old}`).toBeTruthy();
      expect(l).toContain(now as string);
    }
  });

  describe("--require-ownership", () => {
    const rig = { launcher_pid: 10, runtime_file_pid: 10, socket_owner_pid: 11, listen_inode: 555, listen_inode_in_owned_set: true, owned_set: [10, 11], nonce_in_tasks_db: true, stopped: true, netns_isolated: true };
    function withRig(mutate?: (r: Record<string, unknown>) => void): string {
      const f = materialise("accept", "real-bb");
      const rows = readFileSync(f, "utf8").trim().split("\n").map((l) => JSON.parse(l));
      for (const x of rows) {
        x.evidence.rig = structuredClone(rig);
        mutate?.(x.evidence.rig);
      }
      writeFileSync(f, rows.map((x) => JSON.stringify(x)).join("\n") + "\n");
      return f;
    }
    it("fails a record with no evidence.rig", () => {
      const r = run(materialise("accept", "real-bb"), "real-bb", { ownership: true });
      expect(r.status).toBe(1);
      expect(r.stderr).toContain("evidence.rig is missing");
    });
    it("accepts complete ownership evidence", () => {
      const r = run(withRig(), "real-bb", { ownership: true });
      expect(r.stderr).toBe("");
      expect(r.status).toBe(0);
    });
    it("does not require the rig block without the flag", () => {
      expect(run(materialise("accept", "real-bb"), "real-bb").status).toBe(0);
    });
    for (const [n, m] of [
      ["launcher pid differs from the runtime file pid", (r: Record<string, unknown>) => (r.runtime_file_pid = 99)],
      ["socket owner outside the owned set", (r: Record<string, unknown>) => (r.owned_set = [10])],
      ["listen inode not proven in the owned set", (r: Record<string, unknown>) => (r.listen_inode_in_owned_set = false)],
      ["nonce not in the tasks db", (r: Record<string, unknown>) => (r.nonce_in_tasks_db = false)],
      ["server not stopped", (r: Record<string, unknown>) => (r.stopped = false)],
      ["no netns isolation", (r: Record<string, unknown>) => (r.netns_isolated = false)],
    ] as const)
      it(`rejects ${n}`, () => {
        const r = run(withRig(m), "real-bb", { ownership: true });
        expect(r.status).toBe(1);
        expect(r.stderr).toContain("evidence.rig");
      });
  });

  it("fails with usage when arguments are missing", () => {
    const r = spawnSync("node", [CHECK, join(t.dir, "x")], { encoding: "utf8" });
    expect(r.status).toBe(2);
  });
});
