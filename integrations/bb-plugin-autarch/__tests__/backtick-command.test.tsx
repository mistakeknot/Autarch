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

describe("what the review found (round 6)", () => {
  const UNDO = "Undo: `bash /srv/work/m10/m10-deploy.sh --rollback`";
  it("a check chained to the real run by && is the run, in a fence or in backticks", () => {
    for (const join of ["&&", "||", ";"]) {
      const chain = `${CHECK} ${join} ${RUN}`;
      for (const text of ["```bash\n" + chain + "\n```\n\n" + UNDO, "Run `" + chain + "`.\n" + UNDO]) {
        expect(cmds(text).map((p) => [p.text, p.role])).toEqual([[chain, "final"], ["bash /srv/work/m10/m10-deploy.sh --rollback", "undo"]]);
      }
    }
  });
  it("a chain of checks is a check and a chain of rollbacks is an undo", () => {
    const checks = `${CHECK} && bash /srv/work/m10/other.sh --check`;
    const undos = "bash /srv/work/m10/m10-deploy.sh --rollback && bash /srv/work/m10/other.sh --rollback";
    expect(cmds("Check: `" + checks + "`\nRun: `" + RUN + "`\nUndo: `" + undos + "`").map((p) => p.role)).toEqual(["check", "final", "undo"]);
    expect(cmds("Run `" + RUN + "`, and `" + undos + "` if it fails.").map((p) => p.role)).toEqual(["final", "undo"]);
  });
  it("a rollback chained to a real run is the run, not an undo", () => {
    const chain = `bash /srv/work/m10/m10-deploy.sh --rollback && ${RUN}`;
    expect(cmds("Check: `" + CHECK + "`\nThen `" + chain + "`").map((p) => p.role)).toEqual(["check", "final"]);
  });
});

describe("what the review found (round 7)", () => {
  const UNDO = "Undo: `bash /srv/work/m10/m10-deploy.sh --rollback`";
  it("a check piped into or backgrounded with the real run is the run", () => {
    for (const join of ["|", "&"]) {
      const chain = `${CHECK} ${join} ${RUN}`;
      expect(cmds("Run `" + chain + "`.\n" + UNDO).map((p) => [p.text, p.role])).toEqual([[chain, "final"], ["bash /srv/work/m10/m10-deploy.sh --rollback", "undo"]]);
    }
  });
  it("a check with its output redirected is still a check", () => {
    for (const check of [`${CHECK} 2>&1`, `${CHECK} &> /tmp/check.log`, `${CHECK} >> /tmp/check.log 2>&1`]) {
      expect(cmds("First: `" + check + "`\nThen: `" + RUN + "`").map((p) => [p.text, p.role])).toEqual([[check, "check"], [RUN, "final"]]);
    }
  });
});

describe("what the review found (round 8)", () => {
  it("a pipe, a substitution, a quoted flag or a commented flag never makes a check or an undo", () => {
    for (const block of [
      `${CHECK} | tee >(${RUN})`,
      `bash /srv/work/m10/m10-deploy.sh --rollback | tee >(${RUN})`,
      `${CHECK} | tail -n 20`,
      `${CHECK} && tee /srv/live/config`,
      `${RUN} --message "please --check now"`,
      `${RUN} --message 'roll back with --rollback'`,
      `${RUN} # --check`,
      `${CHECK} $(${RUN})`,
    ]) {
      expect(cmds("First: `" + block + "`\nThen: `bash /srv/work/m10/after.sh`").map((p) => p.role)).toEqual(["step", "final"]);
    }
  });
  it("a separator inside quotes does not split a check", () => {
    const check = `${CHECK} --note "a;b && c"`;
    expect(cmds("First: `" + check + "`\nThen: `" + RUN + "`").map((p) => p.role)).toEqual(["check", "final"]);
  });
});

describe("what the review found (round 9)", () => {
  it("a substitution in double quotes, an interpreter payload or a redirection to a flag-shaped name never makes a check or an undo", () => {
    for (const block of [
      `${CHECK} "$(bash /srv/live.sh)"`,
      "bash /srv/work/m10/m10-deploy.sh --rollback \"`bash /srv/live.sh`\"",
      `bash -c "bash /srv/live.sh" --check`,
      `sh -c 'bash /srv/live.sh' --rollback`,
      `python3 -c "import os; os.system('bash /srv/live.sh')" --check`,
      `bash /srv/live.sh > --check`,
      `bash /srv/live.sh >> --rollback`,
      `sudo bash -c "bash /srv/live.sh" --check`,
    ]) {
      const c = cmds("```bash\n" + block + "\n```\nThen: `bash /srv/work/m10/after.sh`");
      expect(c.map((p) => [p.text, p.role])).toEqual([[block, "step"], ["bash /srv/work/m10/after.sh", "final"]]);
    }
  });
});

describe("what the review found (round 10)", () => {
  it("only a known command shape with the exact flag as its own argument is a check or an undo", () => {
    for (const block of [
      `sudo -u root bash -c "bash /srv/live.sh" --check`,
      `sudo /bin/bash -c "bash /srv/live.sh" --rollback`,
      `sudo -s bash /srv/live.sh --check`,
      `bash /usr/bin/env bash /srv/live.sh --check`,
      `bash /srv/live.sh -- --check`,
      `bash /srv/live.sh --label --check`,
      `bash /srv/live.sh --check=false`,
      `bash /srv/live.sh --check --rollback`,
    ]) {
      const c = cmds("```bash\n" + block + "\n```\nThen: `bash /srv/work/m10/after.sh`");
      expect(c.map((p) => [p.text, p.role])).toEqual([[block, "step"], ["bash /srv/work/m10/after.sh", "final"]]);
    }
  });
  it("the plain shapes keep their roles", () => {
    for (const [block, role] of [
      [CHECK, "check"],
      [`sudo -u root bash /srv/live.sh --check`, "check"],
      [`runuser -u mk -- bash /srv/live.sh --dry-run`, "check"],
      [`bash /srv/live.sh --label x --check`, "check"],
      [`bash /srv/live.sh --check > /tmp/check.log 2>&1`, "check"],
      [`sudo bash /srv/live.sh rollback /tmp/backup`, "undo"],
      [`python3 /srv/live.py --rollback`, "undo"],
    ] as const) {
      const c = cmds("```bash\n" + block + "\n```\nThen: `bash /srv/work/m10/after.sh`");
      expect(c.map((p) => [p.text, p.role])).toEqual([[block, role], ["bash /srv/work/m10/after.sh", "final"]]);
    }
  });
});

describe("what the review found (round 11)", () => {
  it("only a .sh, .bash or .py script path is the script of a check or an undo, and a role flag with a value voids the role", () => {
    for (const block of [
      `sudo /usr/bin/python3.12 -c "print(123)" --check`,
      `sudo docker run alpine sh -c "touch /tmp/live" --check`,
      `sudo /srv/bin/deploy --check`,
      `bash /srv/live.sh --check --check=false`,
      `bash /srv/live.sh --dry-run --dry-run=false`,
      `bash /srv/live.sh --rollback --rollback=false`,
      `bash /srv/live.sh rollback --undo=no`,
    ]) {
      const c = cmds("```bash\n" + block + "\n```\nThen: `bash /srv/work/m10/after.sh`");
      expect(c.map((p) => [p.text, p.role])).toEqual([[block, "step"], ["bash /srv/work/m10/after.sh", "final"]]);
    }
  });
  it("relative and home script paths keep their roles", () => {
    for (const [block, role] of [
      [`bash ./live.sh --check`, "check"],
      [`bash ~/live.bash --dry-run`, "check"],
      [`sudo bash ./live.sh --undo`, "undo"],
    ] as const) {
      const c = cmds("```bash\n" + block + "\n```\nThen: `bash /srv/work/m10/after.sh`");
      expect(c.map((p) => [p.text, p.role])).toEqual([[block, role], ["bash /srv/work/m10/after.sh", "final"]]);
    }
  });
});

describe("what the review found (round 12)", () => {
  it("a redirection glued to an argument, an expansion, a glob or a brace makes the command a step", () => {
    for (const block of [
      `bash /srv/live.sh --check2>/tmp/log`,
      `bash /srv/live.sh --dry-run3>/tmp/log`,
      `bash /srv/live.sh --rollback2>/tmp/log`,
      `bash /srv/live.sh --check2>&1`,
      `bash /srv/live.sh --undo1>>/tmp/log`,
      `bash /srv/live.sh $OPTIONS --check`,
      `bash /srv/live.sh --check $OPTIONS`,
      `bash /srv/live.sh --check "$OPTIONS"`,
      `bash /srv/live.sh \${OPTIONS} --rollback`,
      `bash /srv/live.sh * --check`,
      `bash /srv/live.sh --check /tmp/o?`,
      `bash /srv/live.sh --check /tmp/[ab]`,
      `bash /srv/live.sh {--check,--apply}`,
    ]) {
      const c = cmds("```bash\n" + block + "\n```\nThen: `bash /srv/work/m10/after.sh`");
      expect(c.map((p) => [p.text, p.role])).toEqual([[block, "step"], ["bash /srv/work/m10/after.sh", "final"]]);
    }
  });
  it("a redirection that stands as its own word still leaves a check a check", () => {
    for (const [block, role] of [
      [`bash /srv/live.sh --check 2>/tmp/log`, "check"],
      [`bash /srv/live.sh --check 2>&1`, "check"],
      [`bash /srv/live.sh --rollback >> /tmp/log 2>&1`, "undo"],
    ] as const) {
      const c = cmds("```bash\n" + block + "\n```\nThen: `bash /srv/work/m10/after.sh`");
      expect(c.map((p) => [p.text, p.role])).toEqual([[block, role], ["bash /srv/work/m10/after.sh", "final"]]);
    }
  });
  it("a fence whose info string carries more than a shell name, or that never closes, stays a prose sample", () => {
    for (const open of ["```text title=\"example\"", "```bash title=\"example\"", "```c++", "```{.bash}", "~~~ text"]) {
      const close = open.startsWith("~") ? "~~~" : "```";
      const text = open + "\n`bash /srv/live.sh --check`\n" + close;
      expect(splitCommands(text)).toEqual([{ type: "prose", text }]);
    }
    const unclosed = "Example:\n```text\n`bash /srv/live.sh --check`\nbash /srv/live.sh";
    expect(splitCommands(unclosed)).toEqual([{ type: "prose", text: unclosed }]);
    const unclosedShell = "```bash\nbash /srv/live.sh --check";
    expect(cmds(unclosedShell)).toEqual([]);
  });
});

describe("what the review found (round 13)", () => {
  it("a redirection operand with a quoted part is dropped whole, and whitespace the shell keeps in an argument makes a step", () => {
    for (const block of [
      `bash /srv/live.sh > ""--check`,
      `bash /srv/live.sh > ''--rollback`,
      `bash /srv/live.sh 2> "log"--dry-run`,
      `bash /srv/live.sh --check\u00a0--apply`,
      `bash /srv/live.sh --rollback\u000b --apply`,
      `bash /srv/live.sh\u2003--check`,
    ]) {
      const c = cmds("Run `" + block + "` and then `bash /srv/work/m10/after.sh`.");
      expect(c.map((p) => [p.text, p.role])).toEqual([[block, "step"], ["bash /srv/work/m10/after.sh", "final"]]);
    }
    // A form feed at the end would be trimmed from the copied text, so the span stays prose instead.
    expect(cmds("Run `bash /srv/live.sh --check\f` and then `bash /srv/work/m10/after.sh`.").map((p) => p.text)).toEqual(["bash /srv/work/m10/after.sh"]);
    const fence = "```bash\nbash /srv/live.sh --check\r\n\nbash /srv/other.sh --check\n```\nThen: `bash /srv/work/m10/after.sh`";
    expect(cmds(fence).map((p) => p.role)).toEqual(["step", "final"]);
  });
  it("a backticked command whose words are split by a tab is a command, with its tab kept", () => {
    const c = cmds("Run `bash\t/srv/live.sh` now.");
    expect(c.map((p) => p.text)).toEqual(["bash\t/srv/live.sh"]);
  });
});

describe("what the review found (round 14)", () => {
  it("a backslash-newline joins the two halves of a word, as the shell does", () => {
    for (const flag of ["--check", "--dry-run", "--rollback", "--undo", "--revert"]) {
      const block = `bash /srv/live.sh ${flag}\\\n=false`;
      const c = cmds("```bash\n" + block + "\n```\nThen: `bash /srv/work/m10/after.sh`");
      expect(c.map((p) => [p.text, p.role])).toEqual([[block, "step"], ["bash /srv/work/m10/after.sh", "final"]]);
    }
    const glued = `bash /srv/live.sh --check2\\\n>/tmp/log`;
    expect(cmds("```bash\n" + glued + "\n```\nThen: `bash /srv/work/m10/after.sh`").map((p) => p.role)).toEqual(["step", "final"]);
    const spaced = `bash /srv/live.sh \\\n  --check`;
    expect(cmds("```bash\n" + spaced + "\n```\nThen: `bash /srv/work/m10/after.sh`").map((p) => p.role)).toEqual(["check", "final"]);
  });
});

describe("what the review found (round 15)", () => {
  it("an undo word with a check flag after it is a step, and with an undo flag after it stays an undo", () => {
    for (const word of ["rollback", "undo", "revert"]) {
      for (const flag of ["--check", "--dry-run"]) {
        const c = cmds("Run `bash /srv/live.sh " + word + " " + flag + "` then `bash /srv/work/m10/after.sh`.");
        expect(c.map((p) => p.role)).toEqual(["step", "final"]);
      }
      const c = cmds("Run `bash /srv/live.sh " + word + " --undo` then `bash /srv/work/m10/after.sh`.");
      expect(c.map((p) => p.role)).toEqual(["undo", "final"]);
    }
  });
});

describe("what the review found (round 16)", () => {
  it("a flag in quotes is read as the shell passes it, so it can void or conflict with the role", () => {
    const blocks = [
      `bash /srv/live.sh --check "--check=false"`,
      `bash /srv/live.sh --check '--check=false'`,
      `bash /srv/live.sh --dry-run --dry"-run=false"`,
      `bash /srv/live.sh --rollback "--rollback=false"`,
      `bash /srv/live.sh --undo '--undo=no'`,
      `bash /srv/live.sh --revert "--revert=0"`,
      `bash /srv/live.sh rollback "--check"`,
      `bash /srv/live.sh undo '--dry-run'`,
      `bash /srv/live.sh --check "--rollback"`,
      `bash /srv/live.sh --check""=false --check`,
      `bash /srv/live.sh --check ''--dry-run=false`,
    ];
    for (const block of blocks) {
      const inline = cmds("Run `" + block + "` then `bash /srv/work/m10/after.sh`.");
      expect(inline.map((p) => [p.text, p.role])).toEqual([[block, "step"], ["bash /srv/work/m10/after.sh", "final"]]);
      const fenced = cmds("```bash\n" + block + "\n```\nThen: `bash /srv/work/m10/after.sh`");
      expect(fenced.map((p) => [p.text, p.role])).toEqual([[block, "step"], ["bash /srv/work/m10/after.sh", "final"]]);
    }
  });
  it("a quoted flag that agrees, or quoted words that are no flag, keep the role", () => {
    for (const [block, role] of [
      [`bash /srv/live.sh "--check"`, "check"],
      [`bash /srv/live.sh --label "a | b; c" --check`, "check"],
      [`bash /srv/live.sh rollback '/tmp/backup dir'`, "undo"],
    ] as const) {
      const c = cmds("```bash\n" + block + "\n```\nThen: `bash /srv/work/m10/after.sh`");
      expect(c.map((p) => [p.text, p.role])).toEqual([[block, role], ["bash /srv/work/m10/after.sh", "final"]]);
    }
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
