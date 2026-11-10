// The phone inbox (bead mk-yah6): which cards land in Waiting and Later, the tabs, and the sticky bar on a card.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { inboxRows, MobileInbox, type InboxTab } from "../ui/inbox.js";
import type { AsksData, OwedAsk } from "../ui/asks.js";

const NOW = Date.parse("2026-10-09T12:00:00Z");
const ask = (over: Partial<OwedAsk> = {}): OwedAsk => ({
  id: "d1",
  project: "Clavain",
  thread: "thr-a",
  subject: "Which effort change?",
  asker: "thread",
  filed_at: "2026-10-09T07:00:00Z",
  revision: "r1",
  task_id: "t1",
  ask: { question: "Pick one.", recommendation: "a", options: [{ id: "a", label: "A: no change", kind: "ruling-only" }, { id: "b", label: "B: change it", kind: "ruling-only" }] },
  ...over,
});
const asks = (owed: OwedAsk[]): AsksData => ({ owed, runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {} });
const LATER = { at: "2026-10-09T09:00:00Z", by: "mk" };
const noop = () => {};

const view = (a: AsksData, initial?: { tab?: InboxTab; open?: string }) =>
  renderToStaticMarkup(<MobileInbox asks={a} waiting={2} onPick={noop as never} onOpen={noop} onNote={noop} onLater={noop} sinceNode={<p>since body</p>} sinceCount={3} onDesktop={noop} now={NOW} {...(initial ? { initial } : {})} />);

describe("inboxRows", () => {
  it("puts live decisions in Waiting and set-aside cards in Later; held cards are listed apart, under the On hold row", () => {
    const r = inboxRows(asks([ask(), ask({ id: "d2", task_id: "t2", later: LATER }), ask({ id: "d3", task_id: "t3", held: { reason: "x", by: "v", at: "z" } })]), NOW);
    expect(r.waiting.map((x) => x.key)).toEqual(["ask:d1"]);
    expect(r.later.map((x) => x.key)).toEqual(["ask:d2"]);
    expect(r.held.map((x) => x.key)).toEqual(["ask:d3"]);
  });
  it("labels the kind and age", () => {
    const r = inboxRows(asks([ask()]), NOW);
    expect(r.waiting[0]).toMatchObject({ chip: "Decide", age: "5 h", project: "Clavain" });
  });
});

describe("MobileInbox", () => {
  it("opens on Waiting with one row per card, the count in the title and three tabs at tap height", () => {
    const html = view(asks([ask(), ask({ id: "d2", task_id: "t2", later: LATER })]));
    expect(html).toContain("Waiting on you · 2");
    expect(html).toContain('data-inbox-row="ask:d1"');
    expect(html).not.toContain("ask:d2");
    for (const t of ["waiting", "later", "since"]) expect(html.match(new RegExp(`<button[^>]*data-inbox-tab="${t}"[^>]*>`))![0]).toContain("min-h-14");
    expect(html).toContain("Later 1");
    expect(html).toContain("Since you left 3");
    expect(html).toContain("data-inbox-desktop");
  });
  it("a row shows the card key, the title and the recommended option", () => {
    const html = view(asks([ask({ key: "AUTA-318", subject: "Which effort change?" })]));
    expect(html).toContain('data-row-key="true">AUTA-318<');
    expect(html).toContain("Which effort change?");
    expect(html).toContain("Rec: A: no change");
    expect(view(asks([ask({ key: null, ask: { question: "q", options: [{ id: "a", label: "A", kind: "ruling-only" }] } })]))).not.toContain("data-row-rec");
  });

  it("says so when nothing waits", () => {
    expect(view(asks([]))).toContain("Nothing is waiting on you.");
  });
  it("Since you left shows the page's catch-up body", () => {
    const html = view(asks([]), { tab: "since" });
    expect(html).toContain("since body");
    expect(html).toContain("Since you left");
  });
  it("a decision opens full-screen with the whole card and a bar: Later and the recommended option", () => {
    const html = view(asks([ask()]), { open: "ask:d1" });
    expect(html).toContain('data-inbox-detail="ask:d1"');
    expect(html).toContain('data-option="b"'); // every option stays on the card
    expect(html).toContain("data-inbox-back");
    expect(html).not.toContain("data-inbox-tabs");
    expect(html.match(/<button[^>]*data-bar-later[^>]*>[^<]*/)![0]).toMatch(/Later$/);
    expect(html.match(/<button[^>]*data-bar-main[^>]*>[^<]*/)![0]).toMatch(/A: no change$/);
  });
  it("with no recommendation the bar offers only Later and points at the options", () => {
    const html = view(asks([ask({ ask: { question: "?", options: [{ id: "a", label: "A", kind: "ruling-only" }] } })]), { open: "ask:d1" });
    expect(html).not.toContain("data-bar-main");
    expect(html).toContain("Choose an option above.");
  });
  it("never makes a destructive option the one big button", () => {
    const html = view(asks([ask({ ask: { question: "?", recommendation: "a", options: [{ id: "a", label: "Delete it", kind: "instruction", instruction: "Delete it.", reversible: false }] } })]), { open: "ask:d1" });
    expect(html).not.toContain("data-bar-main");
  });
  it("a Later card's bar says Move back", () => {
    expect(view(asks([ask({ later: LATER })]), { tab: "later", open: "ask:d1" }).match(/<button[^>]*data-bar-later[^>]*>[^<]*/)![0]).toMatch(/Move back$/);
  });
  it("a stale open key falls back to the list", () => {
    expect(view(asks([ask()]), { open: "ask:gone" })).not.toContain("data-inbox-detail");
  });
  it("shows the update control on the list, and not on a card's detail (bead mk-nbt6c)", () => {
    const withUpdate = (initial?: { open?: string }) =>
      renderToStaticMarkup(<MobileInbox asks={asks([ask()])} waiting={1} onPick={noop as never} onOpen={noop} onNote={noop} onLater={noop} sinceNode={null} sinceCount={0} onDesktop={noop} updateNode={<span>Update available</span>} now={NOW} {...(initial ? { initial } : {})} />);
    expect(withUpdate()).toContain("data-inbox-update");
    expect(withUpdate()).toContain("Update available");
    expect(withUpdate({ open: "ask:d1" })).not.toContain("data-inbox-update");
    expect(view(asks([ask()]))).not.toContain("data-inbox-update");
  });
});
