// The fake-mode scenarios (Task 2.11): the real plugin code (Service, Queue, CardWriter, Delegation,
// WakeLoop, FeedCaches, rootRun) over a real SQLite file and real project directories, with only tasks
// (FakeTasks), the thread SDK and the runner's HTTP answer faked. Each returns typed evidence (see
// scripts/check-e2e.mjs) or throws. Nothing here spawns a process or opens a socket except through
// rigexec.ts; the two multi-process scenarios use spawnChild, which goes through rigSpawn.
import assert from "node:assert/strict";
import Database from "better-sqlite3";
import { createHash } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { FeedCaches } from "../../feed.js";
import { rootRun } from "../../rootrun.js";
import { WakeLoop } from "../../wakes.js";
import { cleanupEnvs, opened, pollN, rig, type Rig } from "../../__tests__/card-rig.js";
import { archived, FakeSdk } from "../../__tests__/wakes-helpers.js";
import { Asks } from "../../asks.js";
import { Catchup } from "../../catchup.js";
import { homeCli } from "../../cli.js";
import { FakeBbServer } from "../../__tests__/fake-bb-server.js";
import { makeTarget, rigExec, type RigTarget } from "../rigexec.js";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { randomUUID } from "node:crypto";
import { spawnChild } from "../rig.js";

export interface Ctx {
  /** Path of the autarch binary under test (built from the product commit). */
  autarch: string;
}
export type Evidence = Record<string, unknown>;
export type Scenario = (ctx: Ctx) => Promise<Evidence>;

const sha = (text: string | Buffer) => createHash("sha256").update(text).digest("hex");
const rulings = (root: string) => {
  const dir = join(root, "docs", "decisions");
  return existsSync(dir) ? readdirSync(dir).filter((f) => f.endsWith(".md")) : [];
};
const rulingSha = (root: string) => {
  const f = rulings(root);
  assert.equal(f.length, 1, `expected one ruling file in ${root}, got ${f.length}`);
  return sha(readFileSync(join(root, "docs", "decisions", f[0]!)));
};
const one = <T = any>(r: Rig, sql: string, ...a: unknown[]) => r.db.prepare(sql).get(...a) as T;
const count = (r: Rig, sql: string, ...a: unknown[]) => (one<{ n: number }>(r, sql, ...a)?.n ?? 0);
const labelled = (r: Rig, id: string) => r.fake.tasks.find((x) => x.id === id)!.labelIds.length > 0;
const feedText = (r: Rig, project: string, thread: string): string => {
  const db = new Database(r.env.file, { readonly: true });
  try {
    return new FeedCaches(() => db, () => r.env.clock.t).configure(project, thread) ?? "";
  } finally {
    db.close();
  }
};
const projectsFile = (r: Rig) => {
  const f = join(r.env.dir, "projects.json");
  writeFileSync(f, JSON.stringify(r.env.projects));
  return f;
};
const withRig = async <T>(f: (r: Rig) => Promise<T>, opts?: Parameters<typeof rig>[0]): Promise<T> => {
  try {
    return await f(rig(opts));
  } finally {
    cleanupEnvs();
  }
};
const MAX_AGE = 3_700_000;

/** Pick option a as mk and run the wake loop until the wake is delivered. */
async function pickAndWake(r: Rig, decision: string, sdk = new FakeSdk()) {
  const loop = new WakeLoop(r.svc, sdk);
  const p = r.mkPick(decision);
  assert.ok(p.ok, `pick failed: ${JSON.stringify(p)}`);
  await loop.drain();
  return { sdk, pick: p as { ok: true; pick: { pick_id: string } } };
}

// ---- filing: the built `autarch needs-mk file` against the fake bb (Task 2.12) -----------------------------------
interface Filing {
  r: Rig;
  server: FakeBbServer;
  target: RigTarget;
  /** Run `autarch needs-mk file` in the asking thread; returns its exit code and output. */
  file(key: string, over?: { thread?: string; askFile?: string }): Promise<{ code: number | null; stdout: string; stderr: string; card: string | undefined }>;
  askFile(question: string): string;
}

async function withFiling<T>(autarch: string, f: (x: Filing) => Promise<T>): Promise<T> {
  const r = rig();
  const dir = mkdtempSync(join(tmpdir(), "autarch-e2e-filing-"));
  const server = new FakeBbServer(r.fake, async (argv, threadId) => {
    const cli = homeCli({ svc: r.svc, asks: new Asks(r.svc), catchup: new Catchup(r.svc, r.dele), rule: (id, o, why, ctx) => r.dele.rule(id, o, why, ctx), isVizier: () => false });
    const out = await cli.run(argv, threadId ? { threadId } : {});
    return { exitCode: out.exitCode ?? 0, stdout: out.stdout ?? "", stderr: out.stderr ?? "" };
  });
  await server.start(dir);
  try {
    const target = makeTarget({ url: server.url, dataDir: join(dir, "data"), home: join(dir, "home"), realBb: server.bin });
    const askFile = (question: string) => {
      const file = join(dir, `ask-${randomUUID()}.json`);
      writeFileSync(
        file,
        JSON.stringify({ question, subject: question, project: "Autarch", project_root: r.env.roots.Autarch, options: [{ id: "a", label: "Per project", kind: "ruling-only" }, { id: "b", label: "Per day", kind: "ruling-only" }] }),
      );
      return file;
    };
    const filing: Filing = {
      r,
      server,
      target,
      askFile,
      async file(key, over = {}) {
        const res = await rigExec(autarch, ["needs-mk", "file", "--project", r.tp.prefix, "--title", "Collapse order", "--ask-file", over.askFile ?? askFile("Collapse per project or per day?"), "--request", key, "--blocks", "bead:mk-okek.8"], { target, threadId: over.thread ?? "thr_a", timeoutMs: 90_000 });
        let card: string | undefined;
        try {
          card = (JSON.parse(res.stdout.trim().split("\n").pop() ?? "") as { card?: string }).card;
        } catch {
          /* no card on a failed run */
        }
        return { ...res, card };
      },
    };
    return await f(filing);
  } finally {
    await server.stop();
    cleanupEnvs();
  }
}
const key = (name: string) => `${name}-${randomUUID().slice(0, 8)}`;
const needsMkCards = (r: Rig) => r.fake.tasks.filter((t) => t.labelIds.includes(r.label.id));
const agentComments = (r: Rig, card: string) => r.fake.comments.filter((c) => c.taskId === card && c.kind === "agent");

export const scenarios: Record<string, Scenario> = {
  // ---- card lifecycle ----------------------------------------------------------------
  "card-ingest": () =>
    withRig(async (r) => {
      const { t, g1 } = await opened(r, { key: "k-ingest", blocks: "bead:mk-okek.8 thread:thr_abc project:autarch bead:mk-okek.8" });
      assert.equal(g1.id, `card-${t.id}-g1`);
      const refs = r.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ?").all(g1.id) as { ref: string }[];
      assert.equal(refs.length, 3);
      return { card: t.id, decision: g1.id, generation: g1.generation, blocks: refs.length };
    }),

  "card-edited-before-pick": () =>
    withRig(async (r) => {
      const { t, g1 } = await opened(r, { key: "k-ebp" });
      r.edit(t, { description: r.desc({ key: "k-ebp", question: "Collapse per project, per day or per week?" }) });
      await r.poll();
      const g = r.gens(t.id);
      assert.deepEqual(g.map((x) => x.generation), [1, 2]);
      const old = r.pickOld(g1.id, g1.revision);
      assert.equal(old.ok, false);
      assert.equal((old as { reason?: string }).reason, "superseded");
      assert.ok(r.svc.owed().some((d) => d.id === g[1].id), "g2 must be owed");
      assert.ok(r.pick(g[1].id).ok);
      return { g1: g1.id, g2: g[1].id, pick_g1_status: 409, owed_g2: true };
    }),

  "card-blocks-only-edit": () =>
    withRig(async (r) => {
      const { t, g1 } = await opened(r, { key: "k-bo" });
      r.edit(t, { description: r.desc({ key: "k-bo", blocks: "bead:mk-okek.8 bead:mk-okek.9" }) });
      await r.poll();
      const g = r.gens(t.id);
      assert.equal(g.length, 2);
      assert.notEqual(g[1].card_fp, g1.card_fp);
      const refs = (id: string) => (r.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ? ORDER BY ref").all(id) as { ref: string }[]).map((x) => x.ref);
      assert.notDeepEqual(refs(g[1].id), refs(g1.id));
      return { card: t.id, generations: g.length, blocks_g2: refs(g[1].id).length, card_fp_changed: true };
    }),

  "card-edited-after-pick": () =>
    withRig(async (r) => {
      const { t, g1 } = await opened(r, { key: "k-eap" });
      assert.ok(r.mkPick(g1.id).ok);
      const before = JSON.stringify(r.gens(t.id)[0]);
      r.edit(t, { description: r.desc({ key: "k-eap", blocks: "bead:other" }) });
      await r.poll();
      assert.equal(r.gens(t.id).length, 1, "no generation after a pick");
      assert.equal(JSON.stringify(r.gens(t.id)[0]), before, "g1 row must be byte-identical");
      assert.equal(r.cardRow(t.id).changed_after_ruling, 1);
      return { card: t.id, generations: 1, changed_after_ruling: true };
    }),

  "card-invalidated-old-tab-pick": () =>
    withRig(async (r) => {
      const { t, g1 } = await opened(r, { key: "k-tab" });
      r.edit(t, { description: r.desc({ key: "k-tab", question: "A different question?" }) });
      await r.poll();
      const stale = r.pickOld(g1.id, g1.revision);
      assert.equal(stale.ok, false);
      const wrong = r.pickOld(r.gens(t.id)[1].id, "wrong-revision");
      assert.equal(wrong.ok, false);
      assert.equal(count(r, "SELECT COUNT(*) n FROM picks WHERE decision_id IN (?, ?)", g1.id, r.gens(t.id)[1].id), 0);
      return { decision: g1.id, stale_reason: (stale as { reason?: string }).reason, picks: 0 };
    }),

  "card-unlabelled": () =>
    withRig(async (r) => {
      const { t } = await opened(r, { key: "k-unl" });
      r.edit(t, { labelIds: [] });
      const reads = r.fake.callsOf("listLabels").length;
      await r.poll();
      assert.ok(r.fake.callsOf("listLabels").length > reads, "withdrawal needs a fresh listLabels");
      assert.equal(r.cardRow(t.id).state, "closed");
      return { card: t.id, state: "closed", label_rechecked: true };
    }),

  "label-recreated": () =>
    withRig(async (r) => {
      const { t } = await opened(r, { key: "k-lr" });
      r.fake.labels = r.fake.labels.filter((l) => l.id !== r.label.id);
      const fresh = r.fake.addLabel(r.tp.id, "needs-mk");
      r.edit(t, { labelIds: [fresh.id] });
      await r.poll();
      assert.equal(r.cardRow(t.id).state, "open");
      assert.equal(one(r, "SELECT withdrawn_at w FROM decisions WHERE task_id = ?", t.id).w, null);
      assert.equal(r.gens(t.id).length, 1);
      return { card: t.id, state: "open", withdrawn: false, generations: 1 };
    }),

  // ---- pick and ruling write ----------------------------------------------------------
  "answer-instruction": () =>
    withRig(async (r) => {
      const { t, g1 } = await opened(r, { key: "k-ai", thread: "thr_a" });
      const { sdk, pick } = await pickAndWake(r, g1.id);
      const w = r.svc.store.obligationsFor(g1.id).find((o) => o.kind === "wake")!;
      assert.equal(sdk.sent.length, 1);
      const line = feedText(r, "Autarch", "thr_a").split("\n").find((l) => l.startsWith("ruled "));
      assert.ok(line, "no feed line");
      return {
        decision: g1.id,
        pick_id: pick.pick.pick_id,
        ruling_sha256: rulingSha(r.env.roots.Autarch!),
        wake_state: r.svc.store.obligation(w.id)!.state,
        wake_count: sdk.sent.length,
        feed_line: line,
        card_id: t.id,
        comment_by: r.cardRow(t.id).routed_thread,
      };
    }),

  "pick-retry": () =>
    withRig(async (r) => {
      const { g1 } = await opened(r, { key: "k-pr" });
      const sdk = new FakeSdk();
      const loop = new WakeLoop(r.svc, sdk);
      const pick_id = `pick-${g1.id}`;
      assert.ok(r.mkPick(g1.id, pick_id).ok);
      assert.ok(r.mkPick(g1.id, pick_id).ok, "the same pick_id replays");
      await loop.drain();
      return {
        decision: g1.id,
        pick_id,
        picks: count(r, "SELECT COUNT(*) n FROM picks WHERE decision_id = ?", g1.id),
        wakes: count(r, "SELECT COUNT(*) n FROM obligations WHERE decision_id = ? AND kind = 'wake'", g1.id),
      };
    }),

  "crash-after-pick": () =>
    withRig(async (r) => {
      const { g1 } = await opened(r, { key: "k-cap" });
      const pf = projectsFile(r);
      const rev = (r.svc.store.decision(g1.id) as { revision: string }).revision;
      const pick_id = `pick-${g1.id}`;
      r.svc.store.db.pragma("wal_checkpoint(TRUNCATE)");
      r.svc.store.db.close();
      const ex = await spawnChild(["pick", r.env.file, pf, g1.id, "a", rev, pick_id, "--kill-after-commit"]).exit;
      assert.equal(ex.signal, "SIGKILL", "the child was meant to die after the pick committed");
      r.restart();
      assert.equal(count(r, "SELECT COUNT(*) n FROM picks WHERE decision_id = ?", g1.id), 1);
      assert.deepEqual(rulings(r.env.roots.Autarch!), [], "the ruling file must not exist yet");
      const pendingWrites = count(r, "SELECT COUNT(*) n FROM card_writes WHERE decision_id = ? AND state = 'pending'", g1.id);
      assert.equal(pendingWrites, 2);
      const t0 = Date.now();
      r.env.clock.t = Date.now() + MAX_AGE; // the child stamped its rows with the wall clock
      r.svc.reconcileAll();
      return {
        decision: g1.id,
        pick_id,
        ruling_sha256: rulingSha(r.env.roots.Autarch!),
        pending_wakes: count(r, "SELECT COUNT(*) n FROM obligations WHERE decision_id = ? AND kind = 'wake' AND state = 'pending'", g1.id),
        pending_card_writes: pendingWrites,
        seconds: Math.round((Date.now() - t0) / 100) / 10,
      };
    }),

  "two-writers": () =>
    withRig(async (r) => {
      const { g1 } = await opened(r, { key: "k-tw" });
      const pf = projectsFile(r);
      const rev = (r.svc.store.decision(g1.id) as { revision: string }).revision;
      r.svc.store.db.pragma("wal_checkpoint(TRUNCATE)");
      r.svc.store.db.close();
      const a = spawnChild(["pick", r.env.file, pf, g1.id, "a", rev, "pick-a"]);
      const b = spawnChild(["pick", r.env.file, pf, g1.id, "b", rev, "pick-b"]);
      const results = (await Promise.all([a.out, b.out])).map((o) => JSON.parse(o.trim()) as { status: number });
      assert.deepEqual(results.map((x) => x.status).sort(), [201, 409]);
      r.restart();
      return { decision: g1.id, picks: count(r, "SELECT COUNT(*) n FROM picks WHERE decision_id = ?", g1.id), conflicts: results.filter((x) => x.status === 409).length };
    }),

  "edit-after-pick-failed-write-restart": () =>
    withRig(async (r) => {
      const { t, g1 } = await opened(r, { key: "k-eapf", question: "Original question?" });
      const root = r.env.roots.Autarch!;
      chmodSync(root, 0o555);
      assert.ok(r.mkPick(g1.id).ok);
      assert.deepEqual(rulings(root), []);
      r.edit(t, { description: r.desc({ key: "k-eapf", question: "A rewritten question?" }) });
      await r.poll();
      assert.equal(r.gens(t.id).length, 1);
      r.restart();
      chmodSync(root, 0o755);
      r.advance(MAX_AGE);
      r.svc.reconcileAll();
      const f = rulings(root);
      assert.equal(f.length, 1);
      const text = readFileSync(join(root, "docs", "decisions", f[0]!), "utf8");
      assert.ok(text.includes("Original question?") && !text.includes("A rewritten question?"));
      assert.ok(text.includes(`card_id: "${t.id}"`) && text.includes("generation: 1"));
      return { card: t.id, decision: g1.id, original_text_written: true, ruling_sha256: sha(text) };
    }),

  "ruling-file-blocked": () =>
    withRig(
      async (r) => {
        const a = await opened(r, { key: "k-rfb-a" });
        const ot = r.fake.addProject("Other");
        const l2 = r.fake.addLabel(ot.id, "needs-mk");
        const o2 = r.fake.addTask(ot.id, { labelIds: [l2.id], title: "Other card", description: r.desc({ key: "k-rfb-c", ask: { project: "Other", project_root: r.env.roots.Other } }) });
        r.fake.addComment(o2.id, { threadId: "thr_b" });
        await r.poll();
        const og = r.gens(o2.id)[0];
        assert.ok(og, `the Other card did not open: ${JSON.stringify(r.cardRow(o2.id))}`);
        chmodSync(r.env.roots.Autarch!, 0o555);
        assert.ok(r.mkPick(a.g1.id).ok);
        assert.ok(r.mkPick(og.id).ok);
        const ob = r.svc.store.obligationsFor(a.g1.id).find((o) => o.kind === "ruling-file")!;
        assert.equal(ob.state, "pending");
        return {
          blocked_decision: a.g1.id,
          blocked_state: ob.state,
          blocked_error: String(ob.last_error ?? "ruling file could not be written"),
          other_decision: og.id,
          other_ruling_sha256: rulingSha(r.env.roots.Other!),
        };
      },
      { projects: ["Autarch", "Other"] },
    ),

  "blocks-notices": () =>
    withRig(async (r) => {
      const { g1 } = await opened(r, { key: "k-bn", blocks: "bead:mk-okek.8 thread:thr_abc project:autarch bead:mk-okek.8 weird:thing" });
      const refs = (r.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ? ORDER BY ref").all(g1.id) as { ref: string }[]).map((x) => x.ref);
      const { blocksCount } = await import("../../queueview.js");
      assert.equal(blocksCount(refs), refs.filter((x) => /^(bead|thread|project):/.test(x)).length);
      return { decision: g1.id, refs: refs.length, counted: blocksCount(refs) };
    }),

  // ---- retries and archive ------------------------------------------------------------
  "comment-arrives-later": () =>
    withRig(async (r) => {
      const t = r.card({ thread: null, key: "k-cal" });
      await r.poll();
      assert.equal(r.cardRow(t.id).state, "observed");
      const stamp = t.updatedAt;
      r.fake.addComment(t.id, { threadId: "thr_late" });
      assert.equal(t.updatedAt, stamp);
      r.advance(5_000);
      await r.poll();
      assert.equal(r.cardRow(t.id).state, "open");
      return { card: t.id, routed_thread: r.cardRow(t.id).routed_thread, updated_at_unchanged: true };
    }),

  "serve-recovers": () =>
    withRig(async (r) => {
      r.env.down.value = true;
      const t = r.card({ key: "k-sr" });
      await r.poll();
      assert.match(String(r.cardRow(t.id).display_reason), /^root unverified: /);
      r.env.down.value = false;
      r.advance(5_000);
      await r.poll();
      assert.equal(r.cardRow(t.id).state, "open");
      return { card: t.id, generations: r.gens(t.id).length };
    }),

  "queued-then-archived": () =>
    withRig(async (r) => {
      const { g1 } = await opened(r, { key: "k-qta" });
      const sdk = new FakeSdk();
      sdk.script = [{ kind: "throw", err: archived() }];
      const loop = new WakeLoop(r.svc, sdk);
      assert.ok(r.mkPick(g1.id).ok);
      await loop.drain();
      const w = r.svc.store.obligationsFor(g1.id).find((o) => o.kind === "wake")!;
      return { decision: g1.id, wake_state: r.svc.store.obligation(w.id)!.state, listed: r.svc.undeliverable().some((o) => o.decision_id === g1.id) };
    }),

  // ---- override and delegation ---------------------------------------------------------
  "delegated-override": () =>
    withRig(async (r) => {
      r.enableDelegation();
      const { t, g1 } = await opened(r, { key: "k-do", thread: "thr_a" });
      const sdk = new FakeSdk();
      const loop = new WakeLoop(r.svc, sdk);
      assert.ok(r.vizierPick(g1.id).ok);
      await loop.drain();
      const o = r.dele.override(g1.id, {});
      assert.ok(o.ok, JSON.stringify(o));
      const newId = (o as { decision_id: string }).decision_id;
      await loop.drain();
      assert.ok(r.mkPick(newId).ok);
      await loop.drain();
      const notices = r.svc.store.obligationsFor(g1.id).filter((x) => x.kind === "void-notice");
      const feed = feedText(r, "Autarch", "thr_a");
      const wakes = r.svc.store.obligationsFor(g1.id).filter((x) => x.kind === "wake");
      const newWake = sdk.sent.find((m) => m.input.includes(newId) && m.input.includes("Ruling on"));
      return {
        old_decision: g1.id,
        new_decision: newId,
        asker_wakes: wakes.length,
        void_notices: notices.length,
        old_label_in_feed: feed.split("\n").some((l) => l.startsWith("ruled ") && l.includes(`(${g1.id})`) && !l.includes("supersedes")),
        wake_supersedes: !!newWake && newWake.input.includes(g1.id),
        feed_supersedes: feed.split("\n").some((l) => l.includes(`(${newId})`) && l.includes(`supersedes ${g1.id}`)),
        new_generation: r.gens(t.id)[1]?.generation,
      };
    }),

  "override-unlabel-crash": () =>
    withRig(async (r) => {
      r.enableDelegation();
      const { t, g1 } = await opened(r, { key: "k-ouc" });
      assert.ok(r.vizierPick(g1.id).ok);
      r.fake.lostResponses.push({ method: "updateTask", nth: 1 });
      await r.poll();
      assert.equal(labelled(r, t.id), false);
      assert.equal(one(r, "SELECT state FROM card_writes WHERE kind = 'unlabel'").state, "pending");
      r.restart();
      const o = r.dele.override(g1.id, {});
      assert.ok(o.ok);
      await pollN(r, 10);
      assert.equal(r.cardRow(t.id).state, "open");
      assert.equal(labelled(r, t.id), true);
      assert.ok(r.mkPick((o as { decision_id: string }).decision_id).ok);
      return { card: t.id, state: "open", relabelled: true, generations: r.gens(t.id).length };
    }),

  "project-mismatch-delegation": () =>
    withRig(
      async (r) => {
        r.enableDelegation();
        const t = r.card({ key: "k-pmd" });
        r.edit(t, { description: r.desc({ key: "k-pmd", ask: { project: "Elsewhere", project_root: r.env.roots.Elsewhere } }) });
        await r.poll();
        assert.equal(r.gens(t.id).length, 0);
        assert.equal(r.cardRow(t.id).state, "display");
        assert.match(String(r.cardRow(t.id).display_reason), /^project mismatch: card in Autarch, ask targets Elsewhere/);
        const ruled = r.dele.rule(`card-${t.id}-g1`, "a", "reversible and routine", { threadId: "thr_viz" });
        assert.equal(ruled.ok, false);
        return { card: t.id, state: "display", generations: 0, delegated_rule_refused: true };
      },
      { projects: ["Autarch", "Elsewhere"] },
    ),

  // ---- request handling ------------------------------------------------------------------
  "duplicate-request-reversed": () =>
    withRig(async (r) => {
      const out: Record<string, unknown> = {};
      for (const reverse of [false, true]) {
        const k = reverse ? "k-dup-r" : "k-dup-f";
        const mk = (createdAt: string) => r.card({ key: k, createdAt });
        const [a, b] = reverse ? (() => { const late = mk("2026-10-01T00:00:09.000Z"); const early = mk("2026-10-01T00:00:01.000Z"); return [early, late]; })() : [mk("2026-10-01T00:00:01.000Z"), mk("2026-10-01T00:00:09.000Z")];
        await r.poll();
        assert.equal(r.cardRow(a.id).state, "open", `reverse=${reverse}`);
        assert.equal(r.cardRow(b.id).state, "display");
        assert.equal(r.cardRow(b.id).display_reason, `duplicate Request of ${k}`);
        out[reverse ? "reverse" : "forward"] = { canonical: a.id, duplicate: b.id };
      }
      return { ...out, canonical_in_both_orders: true };
    }),

  // ---- root run (display only) --------------------------------------------------------------
  "root-run-display": () =>
    withRig(async (r) => {
      const dir = r.env.dir;
      const script = join(dir, "run.sh");
      const text = "#!/bin/sh\necho hi\n";
      mkdirSync(dir, { recursive: true });
      writeFileSync(script, text);
      const block = (s: string) => `script: ${s}\nsha256: ${sha(text)}\ntimeout: 900\nset: myset`;
      const desc = (rr: string) => ["Prose", "", "Request: 6f1c-uuid sha256:0123456789abcdef", "", "```home-ask", JSON.stringify({ schema: "home-ask/v2", project: "Autarch", project_root: "/r", question: "Q?", options: [{ id: "a", label: "A", kind: "instruction", instruction: "do a", reversible: true }, { id: "b", label: "B", kind: "instruction", instruction: "do b", reversible: true }], ask_key: "k" }), "```", "", "```root-run", rr, "```"].join("\n");
      const TASK = "01J0000000000000000000000A";
      const comments = [{ id: "c1", kind: "agent", threadId: "thr_abc123", createdAt: "2026-09-30T12:00:00.5Z" }];
      const down = async () => {
        throw new Error("down");
      };
      const view = (s: string) => rootRun({ task: { id: TASK, projectId: "p:1", description: desc(block(s)) }, comments }, { fetch: down as never });
      const good = await view(script);
      const missing = await view(join(dir, "nope.sh"));
      assert.equal(good.state, "match");
      assert.equal(missing.state, "unreadable");
      assert.equal(missing.command, null);
      assert.ok(good.command);
      return { card: TASK, state: good.state, command_shown: true, command_hidden_when_unreadable: true, runner_status: good.status?.kind };
    }),

  "root-run-injection": () =>
    withRig(async (r) => {
      const TASK = "01J0000000000000000000000A";
      const calls: string[] = [];
      const spy = async (u: string) => {
        calls.push(String(u));
        throw new Error("down");
      };
      const ask = { schema: "home-ask/v2", project: "Autarch", project_root: "/r", question: "Q?", options: [{ id: "a", label: "A", kind: "instruction", instruction: "do a", reversible: true }, { id: "b", label: "B", kind: "instruction", instruction: "do b", reversible: true }], ask_key: "k" };
      const body = (set: string) => ["x", "", "Request: 6f1c-uuid sha256:0123456789abcdef", "", "```home-ask", JSON.stringify(ask), "```", "", "```root-run", `script: /a/b.sh\nsha256: ${"1".repeat(64)}\ntimeout: 60\nset: ${set}`, "```"].join("\n");
      const hostile = ["a'; rm -rf /; '", "$(id)", "x y", "a/../b"];
      const shown: (string | null)[] = [];
      for (const set of hostile) {
        const v = await rootRun({ task: { id: TASK, projectId: "p", description: body(set) }, comments: [{ id: "c", kind: "agent", threadId: "thr_a", createdAt: "2026-09-30T12:00:00Z" }] }, { fetch: spy as never });
        shown.push(v.command);
        assert.ok(v.command === null || /^todo-add --set '[^']*' --from-card 'card-[A-Za-z0-9]+\.json'$/.test(v.command), `unsafe command: ${v.command}`);
      }
      await rootRun({ task: { id: "bad'id", projectId: "p", description: body("s") }, comments: [] }, { fetch: spy as never });
      assert.deepEqual(calls.filter((u) => /bad'id|rm -rf|\$\(/.test(u)), []);
      return { hostile_sets: hostile.length, commands_unsafe: 0, runner_urls_with_hostile_input: 0 };
    }),
  // ---- filing (the real autarch binary; bb is faked) ------------------------------------------------------
  "card-file-retry": ({ autarch }) =>
    withFiling(autarch, async ({ r, server, file }) => {
      server.inject({ verb: "create", kind: "drop-after-commit" });
      const run = await file(key("retry"));
      assert.equal(run.code, 0, `filer exit ${run.code}: ${run.stderr}`);
      assert.equal(server.callsOf("create"), 1, "the create call is made once; recovery is a lookup, not a second create");
      const cards = needsMkCards(r);
      assert.equal(cards.length, 1);
      assert.equal(run.card, cards[0]!.id);
      assert.equal(agentComments(r, run.card!).length, 1);
      return { card: run.card, exit_code: run.code, cards: cards.length };
    }),

  "card-file-unknown": ({ autarch }) =>
    withFiling(autarch, async ({ r, server, file }) => {
      const k = key("unknown");
      server.inject({ verb: "comment", kind: "hang-after-commit" });
      const first = await file(k);
      assert.equal(first.code, 4, `first run exit ${first.code}: ${first.stderr}`);
      assert.equal(first.card, undefined);
      const [card] = needsMkCards(r);
      assert.ok(card, "the card was created before the comment hung");
      const again = await file(k);
      assert.equal(again.code, 0, `rerun exit ${again.code}: ${again.stderr}`);
      assert.equal(again.card, card.id);
      assert.equal(needsMkCards(r).length, 1);
      return { card: card.id, first_exit: first.code, rerun_exit: again.code, cards: needsMkCards(r).length, agent_comments: agentComments(r, card.id).length };
    }),

  "not-ready-at-start": ({ autarch }) =>
    withFiling(autarch, async ({ r, file }) => {
      const started = Date.now();
      const k = key("notready");
      const ready = r.svc.ready.bind(r.svc);
      r.svc.ready = () => false;
      const locked = await file(k);
      assert.equal(locked.code, 3, `locked exit ${locked.code}: ${locked.stderr}`);
      const cardsWhileLocked = r.fake.tasks.length;
      assert.equal(cardsWhileLocked, 0, "an unready Home must create no card");
      // The registry read itself reports not ready (exit 3), which is what stops the filer.
      const probe = await homeCli({ svc: r.svc, asks: new Asks(r.svc), catchup: new Catchup(r.svc, r.dele), rule: () => ({ ok: false, status: 400, error: "x" }) as never, isVizier: () => false }).run(["get", "--request", k], {});
      assert.equal(probe.exitCode, 3);
      assert.match(String(probe.stderr), /not ready/);
      r.svc.ready = ready;
      const ok = await file(k);
      assert.equal(ok.code, 0, `ready exit ${ok.code}: ${ok.stderr}`);
      await r.poll();
      const gens = r.gens(ok.card!);
      assert.equal(gens.length, 1, "the filed card materializes");
      const p = r.mkPick(gens[0].id);
      assert.ok(p.ok, `pick failed: ${JSON.stringify(p)}`);
      return { ask_exit_locked: locked.code, cards_while_locked: cardsWhileLocked, not_ready: true, decision: gens[0].id, pick_id: (p as { pick: { pick_id: string } }).pick.pick_id, seconds: (Date.now() - started) / 1000 };
    }),

  "filer-registry-authority": ({ autarch }) =>
    withFiling(autarch, async ({ r, file, askFile }) => {
      const k = key("authority");
      const first = await file(k);
      assert.equal(first.code, 0, `exit ${first.code}: ${first.stderr}`);
      await r.poll(); // Home registers the card: card_requests now names it
      const reg = r.db.prepare("SELECT task_id FROM card_requests WHERE request_key = ?").get(k) as { task_id: string } | undefined;
      assert.equal(reg?.task_id, first.card);
      // A registered Request is answered from the registry: same card, no search, no second card.
      const again = await file(k);
      assert.equal(again.code, 0);
      assert.equal(again.card, first.card);
      // The registered card is edited in tasks: the Request line no longer reads back, so the filer refuses (exit 2)
      // and, above all, does not file a second card for the key.
      const task = r.fake.tasks.find((t) => t.id === first.card)!;
      r.edit(task, { description: task.description.replace(/^Request: .*$/m, `Request: ${k} sha256:ffffffffffffffff`) });
      const changed = await file(k);
      assert.equal(changed.code, 2, `edited-card exit ${changed.code}: ${changed.stderr}`);
      assert.equal(needsMkCards(r).length, 1, "no second card");
      void askFile;
      return { card: first.card, cards: needsMkCards(r).length, registry_row: true };
    }),
};
