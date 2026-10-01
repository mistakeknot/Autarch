// Task 2.7: the docs and the agent instructions must not teach the retired `bb home ask`.
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { renderConfigure } from "../feed.js";

const here = dirname(fileURLToPath(import.meta.url));
const P = join(here, "..");
const docs = ["skills/home/SKILL.md", "README.md", "PLUGIN_OVERVIEW.md", "../../AGENTS.md"];

/** A line teaches the command when it mentions `bb home ask` without saying it is retired or moved. */
const teaches = (line: string) => /bb home ask/.test(line) && !/retired|moved|do not use|not use/i.test(line);

describe("docs and instructions", () => {
  for (const f of docs) {
    it(`${f} does not teach bb home ask`, () => {
      const text = readFileSync(join(P, f), "utf8");
      // Join wrapped lines so a "retired" on the next line counts.
      const paras = text.split(/\n\s*\n/).map((x) => x.replace(/\n/g, " "));
      expect(paras.filter((x) => x.split(/(?<=\.)\s/).some(teaches))).toEqual([]);
    });
  }
  it("the skill and the configure note name the card filer", () => {
    expect(readFileSync(join(P, docs[0]!), "utf8")).toContain("autarch needs-mk file");
    expect(renderConfigure([], [])).toContain("autarch needs-mk file");
  });
  it("the checker catches a live teaching line", () => {
    expect(teaches("File one with `bb home ask --request <json>`.")).toBe(true);
    expect(teaches("`bb home ask` is retired.")).toBe(false);
  });
});
