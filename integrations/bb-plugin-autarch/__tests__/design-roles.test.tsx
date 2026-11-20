import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import { buttonClass } from "../ui/buttons.js";
const BARE_DESTRUCTIVE_TEXT = /text-destructive(?![-\w])/;
const source = (name: string) => readFileSync(new URL(`../ui/${name}.tsx`, import.meta.url), "utf8");
it("enters control hover immediately and keeps the recommendation outlined", () => {
  expect(buttonClass()).toContain("hover:duration-0");
  expect(buttonClass("recommended")).toContain("border-primary bg-transparent");
});
it("uses quiet detail rows without hiding answer effects", () => {
  expect(source("asks")).toContain('className="min-w-0 bg-transparent p-4" data-decision');
  expect(source("asks")).toContain('className="border-b border-border py-3" data-reversible');
  expect(source("asks")).toContain('data-effect');
  expect(source("asks")).not.toContain("font-family:");
});
it("uses compatible theme roles for seams, menus and errors", () => {
  for (const name of ["tabs", "inbox"]) expect(source(name)).toContain("border-[var(--border-seam,var(--border))]");
  for (const name of ["tabs", "projectfilter"]) {
    expect(source(name)).toContain("bg-[var(--popover,var(--background))]");
    expect(source(name)).toContain("text-[var(--popover-foreground,var(--foreground))]");
  }
  for (const name of ["asks", "inbox"]) expect(source(name)).toContain("text-[var(--destructive-text,var(--destructive))]");
});
it("gives archive and project inputs transparent input roles and keyboard focus", () => {
  for (const name of ["archive", "projectfilter", "other"]) {
    expect(source(name)).toContain("border-input bg-transparent");
    expect(source(name)).toContain("focus-visible:ring-ring");
    expect(source(name)).toContain("min-h-11");
  }
});

it("separates legacy entries with quiet hairlines", () => {
  expect(source("blocks")).toContain('className="divide-y divide-border [&>*]:py-3" data-legacy-entries');
});
it("keeps unreadable detail transparent with the same quiet padding", () => {
  expect(source("unreadable")).toContain('className="min-w-0 bg-transparent p-4" data-decision');
  expect(source("unreadable")).not.toContain("rounded-lg border");
});

it("keeps unreadable list rows boxed while their detail stays quiet", () => {
  expect(source("blocks")).toContain('className={`${className} space-y-1 [&>[data-unreadable]]:p-0`} data-row={row.id}');
  expect(source("blocks")).toContain('const className = `rounded-lg border border-border bg-card p-3');
  expect(source("unreadable")).toContain('className="min-w-0 bg-transparent p-4"');
});

it("uses text-safe destructive roles at every remaining text site", () => {
  const counts: Record<string, number> = { archive: 2, actionerror: 1, catchup: 1, blocks: 1, docreader: 3, ideas: 4, settings: 2, other: 3, pickhold: 1, notices: 1, update: 2, multipick: 1 };
  for (const [name, count] of Object.entries(counts)) {
    expect(source(name).split("text-[var(--destructive-text,var(--destructive))]").length - 1, name).toBe(count);
    expect(source(name), name).not.toMatch(BARE_DESTRUCTIVE_TEXT);
  }
});
it("gives every remaining text input the input surface and keyboard ring", () => {
  for (const [name, count] of [["multipick", 1], ["settings", 4], ["ideas", 2]] as const) {
    const sites = [...source(name).matchAll(/<(?:input|textarea|select)\b[\s\S]*?className="([^"]*)"/g)];
    const textSites = sites.filter(m => !m[0].includes('type="radio"'));
    expect(textSites.length, name).toBe(count);
    for (const site of textSites) for (const token of ["border-input", "bg-transparent", "focus-visible:outline-none", "focus-visible:ring-2", "focus-visible:ring-ring"]) expect(site[1], name).toContain(token);
  }
});
it("removes nested unreadable padding and the header's unused hover duration", () => {
  expect(source("blocks")).toContain("[&>[data-unreadable]]:p-0");
  const unreadableRow = source("blocks").split('if (row.unreadable && row.decision_id !== null)')[1]?.split('if (!row.display_only')[0];
  expect(unreadableRow).not.toContain("px-1");
  expect(source("buttons").split('export const HEADER_CONTROL =')[1]?.split(';')[0]).not.toContain("hover:duration-0");
});

it("rejects a bare destructive class at every class ending", () => {
  for (const ending of ['"', " ", "`", "}", ""]) expect(`text-destructive${ending}`).toMatch(BARE_DESTRUCTIVE_TEXT);
  for (const safe of ["text-destructive-foreground", "text-destructive-text"]) expect(safe).not.toMatch(BARE_DESTRUCTIVE_TEXT);
});
it("uses the compatible popover roles on the ideas project list", () => {
  const popup = source("ideas").match(/<ul[^>]*data-idea-project-list/)?.[0];
  expect(popup).toContain("border-border");
  expect(popup).toContain("bg-[var(--popover,var(--background))]");
  expect(popup).toContain("text-[var(--popover-foreground,var(--foreground))]");
});
