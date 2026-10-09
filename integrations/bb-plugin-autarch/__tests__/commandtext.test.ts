// A card filed as free text carries its commands as prose (AUTA-133, bead mk-yjp7). Split them out so each is one
// copyable block, with the real flags marked and the last command labelled the one to run.
import { describe, expect, it } from "vitest";
import { flagsOf, splitCommands } from "../ui/commandtext.js";

const SCRIPT = "/tmp/example/run-it.sh";
const CHECK = `bash ${SCRIPT} --check`;
const APPLY = `bash ${SCRIPT} --apply --accept-install-diff`;
const AUTA_133 = `Install the reviewed hook. First run the dry run:\n\n${CHECK}\n\nthen, when it reports clean:\n\n${APPLY}\n\nIt writes the install diff and nothing else.`;

describe("splitCommands", () => {
  it("separates prose from commands in order", () => {
    const parts = splitCommands(AUTA_133);
    expect(parts.map((p) => p.type)).toEqual(["prose", "command", "prose", "command", "prose"]);
  });

  it("the commands come back byte for byte", () => {
    const cmds = splitCommands(AUTA_133).filter((p) => p.type === "command").map((p) => p.text);
    expect(cmds).toEqual([CHECK, APPLY]);
  });

  it("the last command is final; an earlier one with --check is a dry run", () => {
    const cmds = splitCommands(AUTA_133).filter((p) => p.type === "command");
    expect(cmds.map((c) => c.role)).toEqual(["check", "final"]);
  });

  it("a single command is the final command", () => {
    const cmds = splitCommands(`Run:\n${APPLY}`).filter((p) => p.type === "command");
    expect(cmds.map((c) => c.role)).toEqual(["final"]);
  });

  it("a prose line that merely starts with a command word is not a command", () => {
    const parts = splitCommands("bash is the shell we use.\nrun it after lunch");
    expect(parts.every((p) => p.type === "prose")).toBe(true);
  });

  it("a leading $ prompt is dropped from the copyable text", () => {
    const [c] = splitCommands(`$ ${CHECK}`).filter((p) => p.type === "command");
    expect(c!.text).toBe(CHECK);
  });

  it("text with no commands is one prose part, unchanged", () => {
    expect(splitCommands("Collapse per project or per day?")).toEqual([{ type: "prose", text: "Collapse per project or per day?" }]);
  });
});

describe("flagsOf", () => {
  it("lists the flags in order, without values", () => {
    expect(flagsOf(APPLY)).toEqual(["--apply", "--accept-install-diff"]);
    expect(flagsOf("bash x.sh --out=/tmp/x --force")).toEqual(["--out", "--force"]);
    expect(flagsOf("bash x.sh")).toEqual([]);
  });
});
