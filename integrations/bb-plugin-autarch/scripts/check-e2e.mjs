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
  // card lifecycle
  "card-ingest": { card: id, decision: id, generation: eq(1), blocks: num },
  "card-edited-before-pick": { g1: id, g2: id, pick_g1_status: eq(409), owed_g2: eq(true) },
  "card-blocks-only-edit": { card: id, generations: eq(2), blocks_g2: num, card_fp_changed: eq(true) },
  "card-edited-after-pick": { card: id, generations: eq(1), changed_after_ruling: eq(true) },
  "card-invalidated-old-tab-pick": { decision: id, stale_reason: eq("superseded"), picks: eq(0) },
  "card-unlabelled": { card: id, state: eq("closed"), label_rechecked: eq(true) },
  "label-recreated": { card: id, state: eq("open"), withdrawn: eq(false), generations: eq(1) },
  // pick and ruling write
  "answer-instruction": { decision: id, pick_id: id, ruling_sha256: hex, wake_state: eq("done"), wake_count: eq(1), feed_line: feedLine, card_id: id, comment_by: id },
  "pick-retry": { decision: id, pick_id: id, picks: eq(1), wakes: eq(1) },
  "crash-after-pick": { decision: id, pick_id: id, ruling_sha256: hex, pending_wakes: eq(1), pending_card_writes: eq(2), seconds: num },
  "two-writers": { decision: id, picks: eq(1), conflicts: eq(1) },
  "edit-after-pick-failed-write-restart": { card: id, decision: id, original_text_written: eq(true), ruling_sha256: hex },
  "ruling-file-blocked": { blocked_decision: id, blocked_state: eq("pending"), blocked_error: str, other_decision: id, other_ruling_sha256: hex },
  "blocks-notices": { decision: id, refs: num, counted: num },
  // filing (Task 2.9 owns the filer; these are checked once it exists)
  "card-file-retry": { card: id, exit_code: eq(0), cards: eq(1) },
  "card-file-unknown": { card: id, first_exit: eq(4), rerun_exit: eq(0), cards: eq(1), agent_comments: eq(1) },
  "not-ready-at-start": { ask_exit_locked: eq(3), cards_while_locked: eq(0), not_ready: eq(true), decision: id, pick_id: id, seconds: num },
  "filer-registry-authority": { card: id, cards: eq(1), registry_row: eq(true) },
  // retries and archive
  "comment-arrives-later": { card: id, routed_thread: id, updated_at_unchanged: eq(true) },
  "serve-recovers": { card: id, generations: eq(1) },
  "queued-then-archived": { decision: id, wake_state: eq("undeliverable"), listed: eq(true) },
  // override and delegation
  "delegated-override": {
    old_decision: id, new_decision: id, new_generation: eq(2), asker_wakes: eq(1), void_notices: eq(1),
    old_label_in_feed: eq(false), wake_supersedes: eq(true), feed_supersedes: eq(true),
  },
  "override-unlabel-crash": { card: id, state: eq("open"), relabelled: eq(true), generations: eq(2) },
  "project-mismatch-delegation": { card: id, state: eq("display"), generations: eq(0), delegated_rule_refused: eq(true) },
  "duplicate-request-reversed": { forward: (v) => v && id(v.canonical) && id(v.duplicate), reverse: (v) => v && id(v.canonical) && id(v.duplicate), canonical_in_both_orders: eq(true) },
  // root run
  "root-run-display": { card: id, state: eq("match"), command_shown: eq(true), command_hidden_when_unreadable: eq(true), runner_status: str },
  "root-run-injection": { hostile_sets: num, commands_unsafe: eq(0), runner_urls_with_hostile_input: eq(0) },
  // real-bb (Task 2.12 drives these)
  "filer-from-thread": { threads: idList, card: id, comment_thread_matches: eq(true), env_absent_exit: eq(2), other_thread_replay_exit: eq(2) },
  "poller-refresh": { threads: idList, card: id, panel_updated_without_reload: eq(true) },
  "comment-arrives-later-real": { threads: idList, card: id, routed_thread: id },
  "cross-project-panel": { threads: idList, cards: num, projects: num },
  "upgrade-quiesce": { quiesce_refused: eq(true), db_sha256_unchanged: eq(true), backup_files: eq(0), legacy_ask_picked: eq(true), migrated_after_enable: eq(true) },
  "vizier-chat": { threads: idList, message_id: id, screenshot_sha256: hex },
};

/** Names removed from SCHEMAS in Task 2.11, each mapped or kept in the plan's table (r3-8). */
export const RETIRED = { "file-retry": "card-file-retry", "file-unknown": "card-file-unknown", supersede: "card-edited-before-pick", "ask-cli-proxy": "filer-from-thread" };

/** The harness-owned rig block (Task 2.12 step 3-5); the jq in scripts/rig-acceptance.jq is the full gate. */
const RIG = {
  launcher_pid: num, runtime_file_pid: num, socket_owner_pid: num, listen_inode: num,
  listen_inode_in_owned_set: eq(true), owned_set: (v) => Array.isArray(v) && v.every(num),
  nonce_in_tasks_db: eq(true), stopped: eq(true), netns_isolated: eq(true),
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
    else {
      for (const [k, ok] of Object.entries(schema)) if (!ok(o.evidence[k])) fail(`${label}: evidence.${k} is invalid (${JSON.stringify(o.evidence[k])})`);
      if (opts.requireOwnership) {
        const rig = o.evidence.rig;
        if (rig === null || typeof rig !== "object" || Array.isArray(rig)) fail(`${label}: evidence.rig is missing (ownership is required)`);
        else {
          for (const [k, ok] of Object.entries(RIG)) if (!ok(rig[k])) fail(`${label}: evidence.rig.${k} is invalid (${JSON.stringify(rig[k])})`);
          if (rig.launcher_pid !== rig.runtime_file_pid) fail(`${label}: evidence.rig.launcher_pid is not runtime_file_pid`);
          if (Array.isArray(rig.owned_set) && !rig.owned_set.includes(rig.socket_owner_pid)) fail(`${label}: evidence.rig.socket_owner_pid is not in owned_set`);
        }
      }
    }
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
  const a = { file: null, mode: null, runId: null, commit: null, scenarios: null, build: null, repo: process.cwd(), requireOwnership: false };
  for (let i = 0; i < argv.length; i++) {
    const k = argv[i];
    const v = () => argv[++i];
    if (k === "--mode") a.mode = v();
    else if (k === "--run-id") a.runId = v();
    else if (k === "--product-commit") a.commit = v();
    else if (k === "--scenarios") a.scenarios = (v() ?? "").split(",").filter(Boolean);
    else if (k === "--build") a.build = v();
    else if (k === "--repo") a.repo = v();
    else if (k === "--require-ownership") a.requireOwnership = true;
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
