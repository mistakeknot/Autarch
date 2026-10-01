import { chmodSync, existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { parseAsk, identity, revision, semanticKey, normalizedJson } from "../model.js";
import { parseRuling } from "../ruling.js";
import type { Service } from "../service.js";
import { pickOf } from "./helpers.js";
import { ask, makeEnv, OPTIONS, T0, type Env } from "./service-helpers.js";

let env: Env;
let svc: Service;
beforeEach(() => {
  env = makeEnv(["Autarch", "Other"]);
  svc = env.open();
});
afterEach(() => env.cleanup());

const rev = (id: string) => (svc.store.decision(id) as { revision: string }).revision;
const fileOk = async (over: Record<string, unknown> = {}, ctx = {}) => {
  const r = await svc.file(ask(env, over), ctx);
  if (!r.ok) throw new Error(`file failed: ${r.error}`);
  return r;
};
const pickOk = (id: string, opt: string, pickId = `p-${id}-${opt}`, by = "mk") => {
  const r = svc.pick(id, opt, rev(id), pickId, by, "home");
  if (!r.ok) throw new Error(`pick failed: ${r.error}`);
  return r;
};
const rulingFiles = (project = "Autarch") => {
  const dir = join(env.roots[project]!, "docs", "decisions");
  return existsSync(dir) ? readdirSync(dir).filter((f) => f.endsWith(".md")) : [];
};
const ids = (rows: { id: string }[]) => rows.map((r) => r.id);

describe("file", () => {
  it("files twice with one request_id: one row, one event, and the recorded result", async () => {
    const a = await fileOk({ request_id: "req-1" });
    const b = await svc.file(ask(env, { request_id: "req-1" }), {});
    expect(b).toMatchObject({ ok: true, status: 200, decision_id: a.decision_id });
    expect(a.status).toBe(201);
    expect((svc.store.db.prepare("SELECT COUNT(*) n FROM decisions").get() as { n: number }).n).toBe(1);
    expect(svc.store.events().filter((e) => e.type === "filed")).toHaveLength(1);
  });

  it("refuses a reused request_id with a different thread or project (409)", async () => {
    await fileOk({ request_id: "req-1" });
    const t = await svc.file(ask(env, { request_id: "req-1", thread: "thr-b" }), {});
    expect(t).toMatchObject({ ok: false, status: 409, error: "request id reused for a different decision" });
    const p = await svc.file(ask(env, { request_id: "req-1", project: "Other", project_root: env.roots.Other }), {});
    expect(p).toMatchObject({ ok: false, status: 409 });
  });

  it("validation failures are exit 2, supersedes with mention_of included", async () => {
    const r = await svc.file(ask(env, { question: "" }), {});
    expect(r).toMatchObject({ ok: false, status: 400, exit: 2 });
    const a = await fileOk();
    const s = await svc.file(ask(env, { supersedes: a.decision_id, mention_of: a.decision_id }), {});
    expect(s).toMatchObject({ ok: false, exit: 2 });
  });

  it("requires ctx.threadId, when present, to equal the asker's thread [D-16]", async () => {
    const bad = await svc.file(ask(env), { threadId: "thr-other" });
    expect(bad).toMatchObject({ ok: false, exit: 2 });
    const ok = await svc.file(ask(env), { threadId: "thr-a" });
    expect(ok.ok).toBe(true);
  });

  it("saves the root's dev and ino from serve, and refuses a root that serve does not resolve", async () => {
    const a = await fileOk();
    const d = svc.store.decision(a.decision_id) as { project_root: string; root_dev: string; root_ino: string };
    expect(d.project_root).toBe(env.roots.Autarch);
    expect(d.root_dev).toBe(String(env.projects[0]!.dev));
    expect(d.root_ino).toBe(String(env.projects[0]!.ino));
    const bad = await svc.file(ask(env, { project_root: "/somewhere/else" }), {});
    expect(bad).toMatchObject({ ok: false, status: 400 });
    const unknown = await svc.file(ask(env, { project: "Nope", project_root: "/x" }), {});
    expect(unknown).toMatchObject({ ok: false, status: 400 });
  });

  it("retry while serve is down returns the same result (200), a new request exits 3 [E-10]", async () => {
    const a = await fileOk({ request_id: "req-1" });
    env.down.value = true;
    const again = await svc.file(ask(env, { request_id: "req-1" }), {});
    expect(again).toMatchObject({ ok: true, status: 200, decision_id: a.decision_id });
    const fresh = await svc.file(ask(env, { request_id: "req-2", question: "another?" }), {});
    expect(fresh).toMatchObject({ ok: false, exit: 3, error: "not-filed: project resolution unavailable" });
  });

  it("refuses estate filing when the Uqbar is unset", async () => {
    const r = await svc.file(ask(env, { project: "estate", project_root: "/tmp/uqbar" }), {});
    expect(r).toMatchObject({ ok: false, error: "estate-wide decisions need an Uqbar (see G-1)" });
  });
});

describe("pick", () => {
  it("an instruction pick writes the ruling file and one wake with the exact instruction", async () => {
    const a = await fileOk();
    const p = pickOk(a.decision_id, "project");
    expect(p.pick.by).toBe("mk");
    const files = rulingFiles();
    expect(files).toHaveLength(1);
    const r = parseRuling(readFileSync(join(env.roots.Autarch!, "docs", "decisions", files[0]!), "utf8"));
    expect(r).toMatchObject({ decision_id: a.decision_id, picked: "project", ruled_by: "mk", asking_thread: "thr-a" });
    expect(r.instruction).toBe(OPTIONS[0]!.instruction);
    expect(r.options_shown.map((o) => o.id)).toEqual(["project", "day", "ask"]);
    const obs = svc.store.obligationsFor(a.decision_id);
    expect(obs.map((o) => [o.kind, o.state])).toEqual([["ruling-file", "done"], ["wake", "pending"]]);
    const wakes = svc.wakes();
    expect(wakes).toHaveLength(1);
    expect(wakes[0]).toMatchObject({ recipient: "thr-a", op: `wake:${a.decision_id}:${p.pick.pick_id}` });
    expect(wakes[0]!.payload).toContain(OPTIONS[0]!.instruction);
    expect(wakes[0]!.payload).toContain("Collapse per project");
  });

  it("a needs-context pick wakes; a ruling-only pick by mk does not, and the feed carries it", async () => {
    const a = await fileOk();
    pickOk(a.decision_id, "ask");
    expect(svc.wakes()).toHaveLength(1);
    const b = await fileOk({ question: "second question?", subject: "autarch/second" });
    pickOk(b.decision_id, "day");
    expect(svc.wakes()).toHaveLength(1);
    expect(svc.store.obligationsFor(b.decision_id).map((o) => o.kind)).toEqual(["ruling-file"]);
    expect(svc.feed("Autarch", "thr-a").own.map((l) => l.decision)).toContain(b.decision_id);
  });

  it("retry with the same pick_id: one pick; same pick_id and other option: 409 pick id reused", async () => {
    const a = await fileOk();
    const p1 = pickOk(a.decision_id, "project", "pk-1");
    const p2 = svc.pick(a.decision_id, "project", rev(a.decision_id), "pk-1", "mk", "home");
    expect(p2).toMatchObject({ ok: true, status: 200 });
    expect((svc.store.db.prepare("SELECT COUNT(*) n FROM picks").get() as { n: number }).n).toBe(1);
    expect(p1.pick.pick_id).toBe("pk-1");
    const p3 = svc.pick(a.decision_id, "day", rev(a.decision_id), "pk-1", "mk", "home");
    expect(p3).toMatchObject({ ok: false, status: 409, error: "pick id reused" });
    // a pick id already used on another decision
    const b = await fileOk({ question: "b?", subject: "autarch/b" });
    expect(svc.pick(b.decision_id, "day", rev(b.decision_id), "pk-1", "mk", "home")).toMatchObject({ ok: false, status: 409, error: "pick id reused" });
    expect(svc.store.pick(b.decision_id)).toBeUndefined();
  });

  it("two surfaces race: one wins, the other gets 409 already ruled", async () => {
    const a = await fileOk();
    const other = env.open();
    const w = svc.pick(a.decision_id, "day", rev(a.decision_id), "pk-w", "mk", "home");
    const l = other.pick(a.decision_id, "project", rev(a.decision_id), "pk-l", "mk", "overlay");
    expect(w.ok).toBe(true);
    expect(l).toMatchObject({ ok: false, status: 409, error: "already ruled" });
    expect(svc.store.pick(a.decision_id)!.option_id).toBe("day");
  });

  it("rejects an unknown decision, an unknown option and a stale revision", async () => {
    const a = await fileOk();
    expect(svc.pick("nope", "day", "r", "pk", "mk", "home")).toMatchObject({ ok: false, status: 404 });
    expect(svc.pick(a.decision_id, "zzz", rev(a.decision_id), "pk", "mk", "home")).toMatchObject({ ok: false, status: 400 });
    expect(svc.pick(a.decision_id, "day", "old-rev", "pk", "mk", "home")).toMatchObject({ ok: false, status: 409, error: "revision differs" });
    expect(svc.store.pick(a.decision_id)).toBeUndefined();
  });

  it("a stored revision that does not match its body is a store fault (500)", async () => {
    const a = await fileOk();
    // v3 makes the ask immutable by trigger; drop it to simulate out-of-band corruption.
    svc.store.db.exec("DROP TRIGGER decisions_ask_immutable");
    svc.store.db.prepare("UPDATE decisions SET revision = 'tampered' WHERE id = ?").run(a.decision_id);
    const r = svc.pick(a.decision_id, "day", "tampered", "pk", "mk", "home");
    expect(r).toMatchObject({ ok: false, status: 500 });
  });

  it("the stored revision is the model's revision of the stored body", async () => {
    const a = await fileOk();
    const d = svc.store.decision(a.decision_id) as { body_json: string; revision: string; identity: string; semantic_key: string };
    const back = parseAsk(JSON.parse(d.body_json));
    expect(revision(back)).toBe(d.revision);
    expect(identity(back)).toBe(d.identity);
    expect(semanticKey(back)).toBe(d.semantic_key);
    expect(normalizedJson(back)).toBe(d.body_json);
  });

  it("picking a machine ask is refused", async () => {
    const s = await fileOk({ kind: "machine", options: undefined, machine: { class: "ci", detail: "down" }, question: "ci is down", subject: "autarch/ci" });
    expect(svc.pick(s.decision_id, "done", rev(s.decision_id), "pk", "mk", "home")).toMatchObject({ ok: false, status: 400 });
  });
});

describe("replacement", () => {
  it("replacement before pick: A superseded (409), not owed; B owed [C-4] [C-5]", async () => {
    const a = await fileOk();
    const b = await fileOk({ supersedes: a.decision_id, question: "Collapse per project or per day, revised?" });
    expect(b.status).toBe(201);
    const r = svc.pick(a.decision_id, "day", rev(a.decision_id), "pk", "mk", "home");
    expect(r).toMatchObject({ ok: false, status: 409 });
    expect((r as { error: string }).error).toContain("superseded");
    expect(ids(svc.owed())).toEqual([b.decision_id]);
    expect(svc.store.obligationsFor(a.decision_id)).toEqual([]);
    expect(svc.store.obligationsFor(b.decision_id)).toEqual([]);
  });

  it("two replacements: the second gets 409 already superseded by the first", async () => {
    const a = await fileOk();
    const b = await fileOk({ supersedes: a.decision_id, question: "b?" });
    const c = await svc.file(ask(env, { supersedes: a.decision_id, question: "c?" }), {});
    expect(c).toMatchObject({ ok: false, status: 409, error: `already superseded by ${b.decision_id}` });
  });

  it("a machine or steps ask is not replaceable, and nothing is inserted [I-2]", async () => {
    const m = await fileOk({ kind: "machine", options: undefined, machine: { class: "ci", detail: "runner is down" }, question: "runner down", subject: "ci/runner" });
    const before = (svc.store.db.prepare("SELECT COUNT(*) n FROM decisions").get() as { n: number }).n;
    const b = await svc.file(ask(env, { supersedes: m.decision_id, question: "b?" }), {});
    expect(b).toMatchObject({ ok: false, status: 409, error: "only decide asks can be superseded" });
    const s = await fileOk({ kind: "steps", options: undefined, steps: ["x"], question: "steps", subject: "s" });
    const b2 = await svc.file(ask(env, { supersedes: s.decision_id, question: "b2?" }), {});
    expect(b2).toMatchObject({ ok: false, status: 409, error: "only decide asks can be superseded" });
    expect((svc.store.db.prepare("SELECT COUNT(*) n FROM decisions").get() as { n: number }).n).toBe(before + 1);
  });

  it("supersede outcomes: a picked predecessor is already ruled, a withdrawn pick is withdrawn [D-8]", async () => {
    const a = await fileOk();
    pickOk(a.decision_id, "day");
    const r = await svc.file(ask(env, { supersedes: a.decision_id, question: "again?" }), {});
    expect(r).toMatchObject({ ok: false, status: 409, error: 'already ruled: "Collapse per day"' });
    const w = await fileOk({ question: "w?", subject: "autarch/w" });
    svc.store.db.prepare("UPDATE decisions SET withdrawn_at = ? WHERE id = ?").run(env.now(), w.decision_id);
    expect(svc.pick(w.decision_id, "day", rev(w.decision_id), "pk-w", "mk", "home")).toMatchObject({ ok: false, status: 409, error: "withdrawn" });
    const r2 = await svc.file(ask(env, { supersedes: w.decision_id, question: "w2?" }), {});
    expect(r2).toMatchObject({ ok: false, status: 409, error: "withdrawn" });
  });

  it("replacement is exclusive: it is not also a mention of an equal open ask [E-7]", async () => {
    const c = await fileOk({ thread: "thr-c" });
    const a = await fileOk({ question: "a different first question?", subject: "autarch/other" });
    // B supersedes A and is semantically equal to thread C's open ask.
    const b = await fileOk({ supersedes: a.decision_id });
    expect(b.decision_id).not.toBe(c.decision_id);
    expect((b as { mentioned?: boolean }).mentioned).toBeUndefined();
    expect((svc.store.decision(b.decision_id) as { supersedes: string }).supersedes).toBe(a.decision_id);
    expect(svc.store.mentions(c.decision_id)).toEqual([]);
    expect(svc.pick(a.decision_id, "day", rev(a.decision_id), "pk", "mk", "home")).toMatchObject({ ok: false, status: 409 });
  });
});

describe("mentions", () => {
  it("a semantically equal ask from another thread is one entry with one mention; both threads see the ruling [D-7]", async () => {
    const a = await fileOk();
    const m = await svc.file(ask(env, { thread: "thr-b" }), {});
    expect(m).toMatchObject({ ok: true, decision_id: a.decision_id, mentioned: true });
    expect(svc.store.mentions(a.decision_id).map((x) => x.thread)).toEqual(["thr-b"]);
    expect((svc.store.db.prepare("SELECT COUNT(*) n FROM decisions").get() as { n: number }).n).toBe(1);
    expect(ids(svc.owed())).toEqual([a.decision_id]);
    pickOk(a.decision_id, "day");
    for (const t of ["thr-a", "thr-b"]) expect(svc.feed("Autarch", t).own.map((l) => l.decision)).toEqual([a.decision_id]);
    expect(svc.feed("Autarch", "thr-z").own).toEqual([]);
  });

  it("the same ask_key with a different instruction files a separate decision marked related", async () => {
    const a = await fileOk();
    const other = OPTIONS.map((o) => (o.id === "project" ? { ...o, instruction: "a different instruction" } : o));
    const b = await svc.file(ask(env, { thread: "thr-b", options: other }), {});
    expect(b).toMatchObject({ ok: true, status: 201, related: a.decision_id });
    expect((b as { decision_id: string }).decision_id).not.toBe(a.decision_id);
    expect((b as { mentioned?: boolean }).mentioned).toBeUndefined();
    expect(svc.store.events().some((e) => e.type === "related" && e.decision_id === (b as { decision_id: string }).decision_id)).toBe(true);
  });

  it("unrelated asks file separately, and the second thread never receives the first ruling [E-1]", async () => {
    const yn = [{ id: "yes", label: "Yes", kind: "needs-context" }, { id: "no", label: "No", kind: "needs-context" }];
    const a = await fileOk({ question: "Remove feature A?", subject: "autarch/remove: A", options: yn });
    const b = await svc.file(ask(env, { thread: "thr-b", question: "Remove feature B?", subject: "autarch/remove: B", options: yn }), {});
    expect(b).toMatchObject({ ok: true, status: 201 });
    expect((b as { decision_id: string }).decision_id).not.toBe(a.decision_id);
    const m1 = await fileOk({ kind: "machine", options: undefined, machine: { class: "ci", detail: "runner one down" }, question: "blocked", subject: "ci: blocked" });
    const m2 = await svc.file(ask(env, { thread: "thr-b", kind: "machine", options: undefined, machine: { class: "ci", detail: "runner two down" }, question: "blocked", subject: "ci: blocked" }), {});
    expect((m2 as { decision_id: string }).decision_id).not.toBe(m1.decision_id);
    pickOk(a.decision_id, "yes");
    expect(svc.feed("Autarch", "thr-b").own.map((l) => l.decision)).not.toContain(a.decision_id);
  });

  it("equal asks without a subject file separately; mention_of still joins them", async () => {
    const a = await fileOk({ subject: undefined });
    const b = await svc.file(ask(env, { thread: "thr-b", subject: undefined }), {});
    expect((b as { decision_id: string }).decision_id).not.toBe(a.decision_id);
    const c = await svc.file(ask(env, { thread: "thr-c", subject: undefined, mention_of: a.decision_id }), {});
    expect(c).toMatchObject({ ok: true, decision_id: a.decision_id, mentioned: true });
  });

  it("a mention_of naming a picked, withdrawn or unknown ask returns 409 with its state", async () => {
    const a = await fileOk({ subject: undefined });
    pickOk(a.decision_id, "day");
    const r = await svc.file(ask(env, { thread: "thr-b", mention_of: a.decision_id, question: "x?" }), {});
    expect(r).toMatchObject({ ok: false, status: 409, error: "picked" });
    const w = await fileOk({ subject: undefined, question: "w?" });
    svc.store.db.prepare("UPDATE decisions SET withdrawn_at = ? WHERE id = ?").run(env.now(), w.decision_id);
    expect(await svc.file(ask(env, { thread: "thr-b", mention_of: w.decision_id, question: "y?" }), {})).toMatchObject({ ok: false, status: 409, error: "withdrawn" });
    expect(await svc.file(ask(env, { thread: "thr-b", mention_of: "nope", question: "z?" }), {})).toMatchObject({ ok: false, status: 409, error: "unknown" });
  });

  it("a mention retry returns the same mention result after the ask is picked [D-7]", async () => {
    const a = await fileOk();
    const first = await svc.file(ask(env, { thread: "thr-b", request_id: "R" }), {});
    expect(first).toMatchObject({ mentioned: true, decision_id: a.decision_id });
    pickOk(a.decision_id, "day");
    const retry = await svc.file(ask(env, { thread: "thr-b", request_id: "R" }), {});
    expect(retry).toMatchObject({ ok: true, status: 200, mentioned: true, decision_id: a.decision_id });
    expect((svc.store.db.prepare("SELECT COUNT(*) n FROM decisions").get() as { n: number }).n).toBe(1);
  });
});

describe("reconcile", () => {
  it("ruling-file failure is local to its project and survives a restart [C-7]", async () => {
    const p = await fileOk({ question: "in P?", subject: "in-p" });
    const q = await svc.file(ask(env, { project: "Other", project_root: env.roots.Other, question: "in Q?", subject: "in-q" }), {});
    if (!q.ok) throw new Error(q.error);
    chmodSync(env.roots.Autarch!, 0o555);
    pickOk(p.decision_id, "day");
    pickOk(q.decision_id, "day");
    const po = svc.store.obligationsFor(p.decision_id)[0]!;
    expect(po).toMatchObject({ kind: "ruling-file", state: "pending" });
    expect(po.last_error).toBeTruthy();
    expect(Date.parse(po.next_try_at)).toBe(T0 + 30_000);
    expect(svc.failures().map((f) => f.decision_id)).toEqual([p.decision_id]);
    expect(svc.store.obligationsFor(q.decision_id)[0]!.state).toBe("done");
    expect(rulingFiles("Other")).toHaveLength(1);
    // restart
    const again = env.open();
    again.reconcileAll();
    expect(again.ready()).toBe(true);
    expect(again.store.obligationsFor(p.decision_id)[0]).toMatchObject({ state: "pending" });
    expect(again.store.obligationsFor(q.decision_id)[0]).toMatchObject({ state: "done" });
    // not yet due: backoff holds
    env.clock.t = T0 + 10_000;
    chmodSync(env.roots.Autarch!, 0o755);
    again.reconcileAll();
    expect(again.store.obligationsFor(p.decision_id)[0]!.state).toBe("pending");
    // due: succeeds
    env.clock.t = T0 + 31_000;
    again.reconcileAll();
    expect(again.store.obligationsFor(p.decision_id)[0]!.state).toBe("done");
    expect(rulingFiles()).toHaveLength(1);
  });

  it("backoff doubles from 30 s up to one hour", async () => {
    const p = await fileOk();
    chmodSync(env.roots.Autarch!, 0o555);
    pickOk(p.decision_id, "day");
    const delays: number[] = [];
    for (let i = 0; i < 9; i++) {
      const o = svc.store.obligationsFor(p.decision_id)[0]!;
      delays.push(Date.parse(o.next_try_at) - env.clock.t);
      env.clock.t = Date.parse(o.next_try_at);
      svc.reconcileAll();
    }
    expect(delays).toEqual([30_000, 60_000, 120_000, 240_000, 480_000, 960_000, 1_920_000, 3_600_000, 3_600_000]);
  });

  it("writing twice gives one file with the same bytes", async () => {
    const p = await fileOk();
    pickOk(p.decision_id, "day");
    const before = readFileSync(join(env.roots.Autarch!, "docs", "decisions", rulingFiles()[0]!));
    svc.reconcile(p.decision_id);
    expect(rulingFiles()).toHaveLength(1);
    expect(readFileSync(join(env.roots.Autarch!, "docs", "decisions", rulingFiles()[0]!)).equals(before)).toBe(true);
  });

  it("a replaced root is refused and reported on the obligation", async () => {
    const p = await fileOk();
    const root = env.roots.Autarch!;
    const { renameSync } = await import("node:fs");
    renameSync(root, `${root}-moved`);
    mkdirSync(root);
    pickOk(p.decision_id, "day");
    const o = svc.store.obligationsFor(p.decision_id)[0]!;
    expect(o.state).toBe("pending");
    expect(o.last_error).toContain("project root changed since filing");
    expect(existsSync(join(root, "docs"))).toBe(false);
    void writeFileSync;
  });
});

describe("dismiss", () => {
  it("an undeliverable wake is dismissed; a new service over the same file shows nothing [C-4]", async () => {
    const a = await fileOk();
    pickOk(a.decision_id, "project");
    const wake = svc.store.obligationsFor(a.decision_id).find((o) => o.kind === "wake")!;
    expect(svc.store.transition(wake.id, "pending", "undeliverable", 0)).toEqual({ ok: true });
    expect(ids(svc.undeliverable().map((u) => ({ id: u.id })))).toEqual([wake.id]);
    expect(svc.wakes()).toEqual([]);
    expect(svc.dismiss(a.decision_id, wake.id)).toEqual({ ok: true });
    expect(svc.store.obligation(wake.id)!.state).toBe("dismissed");
    const again = env.open();
    expect(again.wakes()).toEqual([]);
    expect(again.undeliverable()).toEqual([]);
    // terminal, and only undeliverable rows can be dismissed
    expect(svc.dismiss(a.decision_id, wake.id)).toMatchObject({ ok: false });
    expect(svc.dismiss("other", wake.id)).toMatchObject({ ok: false });
  });
});

describe("effective rulings [D-4]", () => {
  it("an override voids the old line and the replacement's pick supersedes it at once", async () => {
    const a = await fileOk();
    await svc.file(ask(env, { thread: "thr-m" }), {}); // thr-m mentions A
    pickOk(a.decision_id, "project", "pk-v", "vizier");
    const aBody = parseAsk(JSON.parse((svc.store.decision(a.decision_id) as { body_json: string }).body_json));
    const bAsk = { ...aBody, request_id: `override:${a.decision_id}`, supersedes: a.decision_id };
    const bParsed = parseAsk(bAsk);
    const res = svc.store.insertReplacement(
      {
        id: "dec-B",
        request_id: bParsed.request_id!,
        identity: identity(bParsed),
        revision: revision(bParsed),
        semantic_key: semanticKey(bParsed),
        subject: bParsed.subject!,
        kind: "decide",
        project: "Autarch",
        project_root: env.roots.Autarch!,
        asker: "thread",
        thread: "thr-a",
        body_json: normalizedJson(bParsed),
        supersedes: a.decision_id,
        delegable: false,
      },
      "override",
    );
    expect(res).toEqual({ ok: true, decision_id: "dec-B" });
    const f = svc.feed("Autarch", "thr-a");
    expect(f.project.map((l) => l.label)).toEqual([]);
    expect(f.own).toHaveLength(1);
    expect(f.own[0]).toMatchObject({ decision: a.decision_id, status: "void", tip: "dec-B" });
    expect(f.text).toContain(`void ${a.decision_id}: overridden, awaiting mk (dec-B)`);
    // mk picks the replacement while the old wake is still pending
    env.clock.t += 60_000;
    pickOk("dec-B", "day");
    const g = svc.feed("Autarch", "thr-a");
    expect(g.own.map((l) => [l.decision, l.status, l.label, l.supersedes])).toEqual([["dec-B", "effective", "Collapse per day", a.decision_id]]);
    expect(g.text).toContain(`, supersedes ${a.decision_id}`);
    expect(g.text).not.toContain("void");
    expect(svc.feed("Autarch", "thr-z").project.map((l) => l.decision)).toEqual(["dec-B"]);
    expect(svc.feed("Autarch", "thr-m").own.map((l) => l.decision)).toEqual(["dec-B"]);
    // the replacement's wake payload starts with the supersedes line (an instruction pick)
    const b2 = await fileOk({ question: "x?", subject: "autarch/x" });
    void b2;
  });

  it("a replacement's instruction wake starts with the supersedes line", async () => {
    const a = await fileOk();
    pickOk(a.decision_id, "day", "pk-v", "vizier");
    const aBody = parseAsk(JSON.parse((svc.store.decision(a.decision_id) as { body_json: string }).body_json));
    const bParsed = parseAsk({ ...aBody, request_id: `override:${a.decision_id}`, supersedes: a.decision_id });
    svc.store.insertReplacement(
      { id: "dec-B", request_id: bParsed.request_id!, identity: identity(bParsed), revision: revision(bParsed), semantic_key: semanticKey(bParsed), subject: bParsed.subject!, kind: "decide", project: "Autarch", project_root: env.roots.Autarch!, asker: "thread", thread: "thr-a", body_json: normalizedJson(bParsed), supersedes: a.decision_id, delegable: false },
      "override",
    );
    pickOk("dec-B", "project");
    const w = svc.wakes().find((x) => x.op?.startsWith("wake:dec-B"))!;
    expect(w.payload!.startsWith(`This supersedes the vizier's ruling on ${a.decision_id}.`)).toBe(true);
    expect(w.payload).toContain(OPTIONS[0]!.instruction);
  });
});

describe("recent, owed and stats", () => {
  it("recent lists the project's decisions newest first with their status", async () => {
    const a = await fileOk();
    env.clock.t += 1000;
    const b = await fileOk({ question: "b?", subject: "autarch/b" });
    pickOk(a.decision_id, "day");
    const r = svc.recent("Autarch", 10);
    expect(r.map((x) => [x.id, x.status])).toEqual([[b.decision_id, "open"], [a.decision_id, "picked"]]);
    expect(svc.recent("Autarch", 1)).toHaveLength(1);
    expect(svc.recent("Other", 10)).toEqual([]);
  });

  it("stats over fixed timestamps", async () => {
    // week of 2026-09-21 (ISO week 39): T0 is Saturday 2026-09-26
    const a = await fileOk({ question: "a?", subject: "s-a" });
    const b = await fileOk({ question: "b?", subject: "s-b" });
    const c = await fileOk({ question: "c?", subject: "s-c" });
    env.clock.t = T0 + 10_000;
    pickOk(a.decision_id, "day"); // mk, 10 s
    env.clock.t = T0 + 30_000;
    pickOk(b.decision_id, "day", "pk-b", "mk"); // mk, 30 s
    svc.pick(c.decision_id, "day", rev(c.decision_id), "pk-c", "vizier", "cli", "cheap and reversible"); // vizier
    env.clock.t = T0 + 90_000;
    const cAsk = parseAsk(JSON.parse((svc.store.decision(c.decision_id) as { body_json: string }).body_json));
    const o = parseAsk({ ...cAsk, request_id: "override:c", supersedes: c.decision_id });
    svc.store.insertReplacement(
      { id: "dec-O", request_id: "override:c", identity: identity(o), revision: revision(o), semantic_key: semanticKey(o), subject: o.subject!, kind: "decide", project: "Autarch", project_root: env.roots.Autarch!, asker: "thread", thread: "thr-a", body_json: normalizedJson(o), supersedes: c.decision_id, delegable: false },
      "override",
    );
    // a filing next week
    env.clock.t = Date.parse("2026-09-29T09:00:00.000Z");
    await fileOk({ question: "d?", subject: "s-d" });
    const s = svc.stats("2026-09-01T00:00:00.000Z");
    expect(s.filed_per_week).toEqual({ "2026-W39": 3, "2026-W40": 1 });
    expect(s.picks_by).toEqual({ mk: 2, vizier: 1 });
    expect(s.picks_by_surface).toEqual({ home: 2, cli: 1 });
    expect(s.mk_latency_ms).toEqual({ n: 2, median: 20_000, p90: 30_000 });
    expect(s.delegated).toBe(1);
    expect(s.overrides).toBe(1);
    expect(s.override_rate).toBe(1);
    expect(s.median_override_ms).toBe(60_000);
    expect(svc.stats("2026-09-28T00:00:00.000Z").filed_per_week).toEqual({ "2026-W40": 1 });
  });
});

describe("Task 2.4 store and fingerprint additions", () => {
  it("insertDecision writes the card link columns at INSERT, defaulting to a home row", async () => {
    const id = (await fileOk()).decision_id;
    expect(svc.store.db.prepare("SELECT source, task_id, generation, tasks_project_id, card_fp FROM decisions WHERE id = ?").get(id)).toEqual({ source: "home", task_id: null, generation: null, tasks_project_id: null, card_fp: null });
  });

  it("withdrawDecision withdraws an unpicked row once and refuses a picked one", async () => {
    const a = (await fileOk({ ask_key: "wd-a", subject: "wd-a" })).decision_id;
    expect(svc.store.withdrawDecision(a, "test")).toBe(true);
    expect(svc.store.withdrawDecision(a, "test")).toBe(false);
    expect(svc.store.recordPick(pickOf(a, { revision: rev(a) }))).toMatchObject({ ok: false, reason: "withdrawn" });
  });
});

// ---- Task 2.5: pick write-back on a card generation ----
import { afterEach as afterEach25, describe as describe25, expect as expect25, it as it25 } from "vitest";
import { cleanupEnvs as cleanup25, opened as opened25, rig as rig25 } from "./card-rig.js";

afterEach25(cleanup25);
const notices = (r: any, id: string) => r.svc.store.obligationsFor(id).filter((o: any) => o.kind === "notice").map((o: any) => o.recipient).sort();

describe25("card pick: wake, notices and write-backs in one transaction", () => {
  it25("inserts the wake, one notice per Blocks thread except the asker, and the card_writes rows", async () => {
    const r = rig25();
    const { g1 } = await opened25(r, { blocks: "thread:thr_a thread:thr_b thread:thr_c bead:x thread:thr_b" });
    expect25(r.mkPick(g1.id)).toMatchObject({ ok: true });
    expect25(r.svc.store.obligationsFor(g1.id).filter((o) => o.kind === "wake")).toHaveLength(1);
    expect25(notices(r, g1.id)).toEqual(["thr_b", "thr_c"]);
    expect25(r.db.prepare("SELECT kind FROM card_writes WHERE decision_id = ? ORDER BY kind").all(g1.id)).toEqual([{ kind: "comment" }, { kind: "unlabel" }]);
    expect25(r.cardRow(g1.task_id).state).toBe("ruled");
  });

  it25("a pull:mycroft card gets no wake but still gets its write-backs", async () => {
    const r = rig25();
    const { g1 } = await opened25(r, { pull: true, thread: null });
    expect25(r.mkPick(g1.id)).toMatchObject({ ok: true });
    expect25(r.svc.store.obligationsFor(g1.id).filter((o) => o.kind === "wake")).toHaveLength(0);
    expect25(r.db.prepare("SELECT COUNT(*) AS n FROM card_writes WHERE decision_id = ?").get(g1.id)).toEqual({ n: 2 });
  });

  it25("Blocks edited before the pick: notices go to the g2 snapshot's threads", async () => {
    const r = rig25();
    const { t, g1 } = await opened25(r, { blocks: "thread:thr_old" });
    r.edit(t, { description: r.desc({ key: String(r.cardRow(t.id).request_key), blocks: "thread:thr_new" }) });
    await r.poll();
    const g2 = r.gens(t.id)[1];
    expect25(g2.id).not.toBe(g1.id);
    r.mkPick(g2.id);
    expect25(notices(r, g2.id)).toEqual(["thr_new"]);
  });

  it25("Blocks edited after the pick: the notices stay with the picked generation's snapshot", async () => {
    const r = rig25();
    const { t, g1 } = await opened25(r, { blocks: "thread:thr_old" });
    r.mkPick(g1.id);
    r.edit(t, { description: r.desc({ key: String(r.cardRow(t.id).request_key), blocks: "thread:thr_new" }) });
    await r.poll();
    expect25(r.gens(t.id)).toHaveLength(1);
    expect25(notices(r, g1.id)).toEqual(["thr_old"]);
  });

  it25("backfill on start repairs a picked card that lacks its writes, with the final payload shape", async () => {
    const r = rig25();
    const { g1 } = await opened25(r);
    r.pick(g1.id); // a store-level pick inserts no card_writes
    expect25(r.db.prepare("SELECT COUNT(*) AS n FROM card_writes").get()).toEqual({ n: 0 });
    r.restart();
    const rows = r.db.prepare("SELECT kind, payload FROM card_writes ORDER BY kind").all() as any[];
    expect25(rows.map((x) => x.kind)).toEqual(["comment", "unlabel"]);
    expect25(JSON.parse(rows[0].payload)).toMatchObject({ option_id: "a", by: "mk", generation: 1 });
  });
});
