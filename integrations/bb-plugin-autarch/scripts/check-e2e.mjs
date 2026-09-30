#!/usr/bin/env node
// Accepts a scenario-harness output file only if every rule of Task 1.10 holds.
// usage: check-e2e.mjs <file> --mode <m> --run-id <id> --product-commit <sha>
//        --scenarios a,b,c [--build <file>] [--repo <dir>]
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";

const HEX64 = /^[0-9a-f]{64}$/;
const ID = /^[A-Za-z0-9:_.-]{1,128}$/;
const str = (v) => typeof v === "string" && v.length > 0;
const num = (v) => typeof v === "number" && Number.isFinite(v);
const id = (v) => typeof v === "string" && ID.test(v);
const hex = (v) => typeof v === "string" && HEX64.test(v);
const eq = (x) => (v) => v === x;
const idList = (v) => Array.isArray(v) && v.length > 0 && v.every(id);
const feedLine = (v) => typeof v === "string" && /^ruled \d{4}-\d{2}-\d{2}T\d{2}:\d{2}Z ".+" \([^)]+\)$/.test(v);

/** Typed evidence schema per scenario; every field is required. */
export const SCHEMAS = {
  "answer-instruction": { decision: id, pick_id: id, ruling_sha256: hex, wake_state: eq("done"), wake_count: eq(1), feed_line: feedLine },
  "pick-retry": { decision: id, pick_id: id, picks: eq(1), wakes: eq(1) },
  "crash-after-pick": { decision: id, pick_id: id, ruling_sha256: hex, pending_wakes: eq(1), seconds: num },
  "two-writers": { decision: id, picks: eq(1), conflicts: eq(1) },
  "file-retry": { decision: id, exit_code: eq(0), decisions: eq(1) },
  "file-unknown": { decision: id, first_exit: eq(4), rerun_exit: eq(0), rerun_decision: id, decisions: eq(1) },
  "not-ready-at-start": { ask_exit_locked: eq(3), not_ready: eq(true), decision: id, pick_id: id, seconds: num },
  supersede: { decision_a: id, decision_b: id, pick_a_status: eq(409), second_replacement_status: eq(409), owed: eq(true) },
  "ruling-file-blocked": { blocked_decision: id, blocked_state: eq("pending"), blocked_error: str, other_decision: id, other_ruling_sha256: hex },
  "queued-then-archived": { decision: id, wake_state: eq("undeliverable"), listed: eq(true) },
  "delegated-override": {
    old_decision: id, new_decision: id, asker_wakes: eq(1), void_notices: eq(1),
    old_label_in_feed: eq(false), wake_supersedes: eq(true), feed_supersedes: eq(true),
  },
  "ask-cli-proxy": { threads: idList, decision: id, env_absent_exit: eq(0), env_conflict_exit: eq(2) },
  "vizier-chat": { threads: idList, message_id: id, screenshot_sha256: hex },
};

const LOADED_KEYS = ["plugin_source", "app_source", "serve_sha256", "autarch_sha256"];

export function check(text, opts) {
  const errors = [];
  const fail = (m) => errors.push(m);
  const lines = text.split("\n").filter((l) => l.trim() !== "");
  if (lines.length === 0) return ["file is empty"];
  const wanted = opts.scenarios;
  const seen = new Map();
  let treeOf = null;
  try {
    treeOf = execFileSync("git", ["-C", opts.repo, "rev-parse", `${opts.commit}^{tree}`], { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim();
  } catch {
    fail(`cannot resolve the tree of ${opts.commit}`);
  }
  let build = null;
  if (opts.mode === "real-bb" && !opts.build) fail("real-bb evidence needs --build: loaded plugin, app and daemon identities cannot be checked without it");
  if (opts.build) {
    try {
      build = JSON.parse(readFileSync(opts.build, "utf8"));
    } catch (e) {
      fail(`cannot read the build file: ${e.message}`);
    }
    if (build && build.commit !== opts.commit) fail(`build file commit ${build.commit} is not the product commit`);
  }
  lines.forEach((line, i) => {
    const at = `line ${i + 1}`;
    let o;
    try {
      o = JSON.parse(line);
    } catch {
      return fail(`${at}: not JSON`);
    }
    if (o === null || typeof o !== "object") return fail(`${at}: not an object`);
    const name = o.scenario;
    if (typeof name !== "string") return fail(`${at}: no scenario`);
    seen.set(name, (seen.get(name) ?? 0) + 1);
    const label = `${at} (${name})`;
    if (o.pass !== true) fail(`${label}: pass is not true`);
    if (o.run_id !== opts.runId) fail(`${label}: run_id ${o.run_id} is not ${opts.runId}`);
    if (o.commit !== opts.commit) fail(`${label}: commit ${o.commit} is not the product commit`);
    if (treeOf && o.tree !== treeOf) fail(`${label}: tree ${o.tree} is not ${treeOf}`);
    if (o.dirty !== false) fail(`${label}: dirty is not false`);
    if (o.mode !== opts.mode) fail(`${label}: mode ${o.mode} is not ${opts.mode}`);
    const schema = SCHEMAS[name];
    if (!schema) fail(`${label}: unknown scenario`);
    else if (o.evidence === null || typeof o.evidence !== "object" || Array.isArray(o.evidence)) fail(`${label}: evidence missing`);
    else for (const [k, ok] of Object.entries(schema)) if (!ok(o.evidence[k])) fail(`${label}: evidence.${k} is invalid (${JSON.stringify(o.evidence[k])})`);
    if (build) {
      const l = o.loaded;
      if (l === null || typeof l !== "object") fail(`${label}: loaded missing`);
      else for (const k of LOADED_KEYS) {
        const want = k === "plugin_source" ? build.source_sha256 : k === "app_source" ? build.source_sha256 : k === "serve_sha256" ? build.autarch_sha256 : build.autarch_sha256;
        if (l[k] !== want) fail(`${label}: loaded.${k} ${l[k]} does not equal the build file (${want})`);
      }
    }
  });
  for (const w of wanted) if (!seen.has(w)) fail(`missing scenario ${w}`);
  for (const [n, c] of seen) {
    if (!wanted.includes(n)) fail(`unexpected scenario ${n}`);
    if (c > 1) fail(`scenario ${n} appears ${c} times`);
  }
  return errors;
}

function parseArgs(argv) {
  const a = { file: null, mode: null, runId: null, commit: null, scenarios: null, build: null, repo: process.cwd() };
  for (let i = 0; i < argv.length; i++) {
    const k = argv[i];
    const v = () => argv[++i];
    if (k === "--mode") a.mode = v();
    else if (k === "--run-id") a.runId = v();
    else if (k === "--product-commit") a.commit = v();
    else if (k === "--scenarios") a.scenarios = (v() ?? "").split(",").filter(Boolean);
    else if (k === "--build") a.build = v();
    else if (k === "--repo") a.repo = v();
    else if (!a.file) a.file = k;
    else throw new Error(`unexpected argument ${k}`);
  }
  for (const [k, v] of Object.entries({ file: a.file, "--mode": a.mode, "--run-id": a.runId, "--product-commit": a.commit, "--scenarios": a.scenarios }))
    if (!v) throw new Error(`missing ${k}`);
  return a;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  let a;
  try {
    a = parseArgs(process.argv.slice(2));
  } catch (e) {
    console.error(`check-e2e: ${e.message}`);
    process.exit(2);
  }
  let text;
  try {
    text = readFileSync(a.file, "utf8");
  } catch (e) {
    console.error(`check-e2e: ${e.message}`);
    process.exit(1);
  }
  const errors = check(text, a);
  if (errors.length) {
    for (const e of errors) console.error(`check-e2e: ${e}`);
    process.exit(1);
  }
  console.log(`check-e2e: ok (${a.scenarios.length} scenarios, run ${a.runId})`);
}
