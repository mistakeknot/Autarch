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
