// A card's commands as copyable blocks (bead mk-yjp7): flags are marked, the final command stands out, and the text
// a person copies is the line, character for character.
import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QuestionText } from "../ui/asks.js";
import { CommandBlock } from "../ui/copy.js";

const SCRIPT = "/tmp/example/run-it.sh";
const CHECK = `bash ${SCRIPT} --check`;
const APPLY = `bash ${SCRIPT} --apply --accept-install-diff`;
const TEXT = `Install the reviewed hook. First run the dry run:\n\n${CHECK}\n\nthen, when it reports clean:\n\n${APPLY}\n\nIt writes the install diff and nothing else.`;

const textOf = (html: string) => html.replace(/<[^>]+>/g, "").replace(/&#x27;/g, "'").replace(/&amp;/g, "&");

describe("CommandBlock flags", () => {
  it("bolds each flag and keeps the text identical", () => {
    const html = renderToStaticMarkup(<CommandBlock label="x" command={APPLY} />);
    expect(html.match(/data-flag/g)).toHaveLength(2);
    const code = /<code data-command-text="true">([\s\S]*?)<\/code>/.exec(html)![1]!;
    expect(textOf(code)).toBe(APPLY);
  });
  it("lists the flags and makes the copy button primary when asked", () => {
    const html = renderToStaticMarkup(<CommandBlock label="Final command" command={APPLY} primary showFlags />);
    expect(html).toContain("Flags in this command: --apply --accept-install-diff.");
    expect(html).toContain('data-tone="recommended"');
  });
  it("lists no flags line when the command has none", () => {
    expect(renderToStaticMarkup(<CommandBlock label="x" command="bb status" showFlags />)).not.toContain("data-command-flags");
  });
});

describe("QuestionText (the AUTA-133 shape)", () => {
  const html = renderToStaticMarkup(<QuestionText text={TEXT} />);
  it("shows two blocks, labelled, the last one final and primary", () => {
    expect(html.match(/data-command-box/g)).toHaveLength(2);
    expect(html).toContain("Before it (dry run / check)");
    expect(html).toContain("Final command");
    expect(html.match(/data-tone="recommended"/g)).toHaveLength(1);
    expect(html).toContain("Flags in this command: --apply --accept-install-diff.");
  });
  it("keeps the prose around the commands", () => {
    expect(html).toContain("Install the reviewed hook.");
    expect(html).toContain("It writes the install diff and nothing else.");
  });
  it("plain prose is unchanged: one paragraph, no blocks", () => {
    const p = renderToStaticMarkup(<QuestionText text="Should we ship on Friday?" />);
    expect(p).not.toContain("data-command-box");
    expect(p).toContain("Should we ship on Friday?");
  });
});
