// Bead mk-vaiyo (regression of mk-1pnbl): a command line that opens with a list number ("(1) " or "1. ") or ends in a
// bracketed note is one copyable block, not prose with a path chip and loose flags. The number and the note are not
// part of what is copied.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { QuestionText } from "../ui/asks.js";
import { splitCommands } from "../ui/commandtext.js";

const P = "/srv/handoff/stripe/create-test-prices.sh";
const SHA = "1d5fbe3243688d49c39507df979f27b82b96d4a384b8b81f40165498e78464bf";
const N1 = "[read-only: one Stripe GET, creates nothing]";
const N2 = "[creates the test product and two prices via the Stripe API; no safe dry run for this step]";
const JAWN_111 = `Run in a root shell on the server:\n(1) bash ${P} --check   ${N1}\n(2) bash ${P}   ${N2}\nsha256 ${SHA}\nREHEARSED sha256=${SHA} uid=1001 check_rc=0\nIf the key lacks write access, step 2 stops.`;
const cmds = (t: string) => splitCommands(t).filter((p) => p.type === "command");
const textOf = (html: string) => html.replace(/<[^>]+>/g, "").replace(/&#x27;/g, "'").replace(/&amp;/g, "&");

describe("a numbered command line", () => {
  it("stays fast on a long line full of brackets and spaces", () => {
    const line = "1. bash x " + "[a ".repeat(20000);
    const t0 = Date.now();
    splitCommands(line);
    expect(Date.now() - t0).toBeLessThan(500);
  });

  it.each(["(1) ", "1. ", "1) ", "  (12) "])("drops the list marker %j from the copied text", (m) => {
    expect(cmds(`${m}bash ${P} --check`).map((c) => c.text)).toEqual([`bash ${P} --check`]);
  });
  it("splits a trailing bracketed note off the command and keeps it as prose", () => {
    const parts = splitCommands(`(1) bash ${P} --check   ${N1}`);
    expect(parts.filter((p) => p.type === "command").map((p) => p.text)).toEqual([`bash ${P} --check`]);
    expect(parts.filter((p) => p.type === "prose").map((p) => p.text)).toEqual([N1]);
  });
  it("reads both steps of the JAWN-111 card as commands, check then final", () => {
    const c = cmds(JAWN_111);
    expect(c.map((x) => x.text)).toEqual([`bash ${P} --check`, `bash ${P}`]);
    expect(c.map((x) => x.role)).toEqual(["check", "final"]);
  });
  it("leaves numbered prose alone", () => {
    for (const s of ["(1) bash is the shell we use", "1. the bash script is below", "(2) Note: bash scripts need review [see doc]"]) {
      expect(cmds(s)).toEqual([]);
    }
  });
  it("keeps a bracket that belongs to the command", () => {
    for (const c of ["bash /tmp/x.sh --only [a-z]*", "bash /tmp/x.sh && [ -f /tmp/ready ]", "(1) bash /tmp/x.sh && [ -f /tmp/ready ]", "bash /tmp/x.sh || [[ -d /tmp/a b ]]", "bash /tmp/x.sh --only [a\\ b]", "bash /tmp/x.sh /tmp/\\ [backup\\ files]", 'bash /tmp/x.sh --only [a" "b]', 'bash /tmp/x.sh --label "release [candidate one"]', 'bash /tmp/x.sh --label "see [the note here]"', "bash /tmp/x.sh; [ -z \"$A B\" ]"]) {
      expect(cmds(c).map((x) => x.text)).toEqual([c.replace(/^\(1\) /, "")]);
    }
  });
  it("splits the note the same way with or without a number", () => {
    const bare = splitCommands(`bash ${P} --check ${N1}`);
    expect(bare.filter((p) => p.type === "command").map((p) => p.text)).toEqual([`bash ${P} --check`]);
    expect(bare.filter((p) => p.type === "prose").map((p) => p.text)).toEqual([N1]);
  });
  it("a flag named inside a note does not change the command's role", () => {
    const c = cmds(`bash /tmp/a.sh [do not use --check here]\nbash /tmp/b.sh --apply`);
    expect(c.map((x) => x.role)).toEqual(["step", "final"]);
  });
});

describe("the JAWN-111 card as rendered", () => {
  const html = renderToStaticMarkup(<QuestionText text={JAWN_111} />);
  it("has two copy buttons and no path chip inside a command block", () => {
    expect(html.match(/data-copy-button/g)?.length).toBe(2);
    for (const m of html.matchAll(/<code data-command-text="true">([\s\S]*?)<\/code>/g)) expect(m[1]).not.toContain("data-ref");
    expect(html).not.toContain("data-doc-open");
  });
  it("shows each command exactly and its note after it", () => {
    const boxes = [...html.matchAll(/<code data-command-text="true">([\s\S]*?)<\/code>/g)].map((m) => textOf(m[1]!));
    expect(boxes).toEqual([`bash ${P} --check`, `bash ${P}`]);
    expect(textOf(html)).toContain(N1);
  });
  it("does not let a sha256 break across lines", () => {
    const chips = [...html.matchAll(/<code[^>]*data-ref[^>]*>([^<]*)<\/code>/g)].filter((m) => m[1] === SHA);
    expect(chips.length).toBe(2);
    for (const m of chips) expect(m[0]).toContain("whitespace-nowrap");
    expect(chips[0]![0]).not.toContain("overflow-wrap:anywhere");
  });
});
