import { describe, expect, it } from "vitest";
import { parseCard } from "../cards";
import { parseMove, scriptCommands } from "../moves";

const SHA = "a".repeat(64);
const SHA2 = "b".repeat(64);
const mv = (o: unknown) => JSON.stringify({ schema: "home-move/v1", ...(o as object) });
const script = (s: object) => mv({ kind: "script", script: { path: "/tmp/x/run.sh", sha256: SHA, ...s } });

describe("parseMove", () => {
  it("parses each kind", () => {
    expect(parseMove(script({}))).toEqual({ kind: "script", script: { path: "/tmp/x/run.sh", sha256: SHA, args: [], recover: null } });
    expect(parseMove(mv({ kind: "pr", pr: { url: "https://github.com/o/r/pull/12" } }))).toEqual({ kind: "pr", url: "https://github.com/o/r/pull/12" });
    expect(parseMove(mv({ kind: "read", read: { url: "https://github.com/o/r" } }))).toMatchObject({ kind: "read" });
    expect(parseMove(mv({ kind: "context", context: { need: "which host?" } }))).toEqual({ kind: "context", need: "which host?" });
    expect(parseMove(mv({ kind: "context", context: { need: "x".repeat(8000) } }))).toMatchObject({ kind: "context" });
    expect(parseMove(mv({ kind: "context", context: { need: "😀".repeat(8000) } }))).toMatchObject({ kind: "context" });
    expect(() => parseMove(mv({ kind: "context", context: { need: "😀".repeat(8001) } }))).toThrow(/1-8000 characters/);
  });

  it.each([
    ["not json", "{"],
    ["wrong schema", JSON.stringify({ schema: "x", kind: "pr" })],
    ["unknown kind", mv({ kind: "run" })],
    ["member missing", mv({ kind: "pr" })],
    ["other member present", mv({ kind: "pr", pr: { url: "https://github.com/o/r/pull/1" }, script: {} })],
    ["relative path", script({ path: "run.sh" })],
    ["dotdot", script({ path: "/tmp/../etc/x" })],
    ["quote in path", script({ path: "/tmp/a'b" })],
    ["space in path", script({ path: "/tmp/a b" })],
    ["short sha", script({ sha256: "abc" })],
    ["uppercase sha", script({ sha256: "A".repeat(64) })],
    ["bad arg", script({ args: ["a b"] })],
    ["arg shell metachar", script({ args: ["$(id)"] })],
    ["too many args", script({ args: Array(9).fill("a") })],
    ["free-text recover", script({ recover: "rm -rf /" })],
    ["recover with args", script({ recover: { path: "/tmp/r.sh", sha256: SHA2, args: ["a"] } })],
    ["extra script field", script({ check: "evil" })],
    ["pr url with query", mv({ kind: "pr", pr: { url: "https://github.com/o/r/pull/1?x=1" } })],
    ["pr url other host", mv({ kind: "pr", pr: { url: "https://evil.example/o/r/pull/1" } })],
    ["pr number zero", mv({ kind: "pr", pr: { url: "https://github.com/o/r/pull/0" } })],
    ["read http", mv({ kind: "read", read: { url: "http://github.com/o/r" } })],
    ["read javascript", mv({ kind: "read", read: { url: "javascript:alert(1)" } })],
    ["read other host", mv({ kind: "read", read: { url: "https://evil.example/" } })],
    ["read userinfo", mv({ kind: "read", read: { url: "https://u:p@github.com/" } })],
    ["context empty", mv({ kind: "context", context: { need: "  " } })],
    ["context too long", mv({ kind: "context", context: { need: "x".repeat(8001) } })],
  ])("refuses %s", (_n, body) => {
    expect(() => parseMove(body)).toThrow();
  });
});

describe("scriptCommands", () => {
  it("derives sha256sum, check, run and recover in order, one line each", () => {
    const m = parseMove(script({ args: ["--set", "a=1"], recover: { path: "/tmp/x/undo.sh", sha256: SHA2 } }));
    if (m.kind !== "script") throw new Error("kind");
    const cmds = scriptCommands(m.script);
    expect(cmds.map((c) => c.label)).toEqual(["sha256sum", "check", "run", "recover"]);
    expect(cmds.map((c) => c.command)).toEqual([
      "sha256sum '/tmp/x/run.sh'",
      "bash '/tmp/x/run.sh' --check",
      "bash '/tmp/x/run.sh' '--set' 'a=1'",
      "bash '/tmp/x/undo.sh'",
    ]);
    expect(cmds[0]!.expectedSha).toBe(SHA);
    expect(cmds[3]!.expectedSha).toBe(SHA2);
    for (const c of cmds) {
      expect(c.command).toBe(c.command.trim());
      expect(c.command).not.toMatch(/[\n\r`]/);
    }
  });
});

describe("home-move block in a card", () => {
  const base = (extra: string) =>
    ["Run it.", "Request: k1 sha256:0123456789abcdef", "```home-ask", JSON.stringify({ schema: "home-ask/v2", project: "p", project_root: "/r", question: "q?", options: [{ id: "a", label: "A", kind: "instruction", instruction: "Do A.", reversible: true }, { id: "b", label: "B", kind: "instruction", instruction: "Do B.", reversible: true }] }), "```", extra].join("\n");

  it("a card without a move has move null", () => {
    expect(parseCard(base("")).move).toBeNull();
  });
  it("a card with a move carries it, and a bad move or a second block makes the card display-only", () => {
    const blk = (b: string) => "```home-move\n" + b + "\n```";
    expect(parseCard(base(blk(script({})))).move?.kind).toBe("script");
    expect(() => parseCard(base(blk("{}")))).toThrow();
    expect(() => parseCard(base(blk(script({})) + "\n" + blk(script({}))))).toThrow("more than one home-move block");
  });
});

describe("pr move merge-card facts", () => {
  const pr = (o: Record<string, unknown>) => JSON.stringify({ schema: "home-move/v1", kind: "pr", pr: { url: "https://github.com/a/b/pull/1", ...o } });
  it("accepts the optional facts and keeps a bare pr unchanged", () => {
    expect(parseMove(pr({}))).toEqual({ kind: "pr", url: "https://github.com/a/b/pull/1" });
    expect(parseMove(pr({ summary: "adds X", verdict: "PASS", why: "taste", review_url: "https://github.com/a/b/pull/1#issuecomment-2" }))).toMatchObject({ summary: "adds X", verdict: "PASS", why: "taste" });
  });
  it("accepts door, door_why and blast", () => {
    expect(parseMove(pr({ door: "one-way", door_why: "deploy-timer", blast: "deploys the Home plugin" }))).toMatchObject({ door: "one-way", door_why: "deploy-timer", blast: "deploys the Home plugin" });
    expect(parseMove(pr({ door: "two-way" }))).toEqual({ kind: "pr", url: "https://github.com/a/b/pull/1", door: "two-way" });
  });
  it.each([
    ["multiline summary", { summary: "a\nb" }],
    ["long summary", { summary: "x".repeat(201) }],
    ["bad verdict", { verdict: "great" }],
    ["bad why", { why: "whim" }],
    ["off-host review url", { review_url: "https://evil.example/x" }],
    ["unknown field", { note: "x" }],
    ["bad door", { door: "maybe" }],
    ["door_why without door", { door_why: "data-migration" }],
    ["door_why on a two-way door", { door: "two-way", door_why: "data-migration" }],
    ["bad door_why", { door: "one-way", door_why: "vibes" }],
    ["multiline blast", { blast: "a\nb" }],
    ["long blast", { blast: "x".repeat(201) }],
    ["empty blast", { blast: "  " }],
    ["NUL blast", { blast: "\u0000" }],
    ["ESC in blast", { blast: "a\u001b[31mb" }],
    ["DEL in blast", { blast: "a\u007fb" }],
    ["C1 control in blast", { blast: "a\u009fb" }],
    ["tab or form feed in blast", { blast: "a\u000cb" }],
  ])("rejects %s", (_n, o) => {
    expect(() => parseMove(pr(o))).toThrow();
  });
});
