// Bead mk-ff7v6: a command an agent wraps in backticks ("check first: `bash /x.sh --check`") is one bare copy block,
// so the copied text has no backticks. A fenced block that holds a command is read the same way. A backticked word
// that is not a command ("`--check`", a file name) stays prose.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { QuestionText } from "../ui/asks.js";
import { splitCommands } from "../ui/commandtext.js";

const SCRIPT = "/srv/work/m10/m10-deploy.sh";
const CHECK = `bash ${SCRIPT} --check`;
const RUN = `bash ${SCRIPT}`;
const SCP = `scp dev@buildhost:${SCRIPT} /tmp/x.sh && bash /tmp/x.sh`;
const cmds = (text: string) => splitCommands(text).filter((p) => p.type === "command");
const texts = (text: string) => cmds(text).map((p) => p.text);

describe("a backticked command inside a sentence", () => {
  it("comes back bare, with the words around it kept as prose", () => {
    const parts = splitCommands(`Check first: \`${CHECK}\`, then run it.`);
    expect(parts.map((p) => p.text)).toEqual(["Check first:", CHECK, ", then run it."]);
    expect(parts.map((p) => p.type)).toEqual(["prose", "command", "prose"]);
  });
  it("takes each backticked command on one line, in order, with check and final roles", () => {
    const c = cmds(`Check first: \`${CHECK}\`, then run \`${RUN}\`.`);
    expect(c.map((p) => p.text)).toEqual([CHECK, RUN]);
    expect(c.map((p) => p.role)).toEqual(["check", "final"]);
  });
  it("keeps a backticked scp && bash one-liner whole", () => {
    expect(texts(`On the Mac, paste one line: \`${SCP}\` (sha256 ${"8f".repeat(32)}).`)).toEqual([SCP]);
  });
  it("renders a code block whose copied text holds no backtick", () => {
    const html = renderToStaticMarkup(<QuestionText text={`Check first: \`${CHECK}\``} />);
    const box = /<code data-command-text="true">([\s\S]*?)<\/code>/.exec(html);
    expect(box).not.toBeNull();
    expect(box![1]!.replace(/<[^>]+>/g, "")).toBe(CHECK);
    expect(html).toContain("data-copy-button");
  });
});

describe("a backticked command alone on a line", () => {
  it("is the bare command, with a list number or bullet dropped", () => {
    expect(texts(`Run:\n\`${RUN}\``)).toEqual([RUN]);
    expect(texts(`Steps:\n1. \`${CHECK}\`\n- \`${RUN}\``)).toEqual([CHECK, RUN]);
  });
  it("drops a $ prompt inside the backticks", () => {
    expect(texts(`\`$ ${RUN}\``)).toEqual([RUN]);
  });
});

describe("a fenced block", () => {
  it("drops the fence and keeps each command as written", () => {
    const parts = splitCommands(`Run these:\n\`\`\`bash\n${CHECK}\n${RUN}\n\`\`\`\nThat is all.`);
    expect(parts.map((p) => p.text)).toEqual(["Run these:", CHECK, RUN, "That is all."]);
    expect(parts.filter((p) => p.type === "command").map((p) => p.role)).toEqual(["check", "final"]);
  });
  it("keeps a continued command as one block", () => {
    const parts = splitCommands(`\`\`\`\nbash ${SCRIPT} \\\n  --apply\n\`\`\``);
    expect(parts).toEqual([{ type: "command", text: `bash ${SCRIPT} \\\n  --apply`, role: "final" }]);
  });
  it("leaves a fence with no command in it, and an unclosed fence, as prose", () => {
    const log = "```\nerror: disk full\nexit 1\n```";
    expect(splitCommands(log)).toEqual([{ type: "prose", text: log }]);
    expect(cmds("```\nnot closed here")).toEqual([]);
  });
});

describe("what the review found (round 1)", () => {
  it("a command-free fence stays whole, and the fence after it is still read", () => {
    const text = "```\nerror: disk full\n```\nRun:\n```bash\n" + RUN + "\n```";
    expect(splitCommands(text).map((p) => p.text)).toEqual(["```\nerror: disk full\n```\nRun:", RUN]);
  });
  it("a backticked span with space just inside the backticks is left alone, so no character is trimmed", () => {
    for (const s of [`Run \`  ${RUN}  \` now.`, `Run \` ${RUN}\` now.`]) expect(splitCommands(s)).toEqual([{ type: "prose", text: s }]);
  });
  it("a bracketed argument inside a fence stays in the command", () => {
    expect(texts("```\nbash /x.sh [read-only: one GET]\n```")).toEqual(["bash /x.sh [read-only: one GET]"]);
  });
  it("a quote or an && that runs onto the next line keeps the fence one command, exactly as written", () => {
    const quoted = `bash /x.sh "hello\nworld" --dry-run`;
    const chained = "bash /x.sh &&\n  bash /y.sh --check";
    expect(cmds("```\n" + quoted + "\n```").map((p) => [p.text, p.role])).toEqual([[quoted, "final"]]);
    expect(texts("```sh\n" + chained + "\n```")).toEqual([chained]);
  });
  it("a fence named for something other than a shell, or one with a comment and a prompt, stays prose", () => {
    for (const f of ["```text\n" + RUN + "\n```", "```json\n" + RUN + "\n```", "```\n# first\n$ " + RUN + "\n```"]) expect(cmds(f)).toEqual([]);
  });
  it("commands joined by \"and\" after an undo lead are all undo", () => {
    const c = cmds(`Run \`${RUN}\`.\nUndo: \`bash /x.sh down\` and \`bash /y.sh down\``);
    expect(c.map((p) => p.role)).toEqual(["final", "undo", "undo"]);
  });
  it("commands in one fence after an undo lead are all undo", () => {
    const c = cmds(`Run \`${RUN}\`.\nUndo:\n\`\`\`\nbash /x.sh down\nbash /y.sh down\n\`\`\``);
    expect(c.map((p) => p.role)).toEqual(["final", "undo", "undo"]);
  });
});

describe("what the review found (round 2)", () => {
  it("a fence line with space around it is not trimmed: the fence is one command exactly as written", () => {
    for (const body of [`  ${CHECK}  `, `\t${CHECK}`, `${CHECK}  \n${RUN}`]) expect(texts("```bash\n" + body + "\n```")).toEqual([body]);
  });
  it("a continuation keeps its lines' space exactly", () => {
    const body = `bash ${SCRIPT} \\\n    --apply`;
    expect(texts("```\n" + body + "\n```")).toEqual([body]);
  });
  it("a substitution, group or heredoc that runs onto the next line keeps the fence one command", () => {
    for (const body of ["bash /x.sh $(\nbash /y.sh --check)", "bash /x.sh && {\n  bash /y.sh\n}", "bash /x.sh <<'EOF'\nhello\nEOF", "bash /x.sh `\nbash /y.sh`"]) {
      expect(cmds("```bash\n" + body + "\n```").map((p) => [p.text, p.role])).toEqual([[body, "final"]]);
    }
  });
  it("plain lines split one per line; any grouping or substitution keeps the fence one command", () => {
    expect(texts(`\`\`\`\n${CHECK}\nbash /y.sh "a b" 'c'\n\`\`\``)).toEqual([CHECK, `bash /y.sh "a b" 'c'`]);
    const grouped = "bash /x.sh $(date) --check\nbash /y.sh \"(a)\"";
    expect(texts("```\n" + grouped + "\n```")).toEqual([grouped]);
  });
});

describe("what the review found (round 3)", () => {
  it("a substitution with nested quotes across lines keeps the fence one command", () => {
    const body = 'bash /x.sh "$(printf "x\nbash /y.sh")"\nbash /z.sh --check';
    expect(texts("```\n" + body + "\n```")).toEqual([body]);
  });
  it("blank and space-only edge lines stay in a one-command fence", () => {
    const body = "\n  bash /x.sh --check  \n\n";
    expect(texts("```bash\n" + body + "\n```")).toEqual([body]);
  });
});

describe("what the review found (round 4)", () => {
  it("commands joined by && or || after an undo lead are all undo", () => {
    for (const join of ["&&", "||", ";", "or"]) {
      const c = cmds(`Run \`${RUN}\`.\nUndo: \`bash /y.sh down\` ${join} \`bash /z.sh down\``);
      expect(c.map((p) => p.role)).toEqual(["final", "undo", "undo"]);
    }
  });
  it("a plain fenced command with blank lines around it is copied with every character between the fence lines", () => {
    const parts = cmds("```bash\n\n" + CHECK + "\n\n\n```");
    expect(parts.map((p) => p.text)).toEqual(["\n" + CHECK + "\n\n"]);
  });
  it("blank, whitespace-only or carriage-return lines inside a fence keep the fence whole and as written", () => {
    for (const inner of [CHECK + "\n\n" + RUN, CHECK + "\n   \n" + RUN, CHECK + "\r\n" + RUN]) {
      expect(cmds("```bash\n" + inner + "\n```").map((p) => p.text)).toEqual([inner]);
    }
  });
});

describe("what the review found (round 5)", () => {
  it("a fence kept whole that holds a check and a real run is the run, not a check", () => {
    const c = cmds("```bash\n" + CHECK + "\n\n" + RUN + "\n```\nUndo: `bash /srv/work/m10/m10-deploy.sh --rollback`");
    expect(c.map((p) => [p.text, p.role])).toEqual([[CHECK + "\n\n" + RUN, "final"], ["bash /srv/work/m10/m10-deploy.sh --rollback", "undo"]]);
  });
  it("a fence kept whole whose every command is a check is still a check", () => {
    const c = cmds("```bash\n" + CHECK + "\n\nbash /srv/work/m10/other.sh --check\n```\nThen: `" + RUN + "`");
    expect(c.map((p) => p.role)).toEqual(["check", "final"]);
  });
});

describe("backticked words that are not commands", () => {
  it("stay prose, backticks and all", () => {
    for (const s of ["Pass `--check` first.", "It writes `m10-deploy.sh` and `/tmp/out.json`.", "Use `bash` here.", "Note: `bash scripts` need review.", "The `bb` CLI is on PATH.", "A ``bash /x.sh`` double span is left alone."]) {
      const parts = splitCommands(s);
      expect(parts).toEqual([{ type: "prose", text: s }]);
    }
  });
});

describe("the cases that were already split", () => {
  it("a bare command line and an inline command after a colon still split as before", () => {
    expect(texts(`Run:\n${CHECK}`)).toEqual([CHECK]);
    expect(splitCommands(`Do this: ${SCP} (sha256 ab). Then rest.`).map((p) => p.text)).toEqual(["Do this:", SCP, "(sha256 ab). Then rest."]);
  });
});
