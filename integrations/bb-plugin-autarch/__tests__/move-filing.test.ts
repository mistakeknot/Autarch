// The home-move/v1 vectors, shared with the Go filer (internal/homeask/move_test.go reads the same file), plus a
// card written the way `autarch needs-mk file --move` writes it. If the Go filer would write a move this parser
// refuses, or the reverse, one of the two reads of this file fails.
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { parseCard } from "../cards";
import { parseMove, READ_HOSTS_ENV } from "../moves";

const file = join(dirname(fileURLToPath(import.meta.url)), "fixtures", "cards", "home-move.json");
const doc = JSON.parse(readFileSync(file, "utf8")) as { kind: string; env: Record<string, string>; cases: { name: string; body: string; error: string | null; kind: string | null }[] };

describe("home-move vectors (shared with Go)", () => {
  // The vectors name the extra read host through the same environment variable the Go filer reads.
  const saved = process.env[READ_HOSTS_ENV];
  beforeEach(() => {
    process.env[READ_HOSTS_ENV] = doc.env[READ_HOSTS_ENV];
  });
  afterEach(() => {
    if (saved === undefined) delete process.env[READ_HOSTS_ENV];
    else process.env[READ_HOSTS_ENV] = saved;
  });
  for (const c of doc.cases) {
    it(c.name, () => {
      if (c.error !== null) expect(() => parseMove(c.body)).toThrow(c.error);
      else expect(parseMove(c.body).kind).toBe(c.kind);
    });
  }
});

describe("the extra read host comes from configuration only", () => {
  const body = '{"schema":"home-move/v1","kind":"read","read":{"url":"https://docs.example.test/plan"}}';
  const saved = process.env[READ_HOSTS_ENV];
  afterEach(() => {
    if (saved === undefined) delete process.env[READ_HOSTS_ENV];
    else process.env[READ_HOSTS_ENV] = saved;
  });
  it("is refused when nothing is configured", () => {
    delete process.env[READ_HOSTS_ENV];
    expect(() => parseMove(body)).toThrow("allowed host");
  });
  it("is allowed when listed (case and spaces ignored), and an invalid entry allows nothing", () => {
    process.env[READ_HOSTS_ENV] = " x.example.test , DOCS.example.test";
    expect(parseMove(body).kind).toBe("read");
    process.env[READ_HOSTS_ENV] = "*.example.test,docs.example.test/path";
    expect(() => parseMove(body)).toThrow("allowed host");
  });
});

describe("a card as the Go filer writes it for --move", () => {
  const pr = '{"schema":"home-move/v1","kind":"pr","pr":{"url":"https://github.com/o/r/pull/12"}}';
  const ask = '{"schema":"home-ask/v2","project":"autarch","project_root":"/srv/autarch","question":"Merge the fix","options":[{"id":"noted","label":"Noted","kind":"ruling-only"},{"id":"not-now","label":"Not now","kind":"ruling-only"}]}';
  const desc = (extra: string) => `Merge the fix\n\nRequest: k-1 sha256:0123456789abcdef\n\n\`\`\`home-ask\n${ask}\n\`\`\`\n${extra}`;

  it("reads back with its move and the synthesized ruling-only ask", () => {
    const c = parseCard(desc(`\n\`\`\`home-move\n${pr}\n\`\`\`\n`));
    expect(c.move).toEqual({ kind: "pr", url: "https://github.com/o/r/pull/12" });
    expect((c.ask.options as { kind: string }[]).map((o) => o.kind)).toEqual(["ruling-only", "ruling-only"]);
  });

  it("an adopted card (block appended after the old text) reads back the same", () => {
    const c = parseCard(desc("") + `\n\`\`\`home-move\n${pr}\n\`\`\`\n`);
    expect(c.move?.kind).toBe("pr");
    expect(c.question).toBe("Merge the fix");
  });

  it("a second block or a bad block makes the whole card display-only", () => {
    const block = `\n\`\`\`home-move\n${pr}\n\`\`\`\n`;
    expect(() => parseCard(desc(block + block))).toThrow("more than one home-move block");
    expect(() => parseCard(desc(`\n\`\`\`home-move\n{"schema":"home-move/v1","kind":"exec"}\n\`\`\`\n`))).toThrow("kind must be");
  });
});
