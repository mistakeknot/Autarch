// Bead mk-1pnbl: a command written inside a sentence ("paste one line: scp ...") is one copyable block, not
// prose with a path chip after every path. The chips that Home's host adds to path text must not split the command,
// and the File header names the move's script, not a path from the command.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { AskCard, QuestionText, type OwedAsk } from "../ui/asks.js";
import { splitCommands } from "../ui/commandtext.js";

const SCRIPT = "/srv/work/playtest/playtest-mac.sh";
const SCP = `scp dev@buildhost:${SCRIPT} /tmp/x.sh && bash /tmp/x.sh`;
const Q = `After Them playtest 24b (tougher tuning). On the Mac, paste one line: ${SCP} (sha256 ${"8f".repeat(32)}). It builds 'After Them Playtest 24b', copies your newest save and opens it. It takes about 25 min. Load your save. Load your save.`;
const textOf = (html: string) => html.replace(/<[^>]+>/g, "").replace(/&#x27;/g, "'").replace(/&amp;/g, "&");

describe("an inline command in a sentence", () => {
  it("splits out as one command part, exactly as written", () => {
    const parts = splitCommands(Q);
    const cmds = parts.filter((p) => p.type === "command");
    expect(cmds.map((c) => c.text)).toEqual([SCP]);
    // Nothing is lost or added: prose + command + prose reads as the original.
    expect(parts.map((p) => p.text).join(" ").replace(/\s+/g, " ")).toContain("paste one line:");
  });
  it("renders in a code block with a Copy button, and no path inside it is its own chip", () => {
    const html = renderToStaticMarkup(<QuestionText text={Q} />);
    const box = /<code data-command-text="true">([\s\S]*?)<\/code>/.exec(html);
    expect(box).not.toBeNull();
    expect(textOf(box![1]!)).toBe(SCP);
    expect(box![1]).not.toContain("data-ref");
    expect(html).toContain("data-copy-button");
    // The host prefix and its path are one string in the block, not "dev@buildhost:" next to a chip.
    expect(html).not.toMatch(/dev@buildhost:<\/?code[^>]*data-ref/);
  });
  it("leaves the plain sentences alone", () => {
    for (const s of ["Run it: bash is the shell we use here.", "Note: scp is not needed.", "Use bb: it is fast."]) {
      expect(splitCommands(s).some((p) => p.type === "command")).toBe(false);
    }
  });
});

describe("where an inline command ends", () => {
  const cmdOf = (line: string) => splitCommands(line).filter((p) => p.type === "command").map((p) => p.text);
  it("keeps quotes, subshells and a lone dot argument whole", () => {
    expect(cmdOf(`Run: bash -c "printf 'hello (world)\\n'"`)).toEqual([`bash -c "printf 'hello (world)\\n'"`]);
    expect(cmdOf("Run: bash /tmp/x.sh && (bash /tmp/y.sh)")).toEqual(["bash /tmp/x.sh && (bash /tmp/y.sh)"]);
    expect(cmdOf("Run: bash /tmp/x.sh --out .")).toEqual(["bash /tmp/x.sh --out ."]);
  });
  it("keeps a trailing dot argument, and leaves a sentence with a parenthesis alone", () => {
    expect(cmdOf("Run: bash /tmp/x.sh --out ..")).toEqual(["bash /tmp/x.sh --out .."]);
    expect(cmdOf("Note: bash scripts (including this one) need review.")).toEqual([]);
    expect(cmdOf("Note: bash scripts (including ours) need review.")).toEqual([]);
    expect(splitCommands("Run: bash /tmp/x.sh. Then rest.").map((p) => p.text)).toEqual(["Run:", "bash /tmp/x.sh", ". Then rest."]);
  });
  it("keeps the words before and after, and the digest note outside the block", () => {
    const parts = splitCommands(`Do this: ${SCP} (sha256 ab). Then rest.`);
    expect(parts.map((p) => p.text)).toEqual(["Do this:", SCP, "(sha256 ab). Then rest."]);
  });
});

describe("the File header", () => {
  const ask: OwedAsk = { id: "d1", project: "Sylveste", thread: "thr-a", subject: "after them: playtest 24b on the mac (about 25 min)", asker: "thread", filed_at: "2026-10-16T06:00:00Z", revision: "r", task_id: "t1", ask: { question: Q, recommendation: "a", options: [{ id: "a", label: "a: played", kind: "ruling-only" }] } };
  it("names the script the command fetches, not its temp copy", () => {
    const html = renderToStaticMarkup(<AskCard ask={ask} onPick={async () => ({ ok: true })} onOpen={() => {}} />);
    expect(/data-glance-path="true">([^<]*)</.exec(html)?.[1]).toBe(SCRIPT);
    expect(html).not.toMatch(/data-glance-path="true">\/tmp\/x.sh/);
  });
});

describe("a remote path in prose", () => {
  it("stays whole after user@host: and gets no read chip of its own", () => {
    const html = renderToStaticMarkup(<QuestionText text={`Fetch it from dev@buildhost:${SCRIPT} first.`} />);
    expect(html).toContain(`dev@buildhost:${SCRIPT}`);
    expect(html).not.toContain("data-ref");
  });
});
