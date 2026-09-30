// The fake-mode scenarios. Each returns typed evidence (see scripts/check-e2e.mjs) or throws.
import assert from "node:assert/strict";
import Database from "better-sqlite3";
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { FeedCaches } from "../../feed.js";
import { Rig, runAsync, sleep, spawnChild, waitFor } from "../rig.js";

export interface Ctx {
  /** Path of the autarch binary under test (built from the product commit). */
  autarch: string;
}
export type Evidence = Record<string, unknown>;
export type Scenario = (ctx: Ctx) => Promise<Evidence>;

const sha = (text: string | Buffer) => createHash("sha256").update(text).digest("hex");
const idOf = (stdout: string) => (JSON.parse(stdout) as { id: string }).id;

type Owed = { id: string; revision: string };
async function owed(rig: Rig, id: string): Promise<Owed> {
  const r = (await rig.home.handlers.listAsks(null)).owed.find((d: Owed) => d.id === id);
  assert.ok(r, `decision ${id} is not owed`);
  return r;
}
async function fileAsk(rig: Rig, thread: string, over: Record<string, unknown> = {}) {
  const r = await rig.ask(thread, rig.askBody({ thread, ...over }));
  assert.equal(r.exitCode, 0, `ask failed: ${r.stderr}`);
  return idOf(r.stdout);
}
async function pickAs(rig: Rig, id: string, option: string, pickId: string, revision?: string) {
  const rev = revision ?? (await owed(rig, id)).revision;
  return rig.home.handlers.pick({ decision_id: id, option_id: option, revision: rev, pick_id: pickId });
}
/** The feed text bb would inject for (project, thread), from a read-only view. */
function feedText(rig: Rig, project: string, thread: string): string {
  const db = new Database(rig.file, { readonly: true });
  try {
    return new FeedCaches(() => db, () => Date.now()).configure(project, thread) ?? "";
  } finally {
    db.close();
  }
}
const count = (rig: Rig, sql: string, ...a: unknown[]) => (rig.read<{ n: number }>(sql, ...a)[0]?.n ?? 0);
const rulingFile = (rig: Rig, project: string) => {
  const name = rig.ruling(project);
  assert.ok(name, `no ruling file in ${project}`);
  return readFileSync(join(rig.roots[project]!, "docs", "decisions", name));
};
const withRig = async <T>(f: (rig: Rig) => Promise<T>, names?: string[]): Promise<T> => {
  const rig = new Rig(names);
  try {
    return await f(rig);
  } finally {
    rig.cleanup();
  }
};

export const scenarios: Record<string, Scenario> = {
  "answer-instruction": () =>
    withRig(async (rig) => {
      rig.wire();
      rig.startWakes();
      const decision = await fileAsk(rig, "thr-a");
      const pick_id = `pick-${decision}`;
      assert.equal((await pickAs(rig, decision, "project", pick_id)).status, 201);
      await waitFor("the wake to finish", () => rig.read("SELECT 1 FROM obligations WHERE decision_id = ? AND kind = 'wake' AND state = 'done'", decision).length > 0);
      assert.equal(rig.sdk.sent.length, 1);
      assert.equal(rig.sdk.sent[0]!.threadId, "thr-a");
      assert.match(rig.sdk.sent[0]!.input, /group per project/);
      const feed = feedText(rig, "Autarch", "thr-a");
      const line = feed.split("\n").map((l) => l.trim()).find((l) => l.startsWith("ruled ") && l.includes(`(${decision})`));
      assert.ok(line, `no feed line for ${decision} in:\n${feed}`);
      assert.ok(line.includes("Collapse per project"));
      assert.ok(!feed.includes("group per project"), "the instruction text leaked into the feed");
      return {
        decision,
        pick_id,
        ruling_sha256: sha(rulingFile(rig, "Autarch")),
        wake_state: "done",
        wake_count: count(rig, "SELECT COUNT(*) n FROM obligations WHERE decision_id = ? AND kind = 'wake'", decision),
        feed_line: line,
      };
    }),

  "pick-retry": () =>
    withRig(async (rig) => {
      rig.wire();
      rig.startWakes();
      const decision = await fileAsk(rig, "thr-a");
      const pick_id = `pick-${decision}`;
      const { revision } = await owed(rig, decision);
      assert.equal((await pickAs(rig, decision, "project", pick_id, revision)).status, 201);
      const again = await pickAs(rig, decision, "project", pick_id, revision);
      assert.equal(again.status, 200, "a retried pick is a replay, not a second pick");
      await waitFor("the wake", () => rig.sdk.sent.length >= 1);
      await sleep(300);
      assert.equal(rig.sdk.sent.length, 1);
      return {
        decision,
        pick_id,
        picks: count(rig, "SELECT COUNT(*) n FROM picks WHERE decision_id = ?", decision),
        wakes: count(rig, "SELECT COUNT(*) n FROM obligations WHERE decision_id = ? AND kind = 'wake'", decision),
      };
    }),

  "crash-after-pick": () =>
    withRig(async (rig) => {
      rig.wire();
      const decision = await fileAsk(rig, "thr-a");
      const { revision } = await owed(rig, decision);
      const pick_id = `pick-${decision}`;
      rig.stop();
      const child = spawnChild(["pick", rig.file, rig.projectsFile, decision, "project", revision, pick_id, "--kill-after-commit"]);
      const ex = await child.exit;
      assert.equal(ex.signal, "SIGKILL", "the child was meant to die after the pick committed");
      assert.equal(count(rig, "SELECT COUNT(*) n FROM picks WHERE decision_id = ?", decision), 1);
      assert.equal(rig.ruling("Autarch"), undefined, "the ruling file must not exist yet");
      // Restart the plugin without the wake loop: only the ruling-file reconcile timer can finish it.
      const t0 = Date.now();
      rig.wire();
      await waitFor("the ruling file after restart", () => rig.ruling("Autarch"), 40_000);
      const seconds = Math.round((Date.now() - t0) / 100) / 10;
      return {
        decision,
        pick_id,
        ruling_sha256: sha(rulingFile(rig, "Autarch")),
        pending_wakes: count(rig, "SELECT COUNT(*) n FROM obligations WHERE decision_id = ? AND kind = 'wake' AND state = 'pending'", decision),
        seconds,
      };
    }),

  "two-writers": () =>
    withRig(async (rig) => {
      rig.wire();
      const decision = await fileAsk(rig, "thr-a");
      const { revision } = await owed(rig, decision);
      rig.stop();
      const a = spawnChild(["pick", rig.file, rig.projectsFile, decision, "project", revision, "pick-a"]);
      const b = spawnChild(["pick", rig.file, rig.projectsFile, decision, "day", revision, "pick-b"]);
      const results = (await Promise.all([a.out, b.out])).map((o) => JSON.parse(o.trim()) as { status: number });
      const statuses = results.map((r) => r.status).sort();
      assert.deepEqual(statuses, [201, 409]);
      return {
        decision,
        picks: count(rig, "SELECT COUNT(*) n FROM picks WHERE decision_id = ?", decision),
        conflicts: results.filter((r) => r.status === 409).length,
      };
    }),

  "file-retry": ({ autarch }) =>
    withRig(async (rig) => {
      rig.wire();
      await rig.serveBb();
      rig.fault = "drop-response-after-commit";
      const r = await decideFile(rig, autarch, "thr-a");
      assert.equal(r.code, 0, `decide file: ${r.stderr}`);
      const decision = r.stdout.trim();
      return { decision, exit_code: 0, decisions: count(rig, "SELECT COUNT(*) n FROM decisions") };
    }),

  "file-unknown": ({ autarch }) =>
    withRig(async (rig) => {
      rig.wire();
      await rig.serveBb();
      rig.fault = "hang-response";
      const first = await decideFile(rig, autarch, "thr-a");
      assert.equal(first.code, 4, `first decide file: ${first.stderr}`);
      const again = await decideFile(rig, autarch, "thr-a");
      assert.equal(again.code, 0, `re-run: ${again.stderr}`);
      const rows = rig.read<{ id: string }>("SELECT id FROM decisions");
      assert.equal(rows.length, 1);
      return { decision: rows[0]!.id, first_exit: first.code, rerun_exit: again.code, rerun_decision: again.stdout.trim(), decisions: rows.length };
    }),

  "not-ready-at-start": () =>
    withRig(async (rig) => {
      const lock = spawnChild(["lock", rig.file]);
      await waitFor("the lock child", () => lock.stdout().includes("locked"));
      rig.wire();
      await sleep(700);
      assert.equal(rig.handle.ready(), false);
      const locked = await rig.ask("thr-a", rig.askBody());
      await assert.rejects(() => rig.home.handlers.listAsks(null), /not ready/);
      lock.proc.stdin!.end();
      await lock.exit;
      const t0 = Date.now();
      await waitFor("the store to become ready", () => rig.handle.ready(), 35_000);
      const seconds = Math.round((Date.now() - t0) / 100) / 10;
      const decision = await fileAsk(rig, "thr-a");
      const pick_id = `pick-${decision}`;
      assert.equal((await pickAs(rig, decision, "day", pick_id)).status, 201);
      return { ask_exit_locked: locked.exitCode, not_ready: true, decision, pick_id, seconds };
    }),

  supersede: () =>
    withRig(async (rig) => {
      rig.wire();
      const decision_a = await fileAsk(rig, "thr-a");
      const decision_b = await fileAsk(rig, "thr-a", { supersedes: decision_a, question: "Collapse per project or per day, revised?" });
      const pick = await rig.home.handlers.pick({ decision_id: decision_a, option_id: "day", revision: "any", pick_id: "pick-a" });
      const second = await rig.ask("thr-a", rig.askBody({ supersedes: decision_a, question: "A third phrasing?" }));
      const isOwed = (await rig.home.handlers.listAsks(null)).owed.some((d: Owed) => d.id === decision_b);
      return { decision_a, decision_b, pick_a_status: pick.status, second_replacement_status: second.exitCode === 5 ? 409 : second.exitCode, owed: isOwed };
    }),

  "ruling-file-blocked": () =>
    withRig(async (rig) => {
      rig.wire();
      const dir = join(rig.roots.P!, "docs", "decisions");
      rmSync(dir, { recursive: true });
      writeFileSync(dir, "not a directory"); // mkdir and open both fail, even for root
      const blocked = await fileAsk(rig, "thr-p", { project: "P", project_root: rig.roots.P });
      const other = await fileAsk(rig, "thr-q", { project: "Q", project_root: rig.roots.Q });
      assert.equal((await pickAs(rig, blocked, "day", "pick-blocked")).status, 201);
      assert.equal((await pickAs(rig, other, "day", "pick-other")).status, 201);
      const ob = rig.read<{ state: string; last_error: string }>("SELECT state, last_error FROM obligations WHERE decision_id = ? AND kind = 'ruling-file'", blocked)[0]!;
      assert.equal(ob.state, "pending");
      assert.ok(ob.last_error);
      const ev = { blocked_decision: blocked, blocked_state: ob.state, blocked_error: ob.last_error, other_decision: other, other_ruling_sha256: sha(rulingFile(rig, "Q")) };
      rmSync(dir);
      mkdirSync(dir);
      return ev;
    }),

  "queued-then-archived": () =>
    withRig(async (rig) => {
      rig.sdk.script = [{ kind: "queue" }];
      rig.wire();
      rig.startWakes();
      const decision = await fileAsk(rig, "thr-a");
      assert.equal((await pickAs(rig, decision, "project", `pick-${decision}`)).status, 201);
      await waitFor("the wake to queue", () => rig.read("SELECT 1 FROM obligations WHERE decision_id = ? AND kind = 'wake' AND state = 'queued'", decision).length > 0);
      rig.emit("thread.archived", { thread: { id: "thr-a" } });
      await waitFor("the wake to become undeliverable", () => rig.read("SELECT 1 FROM obligations WHERE decision_id = ? AND kind = 'wake' AND state = 'undeliverable'", decision).length > 0);
      const list = await rig.home.handlers.listAsks(null);
      const listed = list.undeliverable.some((o: { decision_id: string }) => o.decision_id === decision);
      return { decision, wake_state: "undeliverable", listed };
    }),

  "delegated-override": () =>
    withRig(async (rig) => {
      rig.wire();
      rig.startWakes();
      const s = await rig.home.handlers.setDelegation({ vizierThreadId: "thr-vizier", projects: ["Autarch"], dailyCap: 5 });
      assert.ok(s.ok);
      await rig.home.handlers.markSeen({ item: (s as { item: string }).item });
      const old_decision = await fileAsk(rig, "thr-a");
      const ruled = await rig.run(["rule", old_decision, "project", "--reason", "routine collapse"], "thr-vizier");
      assert.equal(ruled.exitCode, 0, ruled.stderr);
      await waitFor("the vizier wake", () => rig.sdk.sent.length >= 1);
      const asker_wakes = count(rig, "SELECT COUNT(*) n FROM obligations WHERE decision_id = ? AND kind = 'wake'", old_decision);
      const ov = await rig.home.handlers.override({ decision_id: old_decision });
      assert.ok(ov.ok, JSON.stringify(ov));
      const new_decision = (ov as { decision_id: string }).decision_id;
      await waitFor("the void notice", () => rig.sdk.sent.length >= 2);
      const void_notices = count(rig, "SELECT COUNT(*) n FROM obligations WHERE decision_id = ? AND kind = 'void-notice'", old_decision);
      const old_label_in_feed = feedText(rig, "Autarch", "thr-other").includes('"Collapse per project"');
      assert.equal((await pickAs(rig, new_decision, "project", `pick-${new_decision}`)).status, 201);
      await waitFor("the wake on the replacement", () => rig.sdk.sent.length >= 3);
      const wake = rig.sdk.sent[rig.sdk.sent.length - 1]!.input;
      const feed = feedText(rig, "Autarch", "thr-a");
      return {
        old_decision,
        new_decision,
        asker_wakes,
        void_notices,
        old_label_in_feed,
        wake_supersedes: wake.includes(`supersedes the vizier's ruling on ${old_decision}`),
        feed_supersedes: feed.includes(`supersedes ${old_decision}`),
      };
    }),
};

/** `autarch decide file` in the Autarch scratch project, with the fake bb on PATH. */
function decideFile(rig: Rig, autarch: string, thread: string) {
  const req = { v: 1, kind: "decide", question: "Collapse routine catch-up items per project or per day?", subject: "autarch/catch-up: collapse order", options: rig.askBody().options };
  return runAsync(autarch, ["decide", "file", "--thread", thread], {
    cwd: rig.roots.Autarch,
    env: { ...process.env, PATH: `${rig.bbBin}:${process.env.PATH}`, E2E_BB_URL: rig.bbUrl, BB_THREAD_ID: "" },
    input: JSON.stringify(req),
  });
}
