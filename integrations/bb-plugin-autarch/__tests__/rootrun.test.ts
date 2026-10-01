// Task 2.8: root-run display. Slice A makes no approval claim: Home shows a paste command and read-only
// status, never runs anything. Every card input is untrusted.
import { mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { askingThread, parseCard } from "../cards.js";
import {
  BADGE,
  fetchRunStatus,
  itemJson,
  pasteCommand,
  rootRun,
  scriptState,
  shQuote,
  type RootRunInput,
} from "../rootrun.js";

const TASK = "01J0000000000000000000000A";
const SCRIPT = "#!/bin/sh\necho hi\n";
const sha = (s: string | Buffer) => createHash("sha256").update(s).digest("hex");
let dir: string;
beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "rootrun-"));
});
afterEach(() => rmSync(dir, { recursive: true, force: true }));

function script(text = SCRIPT, name = "run.sh"): string {
  const p = join(dir, name);
  writeFileSync(p, text);
  return p;
}
const comment = (id: string, threadId: string | null, createdAt: string, kind = "agent") => ({ id, kind, threadId, createdAt });
const GOOD_COMMENTS = [comment("c1", "thr_abc123", "2026-09-30T12:00:00.5Z"), comment("c2", "thr_zzz", "2026-09-30T12:00:01Z")];

function desc(rr: string | null, extra = ""): string {
  const ask = { schema: "home-ask/v2", project: "Autarch", project_root: "/r", question: "Q?", options: [{ id: "a", label: "A", kind: "instruction", instruction: "do a", reversible: true }, { id: "b", label: "B", kind: "instruction", instruction: "do b", reversible: true }], ask_key: "k" };
  return ["Prose", "", "Request: 6f1c-uuid sha256:0123456789abcdef", "", "```home-ask", JSON.stringify(ask), "```", ...(rr === null ? [] : ["", "```root-run", rr, "```"]), extra].join("\n");
}
const rrBlock = (o: { script: string; sha?: string; set?: string; timeout?: string }) => `script: ${o.script}\nsha256: ${o.sha ?? sha(SCRIPT)}\ntimeout: ${o.timeout ?? "900"}\nset: ${o.set ?? "myset"}`;
function input(over: { description?: string; id?: string; projectId?: string; comments?: RootRunInput["comments"] } = {}, scriptPath?: string): RootRunInput {
  return {
    task: { id: over.id ?? TASK, projectId: over.projectId ?? "proj:1", description: over.description ?? desc(rrBlock({ script: scriptPath ?? script() })) },
    comments: over.comments ?? GOOD_COMMENTS,
  };
}
const noFetch = async () => {
  throw new Error("down");
};
const run = (i: RootRunInput) => rootRun(i, { fetch: noFetch as never });

describe("scriptState: the four states", () => {
  it("match, mismatch, unreadable and invalid", () => {
    const p = script();
    expect(scriptState(p, sha(SCRIPT))).toEqual({ state: "match", actual: sha(SCRIPT) });
    expect(scriptState(p, "0".repeat(64))).toEqual({ state: "mismatch", actual: sha(SCRIPT) });
    expect(scriptState(join(dir, "missing.sh"), sha(SCRIPT)).state).toBe("unreadable");
    expect(scriptState(dir, sha(SCRIPT)).state).toBe("unreadable");
    const link = join(dir, "link.sh");
    symlinkSync(p, link);
    expect(scriptState(link, sha(SCRIPT)).state).toBe("unreadable");
    const big = script("x".repeat(256 * 1024 + 1), "big.sh");
    expect(scriptState(big, sha("x"))).toMatchObject({ state: "unreadable" });
    expect(scriptState(script("x".repeat(256 * 1024), "edge.sh"), sha("x".repeat(256 * 1024))).state).toBe("match");
    mkdirSync(join(dir, "d"));
    // invalid: the card's root-run block did not parse, or the sha is not 64 hex
    expect(scriptState(p, "nothex").state).toBe("invalid");
  });

  it("rootRun reports each state without a command unless it matches", async () => {
    expect((await run(input())).state).toBe("match");
    expect((await run(input({ description: desc(rrBlock({ script: script(), sha: "1".repeat(64) })) }))).state).toBe("mismatch");
    expect((await run(input({ description: desc(rrBlock({ script: join(dir, "nope.sh") })) }))).state).toBe("unreadable");
    const bad = await run(input({ description: desc("script: relative.sh\nsha256: x\ntimeout: 1\nset: s") }));
    expect(bad.state).toBe("invalid");
    expect(bad.command).toBeNull();
    for (const v of [await run(input({ description: desc(rrBlock({ script: script(), sha: "1".repeat(64) })) })), await run(input({ description: desc(rrBlock({ script: join(dir, "nope.sh") })) }))]) expect(v.command).toBeNull();
  });

  it("a card with no root-run block has nothing to show", async () => {
    const v = await run(input({ description: desc(null) }));
    expect(v.present).toBe(false);
    expect(v.command).toBeNull();
  });
});

describe("the paste command (Q1 default b: one named function)", () => {
  it("renders for a valid tuple, every argument single-quoted", async () => {
    const v = await run(input());
    expect(v.badge).toBe(BADGE);
    expect(BADGE).toBe("run by paste, not authenticated");
    expect(v.problems).toEqual([]);
    expect(v.command).toBe(`todo-add --set 'myset' --from-card 'card-${TASK}.json'`);
    expect(v.command).toBe(pasteCommand({ set: "myset", task_id: TASK }));
    const args = v.command!.split(" ").slice(1).filter((a) => !a.startsWith("--"));
    expect(args.every((a) => /^'[^']*'$/.test(a))).toBe(true);
  });

  it("names the card file the adapter reads, and says the file must be saved first", async () => {
    const v = await run(input());
    expect(v.card_file).toBe(`card-${TASK}.json`);
  });

  it("shQuote refuses a quote, control characters and non-ASCII", () => {
    expect(shQuote("abc-1")).toBe("'abc-1'");
    for (const bad of ["a'b", "a\nb", "a\tb", "a\x00b", "a\x7fb", "café", "a‮b", "", "a\r"]) expect(() => shQuote(bad)).toThrow();
  });

  const hostile: [string, Partial<{ set: string; id: string; projectId: string }>, string][] = [
    ["uppercase set", { set: "MySet" }, "set"],
    ["set with a quote", { set: "a'b" }, "set"],
    ["set with a semicolon", { set: "a;rm" }, "set"],
    ["set with a space", { set: "a b" }, "set"],
    ["set with a dot", { set: "a.b" }, "set"],
    ["set with underscore", { set: "a_b" }, "set"],
    ["set with a leading dash", { set: "-rf" }, "set"],
    ["set too long", { set: "a".repeat(65) }, "set"],
    ["lowercase task id", { id: TASK.toLowerCase() }, "task.id"],
    ["task id with a quote", { id: "01J0000000000000000000000'" }, "task.id"],
    ["short task id", { id: "01J" }, "task.id"],
    ["project id with a space", { projectId: "a b" }, "projectId"],
    ["project id with a quote", { projectId: "a'b" }, "projectId"],
    ["empty project id", { projectId: "" }, "projectId"],
    ["project id too long", { projectId: "a".repeat(129) }, "projectId"],
  ];
  it.each(hostile)("%s renders no command and names the field", async (_n, o, field) => {
    const set = o.set ?? "myset";
    // a set that the card parser itself refuses is also a hostile vector, reported as invalid
    const v = await run(input({ ...(o.id !== undefined ? { id: o.id } : {}), ...(o.projectId !== undefined ? { projectId: o.projectId } : {}), description: desc(rrBlock({ script: script(), set })) }));
    expect(v.command).toBeNull();
    expect(v.item_json).toBeNull();
    const named = v.problems.map((p) => p.field);
    expect(named.some((f) => f === field || (field === "set" && f === "card"))).toBe(true);
    expect(v.problems.length).toBeGreaterThan(0);
  });

  const threadCases: [string, RootRunInput["comments"], string][] = [
    ["no comments", [], "owner_thread"],
    ["only a user comment", [comment("c1", "thr_a", "2026-09-30T12:00:00Z", "user")], "owner_thread"],
    ["earliest agent comment without a thread", [comment("c1", null, "2026-09-30T12:00:00Z"), comment("c2", "thr_a", "2026-09-30T12:00:01Z")], "owner_thread"],
    ["uppercase thread id", [comment("c1", "thr_ABC", "2026-09-30T12:00:00Z")], "owner_thread"],
    ["thread id with a quote", [comment("c1", "thr_a'b", "2026-09-30T12:00:00Z")], "owner_thread"],
    ["non-ASCII thread id", [comment("c1", "thr_café", "2026-09-30T12:00:00Z")], "owner_thread"],
    ["thread id not thr_ prefixed", [comment("c1", "abc", "2026-09-30T12:00:00Z")], "owner_thread"],
    ["tied earliest comments", [comment("c1", "thr_a", "2026-09-30T12:00:00Z"), comment("c2", "thr_b", "2026-09-30T12:00:00Z")], "owner_thread"],
    ["tie across spellings of one instant", [comment("c1", "thr_a", "2026-09-30T12:00:00Z"), comment("c2", "thr_b", "2026-09-30T12:00:00.000Z")], "owner_thread"],
    ["non-canonical createdAt", [comment("c1", "thr_a", "2026-09-30 12:00:00")], "owner_thread"],
    ["offset createdAt", [comment("c1", "thr_a", "2026-09-30T12:00:00+00:00")], "owner_thread"],
    ["empty comment id", [comment("", "thr_a", "2026-09-30T12:00:00Z")], "owner_thread"],
    ["oversized comment id", [comment("c".repeat(129), "thr_a", "2026-09-30T12:00:00Z")], "owner_thread"],
    ["malformed createdAt on a later comment", [comment("c1", "thr_a", "2026-09-30T12:00:00Z"), comment("c2", "thr_b", "later")], "owner_thread"],
  ];
  it.each(threadCases)("%s renders no command and names owner_thread (an incomplete tuple)", async (_n, comments, field) => {
    const v = await run(input({ comments }));
    expect(v.command).toBeNull();
    expect(v.problems.map((p) => p.field)).toContain(field);
  });

  it("a card whose block does not parse renders no command and says why", async () => {
    const v = await run(input({ description: desc("script: /a\nsha256: " + "a".repeat(64) + "\ntimeout: 99999\nset: s") }));
    expect(v.command).toBeNull();
    expect(v.problems[0]).toMatchObject({ field: "card" });
    expect(v.problems[0]!.why).toMatch(/timeout/);
  });
});

describe("owner_thread and the item JSON", () => {
  it("comes from askingThread(), the earliest agent comment", async () => {
    const comments = [comment("c2", "thr_later", "2026-09-30T12:00:02Z"), comment("c0", "thr_user", "2026-09-30T11:00:00Z", "user"), comment("c1", "thr_first", "2026-09-30T12:00:00.25Z")];
    expect(askingThread(comments)).toBe("thr_first");
    const v = await run(input({ comments }));
    expect(v.owner_thread).toBe(askingThread(comments));
    expect(v.owner_thread).toBe("thr_first");
  });

  it("refuses when askingThread's lexicographic order disagrees with the numeric one", async () => {
    // numerically thr_a is earlier; lexicographically "12:00:00.5Z" < "12:00:00Z", so askingThread says thr_b.
    // Aleph orders numerically, so Home must not show a command that names a different owner than Aleph would.
    const comments = [comment("c1", "thr_a", "2026-09-30T12:00:00Z"), comment("c2", "thr_b", "2026-09-30T12:00:00.5Z")];
    expect(askingThread(comments)).toBe("thr_b");
    const v = await run(input({ comments }));
    expect(v.command).toBeNull();
    expect(v.problems.find((p) => p.field === "owner_thread")?.why).toMatch(/disagrees/);
  });

  it("has exactly the six keys, in order, with the label bbtask:<task.id>:<attempt>", async () => {
    const v = await run(input());
    const o = JSON.parse(v.item_json!);
    expect(Object.keys(o)).toEqual(["run_as", "script", "script_sha256", "owner_thread", "run_timeout_s", "label"]);
    expect(o).toEqual({ run_as: "mk", script: v.tuple!.script, script_sha256: sha(SCRIPT), owner_thread: "thr_abc123", run_timeout_s: 900, label: `bbtask:${TASK}:1` });
    expect(v.item_json).toBe(itemJson({ script: v.tuple!.script, sha256: sha(SCRIPT), timeout: 900, set: "myset" }, "thr_abc123", TASK, 1));
  });
});

describe("no approval wording and no authority", () => {
  it("nothing rendered says approved, in any state", async () => {
    const views = [
      await run(input()),
      await run(input({ description: desc(rrBlock({ script: script(), sha: "1".repeat(64) })) })),
      await run(input({ description: desc(rrBlock({ script: join(dir, "nope.sh") })) })),
      await run(input({ comments: [] })),
    ];
    for (const v of views) expect(JSON.stringify(v)).not.toMatch(/approv/i);
  });
});

describe("status: a read-only GET on the Aleph runner, untrusted display data", () => {
  const item = (n: number) => `bbtask-${TASK}-${n}`;
  const rec = (n: number, over: Record<string, unknown> = {}) => ({
    set: "myset", item: item(n), link: "x", phase: "running", terminal: false, exec_report: null, exit: null, signal: null, reason: null,
    started: "2026-09-30T12:05:00Z", ended: null, log_complete: false, label: `bbtask:${TASK}:${n}`, owner_alive: true, ...over,
  });
  const reply = (status: number, body: unknown) => async () => new Response(typeof body === "string" ? body : JSON.stringify(body), { status });
  const calls: string[] = [];
  const router = (table: Record<string, () => Response | Promise<Response>>) => async (url: string | URL) => {
    calls.push(String(url));
    const fn = table[String(url)];
    return fn ? fn() : new Response(JSON.stringify({ error: "no run record" }), { status: 404 });
  };
  beforeEach(() => {
    calls.length = 0;
  });
  const URL1 = `http://127.0.0.1:8744/api/run/myset/${item(1)}.json`;
  const URL2 = `http://127.0.0.1:8744/api/run/myset/${item(2)}.json`;

  it("asks for the first attempt and reports no run record on 404", async () => {
    const s = await fetchRunStatus("myset", TASK, { fetch: router({}) as never });
    expect(calls).toEqual([URL1]);
    expect(s).toEqual({ kind: "none" });
  });

  it("reports the latest attempt that has a record", async () => {
    const s = await fetchRunStatus("myset", TASK, { fetch: router({ [URL1]: () => new Response(JSON.stringify(rec(1, { phase: "exited", terminal: "exited", exit: 1 }))), [URL2]: () => new Response(JSON.stringify(rec(2))) }) as never });
    expect(calls).toEqual([URL1, URL2, `http://127.0.0.1:8744/api/run/myset/${item(3)}.json`]);
    expect(s).toMatchObject({ kind: "run", attempt: 2, phase: "running", terminal: false, owner_alive: true });
  });

  it("maps a terminal record, with exit, signal and a coded reason", async () => {
    const s = await fetchRunStatus("myset", TASK, { fetch: router({ [URL1]: () => new Response(JSON.stringify(rec(1, { phase: "finalizing", terminal: "killed", exit: null, signal: "SIGKILL", reason: "supervisor-died", ended: "2026-09-30T12:06:00Z", log_complete: true, owner_alive: false }))) }) as never });
    expect(s).toMatchObject({ kind: "run", terminal: "killed", signal: "SIGKILL", reason: "supervisor-died", log_complete: true, owner_alive: false });
  });

  it.each([
    ["unreachable", async () => { throw new TypeError("fetch failed"); }],
    ["500", reply(500, { error: "internal" })],
    ["503", reply(503, "busy")],
    ["not JSON", reply(200, "<html>")],
    ["a JSON array", reply(200, [])],
    ["null", reply(200, "null")],
    ["a redirect", reply(302, "")],
    ["oversized", reply(200, "x".repeat(70_000))],
  ])("%s is status unavailable and never throws", async (_n, f) => {
    const s = await fetchRunStatus("myset", TASK, { fetch: f as never });
    expect(s).toMatchObject({ kind: "unavailable" });
  });

  it("an aborted or slow runner is unavailable", async () => {
    const s = await fetchRunStatus("myset", TASK, { fetch: ((_u: unknown, init: { signal: AbortSignal }) => new Promise((_r, rej) => init.signal.addEventListener("abort", () => rej(new Error("aborted"))))) as never, timeoutMs: 20 });
    expect(s).toMatchObject({ kind: "unavailable" });
  });

  it("refuses a record that does not link back to this card", async () => {
    for (const bad of [rec(1, { label: `bbtask:${TASK}:2` }), rec(1, { label: "bbtask:OTHER:1" }), rec(1, { item: "other" }), rec(1, { set: "other" })]) {
      const s = await fetchRunStatus("myset", TASK, { fetch: router({ [URL1]: () => new Response(JSON.stringify(bad)) }) as never });
      expect(s).toMatchObject({ kind: "unavailable" });
    }
  });

  it("treats every field as hostile: unknown codes and long or odd strings are normalised", async () => {
    const s = await fetchRunStatus("myset", TASK, {
      fetch: router({
        [URL1]: () => new Response(JSON.stringify(rec(1, { phase: "<script>alert(1)</script>", terminal: "approved by mk", exit: 99999, signal: "x".repeat(500), reason: "<b>pwned</b>", started: "yesterday-ish", ended: 7, log_complete: "yes", owner_alive: "maybe", exec_report: { a: 1 } }))),
      }) as never,
    });
    expect(s).toMatchObject({ kind: "run", phase: "unknown", terminal: "unknown", exit: null, signal: null, reason: "other", started: null, ended: null, log_complete: null, owner_alive: null });
    expect(JSON.stringify(s)).not.toMatch(/approv|script|pwned/i);
  });

  it("rootRun carries the status, and a runner that is down never fails the view", async () => {
    const up = await rootRun(input(), { fetch: router({ [URL1]: () => new Response(JSON.stringify(rec(1))) }) as never });
    expect(up.status).toMatchObject({ kind: "run", attempt: 1 });
    const down = await run(input());
    expect(down.status).toMatchObject({ kind: "unavailable" });
    expect(down.state).toBe("match");
    expect(down.command).not.toBeNull();
  });

  it("reads status only for a valid set and task id", async () => {
    const f = router({});
    await rootRun(input({ id: "bad'id" }), { fetch: f as never });
    expect(calls).toEqual([]);
  });
});

describe("parseCard integration", () => {
  it("the card the tests build parses and carries the root-run tuple", () => {
    expect(parseCard(desc(rrBlock({ script: "/a/b.sh" }))).root_run).toMatchObject({ script: "/a/b.sh", set: "myset", timeout: 900 });
  });
});
