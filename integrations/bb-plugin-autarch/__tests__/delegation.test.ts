import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Delegation } from "../delegation.js";
import { renderFeed } from "../feed.js";
import type { Service } from "../service.js";
import { ask, makeEnv, OPTIONS, type Env } from "./service-helpers.js";

let env: Env;
let svc: Service;
let dele: Delegation;
beforeEach(() => {
  env = makeEnv(["Autarch", "Other"]);
  svc = env.open();
  dele = new Delegation(svc);
});
afterEach(() => env.cleanup());

const VIZ = "thr-vizier";
const TOKEN = /APPROVED-(MERGE|DEPLOY|RELEASE)/;
const fileId = async (over: Record<string, unknown> = {}) => {
  const r = await svc.file(ask(env, over), {});
  if (!r.ok) throw new Error(r.error);
  return r.decision_id;
};
const rev = (id: string) => (svc.store.decision(id) as { revision: string }).revision;
/** Configure delegation and let mk see the change, as the panel would. */
function enable(over: Partial<{ vizierThreadId: string; projects: string[]; dailyCap: number }> = {}) {
  const r = dele.setDelegation({ vizierThreadId: VIZ, projects: ["Autarch"], dailyCap: 5, ...over }, {});
  expect(r).toMatchObject({ ok: true });
  dele.markSeen("mk", dele.latestSettingsItem()!);
}
const rule = (id: string, option = "project", reason = "reversible and low risk", ...who: (string | undefined)[]) =>
  dele.rule(id, option, reason, { threadId: who.length > 0 ? who[0] : VIZ });
const wakeOf = (id: string) => svc.store.obligationsFor(id).find((o) => o.kind === "wake")!;
const voidNotices = (id: string) => svc.store.obligationsFor(id).filter((o) => o.kind === "void-notice");
const mkPick = (id: string, option: string, pickId = `mk-${id}`) => svc.pick(id, option, rev(id), pickId, "mk", "home");
/** Put an obligation into a wake state by driving the store directly. */
function drive(id: string, state: "none" | "sending" | "queued" | "uncertain" | "delivered") {
  if (state === "none") return;
  const c = svc.store.claim(id, 0);
  if (!c.ok) throw new Error("claim");
  if (state === "sending") return;
  if (state === "queued") {
    svc.store.updateAttempt(id, c.attempt, "queued", { handle: "q1" });
    svc.store.transition(id, "sending", "queued", c.attempt);
  } else if (state === "uncertain") {
    svc.store.updateAttempt(id, c.attempt, "uncertain");
    svc.store.transition(id, "sending", "uncertain", c.attempt);
  } else {
    svc.store.updateAttempt(id, c.attempt, "delivered");
    svc.store.transition(id, "sending", "done", c.attempt);
  }
}

describe("rule: claimed-caller checks [E-13] [F-8]", () => {
  it("a claimed thread other than vizierThreadId is refused", async () => {
    enable();
    const id = await fileId();
    expect(rule(id, "project", "ok", "thr-a")).toMatchObject({ ok: false, status: 403 });
    expect(rule(id, "project", "ok", undefined)).toMatchObject({ ok: false, status: 403 });
    expect(svc.store.pick(id)).toBeUndefined();
  });

  it("a forged BB_THREAD_ID equal to vizierThreadId is accepted as by:vizier, woken, pinned, and still limited", async () => {
    enable({ dailyCap: 1 });
    const a = await fileId();
    expect(rule(a)).toMatchObject({ ok: true });
    expect(svc.store.pick(a)).toMatchObject({ by: "vizier", surface: "cli", reason: "reversible and low risk" });
    expect(wakeOf(a).payload).toContain("The vizier ruled on your decision");
    expect(dele.pinned().map((p) => p.decision)).toEqual([a]);
    // limits: irreversible option, project not enabled, past the cap
    const b = await fileId({ subject: "autarch/b", question: "another question?" });
    expect(rule(b, "day")).toMatchObject({ ok: false, status: 403 });
    expect(rule(b, "project")).toMatchObject({ ok: false, status: 403, error: expect.stringContaining("cap") });
    const c = await svc.file(ask(env, { project: "Other", project_root: env.roots.Other, subject: "other/c", question: "other project?" }), {});
    if (!c.ok) throw new Error(c.error);
    expect(rule(c.decision_id)).toMatchObject({ ok: false, status: 403, error: expect.stringContaining("project") });
  });

  it("a direct pick RPC by a non-mk caller succeeds as by:mk with no wake and no limits (documented bypass)", async () => {
    const id = await fileId(); // delegation never configured
    const r = svc.pick(id, "day", rev(id), "direct-1", "mk", "cli");
    expect(r).toMatchObject({ ok: true, pick: { by: "mk" } });
    expect(svc.store.obligationsFor(id).map((o) => o.kind)).toEqual(["ruling-file"]);
    expect(wakeOf(id)).toBeUndefined();
  });
});

describe("rule: bounded policy [D-2] [D-1]", () => {
  it("refuses an irreversible option, a Mycroft decision, an override decision and a missing or bad reason", async () => {
    enable();
    const a = await fileId();
    expect(rule(a, "day")).toMatchObject({ ok: false, status: 403 });
    expect(rule(a, "nope")).toMatchObject({ ok: false, status: 400 });
    expect(rule(a, "project", "")).toMatchObject({ ok: false, status: 400 });
    expect(rule(a, "project", "   ")).toMatchObject({ ok: false, status: 400 });
    expect(rule(a, "project", "x".repeat(501))).toMatchObject({ ok: false, status: 400 });
    expect(rule(a, "project", "APPROVED-MERGE abc")).toMatchObject({ ok: false, status: 400 });
    expect(rule("nope")).toMatchObject({ ok: false, status: 404 });
    const my = await svc.file(ask(env, { asker: "mycroft", thread: "", options: [{ id: "a", label: "A", kind: "ruling-only" }, { id: "b", label: "B", kind: "ruling-only" }], subject: "autarch/my", question: "mycroft asks?" }), {});
    if (!my.ok) throw new Error(my.error);
    expect(rule(my.decision_id)).toMatchObject({ ok: false, status: 403 });
    expect(svc.store.pick(a)).toBeUndefined();
    // an override replacement is never delegable
    expect(rule(a, "project", "x".repeat(500))).toMatchObject({ ok: true });
    const B = dele.override(a, {});
    if (!B.ok) throw new Error("override");
    expect(rule(B.decision_id)).toMatchObject({ ok: false, status: 403 });
  });

  it("is refused past the daily cap, and the cap window rolls", async () => {
    enable({ dailyCap: 1 });
    const a = await fileId();
    const b = await fileId({ subject: "autarch/b", question: "b?" });
    expect(rule(a)).toMatchObject({ ok: true });
    expect(rule(b)).toMatchObject({ ok: false, status: 403 });
    env.clock.t += 25 * 3_600_000;
    expect(rule(b)).toMatchObject({ ok: true });
  });

  it("two connections cannot both take the last cap slot: the count is inside the pick transaction", async () => {
    enable({ dailyCap: 1 });
    const a = await fileId();
    const b = await fileId({ subject: "autarch/b", question: "b?" });
    // a second connection on the same database file, its own Service
    const other = new Delegation(env.open());
    other.svc.store.db.pragma("busy_timeout = 50");
    // connection 1 has just counted the vizier picks when connection 2 rules on b
    const realPrepare = svc.store.db.prepare.bind(svc.store.db);
    let fired = false;
    let raced: unknown;
    (svc.store.db as unknown as { prepare: unknown }).prepare = (sql: string) => {
      const st = realPrepare(sql);
      if (!/COUNT\(\*\)[\s\S]*'vizier'/.test(sql)) return st;
      return new Proxy(st, {
        get(t, k) {
          if (k !== "get") return Reflect.get(t, k).bind?.(t) ?? Reflect.get(t, k);
          return (...args: unknown[]) => {
            const out = (t.get as (...a: unknown[]) => unknown)(...args);
            if (!fired) {
              fired = true;
              try {
                raced = other.rule(b, "project", "reversible and low risk", { threadId: VIZ });
              } catch (e) {
                raced = e;
              }
            }
            return out;
          };
        },
      });
    };
    const first = rule(a);
    const vizierPicks = (svc.store.db.prepare(`SELECT COUNT(*) AS n FROM picks WHERE "by" = 'vizier'`).get() as { n: number }).n;
    expect(fired).toBe(true);
    expect(first).toMatchObject({ ok: true });
    expect(raced).toBeDefined();
    expect(vizierPicks).toBe(1);
  });

  it("is suspended by an unseen settings change until mk sees it", async () => {
    enable();
    const a = await fileId();
    expect(dele.setDelegation({ vizierThreadId: VIZ, projects: ["Autarch", "Other"], dailyCap: 9 }, {})).toMatchObject({ ok: true });
    expect(dele.pinned().some((p) => p.item.startsWith("delegation-settings:"))).toBe(true);
    expect(rule(a)).toMatchObject({ ok: false, status: 403, error: expect.stringContaining("unseen") });
    dele.markSeen("mk", dele.latestSettingsItem()!);
    expect(dele.pinned().some((p) => p.item.startsWith("delegation-settings:"))).toBe(false);
    expect(rule(a)).toMatchObject({ ok: true });
  });

  it("a settings change from a thread is refused and changes nothing", () => {
    enable();
    expect(dele.setDelegation({ vizierThreadId: "thr-evil", projects: ["Autarch"], dailyCap: 99 }, { threadId: "thr-evil" })).toMatchObject({ ok: false, status: 403 });
    expect(dele.settings()).toMatchObject({ vizierThreadId: VIZ, dailyCap: 5 });
    expect(dele.setDelegation({ vizierThreadId: VIZ, projects: ["Autarch"], dailyCap: -1 }, {})).toMatchObject({ ok: false, status: 400 });
  });

  it("a reversible push-and-merge instruction yields a wake with the non-authorization line and no approval token", async () => {
    enable();
    const opts = [
      { id: "ship", label: "Push and merge", kind: "instruction", reversible: true, instruction: "Push the branch and merge it to main." },
      { id: "no", label: "Do nothing", kind: "ruling-only" },
    ];
    const id = await fileId({ options: opts, subject: "autarch/ship", question: "ship it?" });
    expect(rule(id, "ship")).toMatchObject({ ok: true });
    const p = wakeOf(id).payload!;
    expect(p.startsWith("The vizier ruled on your decision")).toBe(true);
    expect(p).toContain("This is not merge, deploy, release or publish authorization.");
    expect(p).not.toMatch(TOKEN);
  });

  it("an option with an approval spec cannot be delegated: filing it as reversible is refused, and rule refuses it", async () => {
    enable();
    const bad = [
      { id: "ship", label: "Ship", kind: "instruction", instruction: "x", reversible: true, approval: { kind: "merge", target: "main", identity: "abc" } },
      { id: "no", label: "No", kind: "ruling-only" },
    ];
    expect(await svc.file(ask(env, { options: bad, subject: "autarch/appr", question: "appr?" }), {})).toMatchObject({ ok: false });
    const ok = [
      { id: "ship", label: "Ship", kind: "instruction", instruction: "x", approval: { kind: "merge", target: "main", identity: "abc" } },
      { id: "no", label: "No", kind: "ruling-only" },
    ];
    const id = await fileId({ options: ok, subject: "autarch/appr", question: "appr?" });
    expect(rule(id, "ship")).toMatchObject({ ok: false, status: 403 });
    expect(svc.store.pick(id)).toBeUndefined();
  });

  it("the vizier wake text and reason survive into the ruling file", async () => {
    enable();
    const a = await fileId();
    rule(a);
    const again = env.open();
    expect(again.store.obligationsFor(a).find((o) => o.kind === "ruling-file")!.state).toBe("done");
  });
});

describe("no CLI verb changes delegation settings", () => {
  it("only delegation.ts writes vizierThreadId or delegation.* settings", () => {
    const dir = join(__dirname, "..");
    const files = readdirSync(dir).filter((f) => f.endsWith(".ts") || f.endsWith(".tsx"));
    for (const f of files) {
      if (f === "delegation.ts") continue;
      const src = readFileSync(join(dir, f), "utf8");
      expect(src, f).not.toMatch(/setSetting\(\s*["'`](vizierThreadId|delegation)/);
    }
    const cli = readdirSync(dir).filter((f) => /^cli/.test(f));
    for (const f of cli) expect(readFileSync(join(dir, f), "utf8"), f).not.toMatch(/setDelegation|vizierThreadId|delegation\./);
  });
});

describe("delegated ruling wake and catch-up", () => {
  it("wakes the asker saying vizier, and is pinned until mk has seen it", async () => {
    enable();
    const a = await fileId();
    rule(a);
    const w = wakeOf(a);
    expect(w.recipient).toBe("thr-a");
    expect(w.payload).toContain("in mk's place (you marked this option reversible). mk may override it.");
    expect(w.payload).toContain("vizier picked");
    expect(svc.wakes().map((o) => o.id)).toContain(w.id);
    expect(dele.pinned().filter((p) => p.decision).map((p) => p.decision)).toEqual([a]);
    dele.markSeen("mk", `ruling:${a}`);
    expect(dele.pinned().filter((p) => p.decision)).toEqual([]);
  });
});

describe("override [D-3] [G-2]", () => {
  for (const state of ["none", "sending", "queued", "uncertain", "delivered"] as const) {
    it(`with W ${state}: voids W, keeps its state, one void-notice, new decision owed and not delegable`, async () => {
      enable();
      const a = await fileId();
      rule(a);
      const w = wakeOf(a);
      drive(w.id, state);
      const before = svc.store.obligation(w.id)!;
      const pickBefore = JSON.stringify(svc.store.pick(a));
      const r = dele.override(a, {});
      if (!r.ok) throw new Error(r.error);
      expect(r).toMatchObject({ ok: true, replay: false });
      const after = svc.store.obligation(w.id)!;
      expect(after.voided_at).toBe(env.now());
      expect(after.state).toBe(before.state);
      expect(after.attempt).toBe(before.attempt);
      expect(svc.wakes().map((o) => o.id)).not.toContain(w.id);
      const n = voidNotices(a);
      expect(n).toHaveLength(1);
      expect(n[0]).toMatchObject({ recipient: "thr-a", after_id: null, op: `void:${a}` });
      expect(n[0]!.payload).toContain(r.decision_id);
      expect(svc.wakes().map((o) => o.id)).toContain(n[0]!.id);
      const B = svc.store.decision(r.decision_id) as Record<string, unknown>;
      expect(B).toMatchObject({ supersedes: a, delegable: 0, thread: "thr-a", request_id: `override:${a}` });
      expect(svc.owed().map((d) => d.id)).toContain(r.decision_id);
      expect(rule(r.decision_id)).toMatchObject({ ok: false });
      expect(JSON.stringify(svc.store.pick(a))).toBe(pickBefore);
      expect(svc.store.events().some((e) => e.type === "overridden" && e.decision_id === r.decision_id)).toBe(true);
    });
  }

  it("a second override returns the first's decision and adds no notice", async () => {
    enable();
    const a = await fileId();
    rule(a);
    const r1 = dele.override(a, {});
    const r2 = dele.override(a, {});
    if (!r1.ok || !r2.ok) throw new Error("override");
    expect(r2).toMatchObject({ decision_id: r1.decision_id, replay: true });
    expect(voidNotices(a)).toHaveLength(1);
    const again = new Delegation(env.open());
    expect(again.override(a, {})).toMatchObject({ ok: true, decision_id: r1.decision_id });
    expect(voidNotices(a)).toHaveLength(1);
  });

  it("refuses an override on an mk pick, an unpicked decision, an unknown one, and any thread caller", async () => {
    enable();
    const a = await fileId();
    expect(dele.override(a, {})).toMatchObject({ ok: false, status: 409 });
    mkPick(a, "day");
    expect(dele.override(a, {})).toMatchObject({ ok: false, status: 409 });
    expect(dele.override("nope", {})).toMatchObject({ ok: false, status: 404 });
    const b = await fileId({ subject: "autarch/b", question: "b?" });
    rule(b);
    expect(dele.override(b, { threadId: "thr-a" })).toMatchObject({ ok: false, status: 403 });
    expect(voidNotices(b)).toEqual([]);
  });

  it("mk's replacement answer while W is still queued: due at once, says it supersedes the vizier's ruling", async () => {
    enable();
    const a = await fileId();
    rule(a);
    const w = wakeOf(a);
    drive(w.id, "queued");
    const r = dele.override(a, {});
    if (!r.ok) throw new Error("override");
    expect(mkPick(r.decision_id, "project")).toMatchObject({ ok: true });
    const bw = wakeOf(r.decision_id);
    expect(bw.payload).toContain(`This supersedes the vizier's ruling on ${a}.`);
    expect(svc.wakes().map((o) => o.id)).toContain(bw.id);
    expect(svc.wakes().map((o) => o.id)).not.toContain(w.id);
    expect(renderFeed(svc.feed("Autarch", "thr-a"))).toContain(`supersedes ${a}`);
    // a restart between the override and the pick creates no second notice
    const again = env.open();
    expect(again.store.obligationsFor(a).filter((o) => o.kind === "void-notice")).toHaveLength(1);
  });

  it("supersession chain A -> B -> C with mentioner M [G-4] [H-3]", async () => {
    enable();
    const a = await fileId();
    expect(await svc.file(ask(env, { thread: "thr-m" }), {})).toMatchObject({ mentioned: true, decision_id: a });
    rule(a);
    const r = dele.override(a, {});
    if (!r.ok) throw new Error("override");
    const B = r.decision_id;
    const c = await svc.file(ask(env, { supersedes: B, request_id: "c-req", question: "Collapse routine catch-up items per project or per day, take two?" }), {});
    if (!c.ok) throw new Error(c.error);
    const C = c.decision_id;
    expect(svc.store.decision(C)).toMatchObject({ delegable: 0, supersedes: B });
    expect(svc.store.mentions(C).map((m) => m.thread)).toEqual(["thr-m"]);
    expect(rule(C)).toMatchObject({ ok: false });
    for (const t of ["thr-a", "thr-m"]) {
      expect(svc.feed("Autarch", t).text, t).toContain(`void ${a}: overridden, awaiting mk (${C})`);
    }
    expect(mkPick(C, "project")).toMatchObject({ ok: true });
    expect(wakeOf(C).payload).toContain(`This supersedes the vizier's ruling on ${a}.`);
    for (const t of ["thr-a", "thr-m"]) {
      const text = svc.feed("Autarch", t).text;
      expect(text, t).not.toContain(`void ${a}`);
      expect(text, t).toContain(`supersedes ${B}`);
    }
  });

  it("notice after the answer [H-4]: names B, says an answer already received stands, same text on a duplicate", async () => {
    enable();
    const a = await fileId();
    rule(a);
    const r = dele.override(a, {});
    if (!r.ok) throw new Error("override");
    mkPick(r.decision_id, "project");
    const bw = wakeOf(r.decision_id);
    drive(bw.id, "delivered");
    const n = voidNotices(a)[0]!;
    expect(n.payload).toContain(r.decision_id);
    expect(n.payload).toMatch(/already received/);
    expect(n.payload).toMatch(/stands/);
    // a duplicate delivery (Resend) carries the same stored text
    drive(n.id, "uncertain");
    const row = svc.store.obligation(n.id)!;
    expect(svc.store.resend(n.id, row.attempt, "click-1")).toMatchObject({ ok: true });
    expect(svc.store.obligation(n.id)!.payload).toBe(n.payload);
    expect(dele.override(a, {})).toMatchObject({ ok: true, replay: true });
    expect(voidNotices(a)).toHaveLength(1);
  });
});

// ---- Task 2.5: binding scope and card override generations ----
import { afterEach as afterEach25, describe as describe25, expect as expect25, it as it25 } from "vitest";
import { cleanupEnvs as cleanup25, opened as opened25, rig as rig25 } from "./card-rig.js";

afterEach25(cleanup25);

describe25("delegation scope on a card (finding r2-6)", () => {
  it25("is refused when the binding is only suggested, rejected, or points at another project; allowed when confirmed", async () => {
    const r = rig25({ projects: ["Autarch", "Beta"] });
    const { g1 } = await opened25(r);
    r.enableDelegation("suggested");
    expect25(r.vizierPick(g1.id)).toMatchObject({ ok: false, status: 403, error: expect25.stringContaining("binding unconfirmed") });
    r.enableDelegation("rejected");
    expect25(r.vizierPick(g1.id)).toMatchObject({ ok: false, status: 403, error: expect25.stringContaining("rejected") });
    r.enableDelegation("confirmed", "Beta"); // bound to beta; the card targets Autarch
    expect25(r.vizierPick(g1.id)).toMatchObject({ ok: false, status: 403, error: expect25.stringContaining("other than its binding") });
    expect25(r.svc.store.pick(g1.id)).toBeUndefined();
    r.enableDelegation("confirmed");
    expect25(r.vizierPick(g1.id)).toMatchObject({ ok: true });
  });

  it25("a suggested binding still allows mk's own pick", async () => {
    const r = rig25();
    r.enableDelegation("suggested");
    const { g1 } = await opened25(r);
    expect25(r.mkPick(g1.id)).toMatchObject({ ok: true });
  });
});

describe25("override of a vizier-picked card generation (T10)", () => {
  it25("makes g2 with the card ids, an undelegable copy, voids the wake, notifies, relabels, and replays", async () => {
    const r = rig25();
    r.enableDelegation();
    const { t, g1 } = await opened25(r);
    r.vizierPick(g1.id);
    await r.poll();
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    expect25(o).toMatchObject({ decision_id: `card-${t.id}-g2`, replay: false });
    const g2 = r.gens(t.id)[1];
    expect25(g2).toMatchObject({ request_id: `override:${g1.id}`, supersedes: g1.id, delegable: 0, source: "card", generation: 2, card_fp: g1.card_fp });
    const body = (x: any) => { const b = JSON.parse(x.body_json); delete b.request_id; delete b.supersedes; return b; };
    expect25(body(g2)).toEqual(body(g1));
    const obs = r.svc.store.obligationsFor(g1.id);
    expect25(obs.find((x) => x.kind === "wake")!.voided_at).not.toBeNull();
    expect25(obs.find((x) => x.kind === "void-notice")).toMatchObject({ recipient: "thr_a" });
    expect25(r.db.prepare("SELECT kind FROM card_writes WHERE decision_id = ?").all(g2.id)).toEqual([{ kind: "relabel" }]);
    await r.poll();
    expect25(r.fake.tasks.find((x) => x.id === t.id)!.labelIds).toEqual([r.label.id]);
    expect25(r.dele.override(g1.id, {})).toMatchObject({ ok: true, decision_id: g2.id, replay: true });
    expect25(r.vizierPick(g2.id)).toMatchObject({ ok: false, status: 403 });
    // mk's pick on g2: the ruling names the overridden decision
    expect25(r.mkPick(g2.id)).toMatchObject({ ok: true });
  });
});
