import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import { buttonClass } from "../ui/buttons.js";
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
  expect(source("blocks")).toContain('className={`${className} space-y-1`} data-row={row.id}');
  expect(source("blocks")).toContain('const className = `rounded-lg border border-border bg-card p-3');
  expect(source("unreadable")).toContain('className="min-w-0 bg-transparent p-4"');
});
