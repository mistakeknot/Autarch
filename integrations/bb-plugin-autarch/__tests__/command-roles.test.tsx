// Bead mk-p9kij: a card that lists a check, the run and then an undo must label the run as the one to run and the
// undo as an undo, not by position.
import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { splitCommands } from "../ui/commandtext.js";
import { QuestionText } from "../ui/asks.js";

const S = "/tmp/example/do-it.sh";
const roles = (t: string) => splitCommands(t).flatMap((p) => (p.type === "command" ? [p.role] : []));

describe("command roles", () => {
  it("check, run, undo: the run is final and the undo is an undo", () => {
    expect(roles(`First:\nbash ${S} --check\nThen:\nbash ${S}\nIf it goes wrong:\nbash ${S} --rollback`)).toEqual(["check", "final", "undo"]);
  });
  it("an undo is found by its lead line when the command has no flag", () => {
    expect(roles(`Run:\nbash ${S} --apply\nUndo:\nbash /tmp/example/back.sh`)).toEqual(["final", "undo"]);
    expect(roles(`Rollback, only if needed:\nbash /tmp/example/back.sh\nRun:\nbash ${S} --apply`)).toEqual(["undo", "final"]);
  });
  it("a rollback subcommand is an undo", () => {
    expect(roles(`Run:\nbash ${S} deploy\nbash ${S} rollback /tmp/backup`)).toEqual(["final", "undo"]);
  });
  it("other orders are unchanged", () => {
    expect(roles(`bash ${S} --check\nbash ${S} --apply`)).toEqual(["check", "final"]);
    expect(roles(`bash ${S} --apply`)).toEqual(["final"]);
    expect(roles(`bash ${S} --a\nbash ${S} --b`)).toEqual(["step", "final"]);
  });
  it("a lone undo command is still the one command", () => {
    expect(roles(`bash ${S} --rollback`)).toEqual(["final"]);
  });
  it("the labels say which is which", () => {
    const html = renderToStaticMarkup(<QuestionText text={`First:\nbash ${S} --check\nThen:\nbash ${S}\nUndo:\nbash ${S} --rollback`} />);
    expect(html).toContain("Before it (dry run / check)");
    expect(html).toContain("Final command");
    expect(html).toContain("Undo (only if needed)");
    expect(html).not.toContain("Earlier step");
  });
});
