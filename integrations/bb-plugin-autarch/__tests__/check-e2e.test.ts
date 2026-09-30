import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { tmpDir } from "./helpers.js";

const CHECK = join(import.meta.dirname, "..", "scripts", "check-e2e.mjs");
const FIX = join(import.meta.dirname, "fixtures", "e2e");
const FAKE = "answer-instruction,crash-after-pick,delegated-override,file-retry,file-unknown,not-ready-at-start,pick-retry,queued-then-archived,ruling-file-blocked,supersede,two-writers";
const REAL = "answer-instruction,ask-cli-proxy,queued-then-archived,vizier-chat";
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

function run(file: string, mode: "fake" | "real-bb", over: { commit?: string; build?: string } = {}) {
  const build = mode === "real-bb" ? join(t.dir, "build.json") : undefined;
  if (build) writeFileSync(build, JSON.stringify({ commit, plugin_dir: "/x", source_sha256: PLUGIN, autarch_path: "/y", autarch_sha256: SERVE }));
  const args = [CHECK, file, "--mode", mode, "--run-id", RUN, "--product-commit", over.commit ?? commit, "--scenarios", mode === "fake" ? FAKE : REAL, "--repo", repo];
  if (build) args.push("--build", over.build ?? build);
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

  it("rejects a real-bb run whose build file records a different commit", () => {
    const b = join(t.dir, "stale-build.json");
    writeFileSync(b, JSON.stringify({ commit: oldCommit, source_sha256: PLUGIN, autarch_sha256: SERVE }));
    const r = run(materialise("accept", "real-bb"), "real-bb", { build: b });
    expect(r.status).toBe(1);
    expect(r.stderr).toContain("build file commit");
  });

  it("fails with usage when arguments are missing", () => {
    const r = spawnSync("node", [CHECK, join(t.dir, "x")], { encoding: "utf8" });
    expect(r.status).toBe(2);
  });
});
