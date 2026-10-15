// The Learn deck (bead mk-uoiwp): eight shipped Markdown cards, off by default, marks kept in the settings table.
import { mkdirSync, mkdtempSync, symlinkSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { learnEnabled, learnSet, learnView, loadDeck, type LearnSettings } from "../learn.js";
import { homeTabs, HomeTabs } from "../ui/tabs.js";
import { LearnPanel, LearnSwitch, orderDeck } from "../ui/learn.js";
import { MIGRATIONS } from "../migrations.js";

const shipped = join(dirname(fileURLToPath(import.meta.url)), "..", "learn");
const kv = (): LearnSettings & { m: Map<string, string> } => {
  const m = new Map<string, string>();
  return { m, setting: (k) => m.get(k), setSetting: (k, v) => void m.set(k, v) };
};

describe("the shipped deck", () => {
  it("has the eight cards in order, each with a title and a body", () => {
    const d = loadDeck(shipped);
    expect(d.map((c) => c.id.slice(0, 2))).toEqual(["01", "02", "03", "04", "05", "06", "07", "08"]);
    expect(d.map((c) => c.title)).toEqual([
      "What Home is and what it never does",
      "Cards and options: how a pick works",
      "Your move versus Waiting on you",
      "The vizier and the sub-vizier trial",
      "Gates: what only mk can merge or deploy",
      "Where scripts come from and how to run one",
      "The Idea box",
      "Where to look when something is wrong",
    ]);
    for (const c of d) expect(c.body.length).toBeGreaterThan(40);
  });
  it("says who gets a pick and what Waiting on you counts, as the code does", () => {
    const d = loadDeck(shipped);
    expect(d[1]!.body).toContain("first agent comment");
    expect(d[1]!.body).not.toContain("latest agent comment");
    expect(d[2]!.body).toContain("updates that are only for reading");
    expect(d[2]!.body).toContain("put on hold");

  });
  it("skips files that are not cards and reads nothing from a missing directory", () => {
    const dir = mkdtempSync(join(tmpdir(), "learn-"));
    try {
      writeFileSync(join(dir, "01-a.md"), "# A\n\nbody\n");
      writeFileSync(join(dir, "02-notitle.md"), "no heading\n");
      writeFileSync(join(dir, "notes.md"), "# Not a card\n");
      mkdirSync(join(dir, "03-dir.md"));
      writeFileSync(join(dir, "outside.txt"), "# Outside\n\nnot part of the deck\n");
      symlinkSync(join(dir, "outside.txt"), join(dir, "04-link.md"));
      expect(loadDeck(dir).map((c) => c.id)).toEqual(["01-a"]);
    } finally {
      rmSync(dir, { recursive: true });
    }
    expect(loadDeck(join(dir, "gone"))).toEqual([]);
  });
});

describe("settings state", () => {
  it("is off by default and shows no cards until turned on", () => {
    const s = kv();
    expect(learnEnabled(s)).toBe(false);
    expect(learnView(s, shipped)).toEqual({ enabled: false, cards: [] });
    expect(learnSet(s, shipped, { enabled: true })).toEqual({ ok: true });
    const v = learnView(s, shipped);
    expect(v.enabled).toBe(true);
    expect(v.cards).toHaveLength(8);
    expect(v.cards.every((c) => c.mark === null)).toBe(true);
    learnSet(s, shipped, { enabled: false });
    expect(learnEnabled(s)).toBe(false);
  });
  it("saves Got it and Later per card, and clears them", () => {
    const s = kv();
    learnSet(s, shipped, { enabled: true });
    const id = loadDeck(shipped)[2]!.id;
    expect(learnSet(s, shipped, { card: id, mark: "got" })).toEqual({ ok: true });
    expect(learnView(s, shipped).cards[2]!.mark).toBe("got");
    learnSet(s, shipped, { card: id, mark: "later" });
    expect(learnView(s, shipped).cards[2]!.mark).toBe("later");
    learnSet(s, shipped, { card: id, mark: "clear" });
    expect(learnView(s, shipped).cards[2]!.mark).toBeNull();
  });
  it("refuses an unknown card, a card with no mark and an empty request", () => {
    const s = kv();
    expect(learnSet(s, shipped, { card: "99-nope", mark: "got" })).toMatchObject({ ok: false, status: 404 });
    expect(learnSet(s, shipped, { card: loadDeck(shipped)[0]!.id })).toMatchObject({ ok: false, status: 400 });
    expect(learnSet(s, shipped, {})).toMatchObject({ ok: false, status: 400 });
    expect(s.m.size).toBe(0);
  });
  it("adds no migration: the code files hold no schema statement for it", () => {
    expect(MIGRATIONS.length).toBeGreaterThan(0);
    const src = readFileSync(join(shipped, "..", "learn.ts"), "utf8");
    expect(src).not.toMatch(/create\s+table|alter\s+table|user_version|schema_meta/i);
  });
});

describe("the tab and the panel", () => {
  const noop = () => {};
  it("the Learn tab is absent by default and present when on", () => {
    expect(homeTabs(false).map((t) => t[2])).toEqual(["Queue", "Ideas", "Vizier", "Map", "Settings"]);
    expect(homeTabs(false, true).map((t) => t[2])).toEqual(["Queue", "Ideas", "Learn", "Vizier", "Map", "Settings"]);
    expect(renderToStaticMarkup(<HomeTabs classic={false} onOpen={noop} onToggleClassic={noop} onTodos={noop} />)).not.toContain("Learn");
    expect(renderToStaticMarkup(<HomeTabs classic={false} learn onOpen={noop} onToggleClassic={noop} onTodos={noop} />)).toContain("Learn");
  });
  const cards = loadDeck(shipped).map((c, i) => ({ ...c, mark: (i === 0 ? "got" : i === 1 ? "later" : null) as "got" | "later" | null }));
  it("orders unread first, then Later, then Got it", () => {
    expect(orderDeck(cards).map((c) => c.mark)).toEqual([null, null, null, null, null, null, "later", "got"]);
  });
  it("renders every card with both buttons and the count of cards left", () => {
    const html = renderToStaticMarkup(<LearnPanel cards={cards} onMark={noop} />);
    expect(html.match(/data-learn-card=/g)).toHaveLength(8);
    expect(html.match(/data-learn-got/g)).toHaveLength(8);
    expect(html.match(/data-learn-later/g)).toHaveLength(8);
    expect(html).toContain("7 of 8 cards left.");
    expect(html).toContain("Got it");
    expect(html).toContain("Later");
  });
  it("the Settings switch is unchecked when off", () => {
    expect(renderToStaticMarkup(<LearnSwitch enabled={false} onChange={noop} />)).not.toContain("checked");
    expect(renderToStaticMarkup(<LearnSwitch enabled onChange={noop} />)).toContain("checked");
  });
});
