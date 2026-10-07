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
    ["context too long", mv({ kind: "context", context: { need: "x".repeat(2001) } })],
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
    for (const c of cmds) expect(c.command).toBe(c.command.trim()), expect(c.command).not.toMatch(/[\n\r`]/);
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
