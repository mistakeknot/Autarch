// The phone inbox (bead mk-yah6): which cards land in Waiting and Later, the tabs, and the sticky bar on a card.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { inboxRows, MobileInbox, type InboxTab } from "../ui/inbox.js";
import type { AsksData, OwedAsk } from "../ui/asks.js";
import type { MoveView, MoveViewGroups } from "../moveview.js";

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
const move = (over: Partial<MoveView> = {}): MoveView =>
  ({ task_id: "m1", generation: 1, kind: "script", state: "open", title: "Run the bundle script", owner: null, opened_at: "2026-10-09T10:00:00Z", claimed_at: null, skipped_at: null, checked_at: null, report_deadline_at: null, url: null, need: null, pr: null, script: null, commands: [], report: null, closed_at: null, closed_by: null, evidence: null, card_later: false, ...over }) as MoveView;
const groups = (g: Partial<MoveViewGroups> = {}): MoveViewGroups => ({ yourMove: [], reported: [], later: [], hidden: [], audit: [], ...g });
const LATER = { at: "2026-10-09T09:00:00Z", by: "mk" };
const noop = () => {};

const view = (a: AsksData, m: MoveViewGroups | null, initial?: { tab?: InboxTab; open?: string }) =>
  renderToStaticMarkup(<MobileInbox asks={a} moves={m} waiting={2} onPick={noop as never} onOpen={noop} onNote={noop} onLater={noop} handlers={{ onCheck: noop, onClaim: noop, onSkip: noop, onNote: noop }} sinceNode={<p>since body</p>} sinceCount={3} onDesktop={noop} now={NOW} {...(initial ? { initial } : {})} />);

describe("inboxRows", () => {
  it("puts decisions then free moves in Waiting, and set-aside cards in Later; held cards appear nowhere", () => {
    const r = inboxRows(asks([ask(), ask({ id: "d2", task_id: "t2", later: LATER }), ask({ id: "d3", task_id: "t3", held: { reason: "x", by: "v", at: "z" } })]), groups({ yourMove: [move()], later: [move({ task_id: "m2" })] }), NOW);
    expect(r.waiting.map((x) => x.key)).toEqual(["ask:d1", "move:m1:1"]);
    expect(r.later.map((x) => x.key)).toEqual(["ask:d2", "move:m2:1"]);
  });
  it("shows a move that rides on a decision's card once, inside the decision", () => {
    const r = inboxRows(asks([ask({ task_id: "m1" })]), groups({ yourMove: [move()] }), NOW);
    expect(r.waiting.map((x) => x.key)).toEqual(["ask:d1"]);
  });
  it("keeps held moves out of both lists", () => {
    const r = inboxRows(asks([]), groups({ yourMove: [move({ held: { reason: "r", by: "v", at: "z" } })], later: [move({ task_id: "m2", held: { reason: "r", by: "v", at: "z" } })] }), NOW);
    expect(r.waiting).toEqual([]);
    expect(r.later).toEqual([]);
  });
  it("labels the kind and age", () => {
    const r = inboxRows(asks([ask()]), groups({ yourMove: [move()] }), NOW);
    expect(r.waiting[0]).toMatchObject({ chip: "Decide", age: "5 h", project: "Clavain" });
    expect(r.waiting[1]).toMatchObject({ chip: "Run a script", age: "2 h" });
  });
});

describe("MobileInbox", () => {
  it("opens on Waiting with one row per card, the count in the title and three tabs at tap height", () => {
    const html = view(asks([ask()]), groups({ yourMove: [move()], later: [move({ task_id: "m2" })] }));
    expect(html).toContain("Waiting on you · 2");
    expect(html).toContain('data-inbox-row="ask:d1"');
    expect(html).toContain('data-inbox-row="move:m1:1"');
    expect(html).not.toContain("move:m2:1");
    for (const t of ["waiting", "later", "since"]) expect(html.match(new RegExp(`<button[^>]*data-inbox-tab="${t}"[^>]*>`))![0]).toContain("min-h-14");
    expect(html).toContain("Later 1");
    expect(html).toContain("Since you left 3");
    expect(html).toContain("data-inbox-desktop");
  });
  it("says so when nothing waits", () => {
    expect(view(asks([]), null)).toContain("Nothing is waiting on you.");
  });
  it("Since you left shows the page's catch-up body", () => {
    const html = view(asks([]), null, { tab: "since" });
    expect(html).toContain("since body");
    expect(html).toContain("Since you left");
  });
  it("a decision opens full-screen with the whole card and a bar: Later and the recommended option", () => {
    const html = view(asks([ask()]), null, { open: "ask:d1" });
    expect(html).toContain('data-inbox-detail="ask:d1"');
    expect(html).toContain('data-option="b"'); // every option stays on the card
    expect(html).toContain("data-inbox-back");
    expect(html).not.toContain("data-inbox-tabs");
    expect(html.match(/<button[^>]*data-bar-later[^>]*>[^<]*/)![0]).toMatch(/Later$/);
    expect(html.match(/<button[^>]*data-bar-main[^>]*>[^<]*/)![0]).toMatch(/A: no change$/);
  });
  it("with no recommendation the bar offers only Later and points at the options", () => {
    const html = view(asks([ask({ ask: { question: "?", options: [{ id: "a", label: "A", kind: "ruling-only" }] } })]), null, { open: "ask:d1" });
    expect(html).not.toContain("data-bar-main");
    expect(html).toContain("Choose an option above.");
  });
  it("never makes a destructive option the one big button", () => {
    const html = view(asks([ask({ ask: { question: "?", recommendation: "a", options: [{ id: "a", label: "Delete it", kind: "instruction", instruction: "Delete it.", reversible: false }] } })]), null, { open: "ask:d1" });
    expect(html).not.toContain("data-bar-main");
  });
  it("a Later card's bar says Move back; a script move's bar says what mk says about himself", () => {
    expect(view(asks([ask({ later: LATER })]), null, { tab: "later", open: "ask:d1" }).match(/<button[^>]*data-bar-later[^>]*>[^<]*/)![0]).toMatch(/Move back$/);
    const html = view(asks([]), groups({ yourMove: [move()] }), { open: "move:m1:1" });
    expect(html.match(/<button[^>]*data-bar-main[^>]*>[^<]*/)![0]).toMatch(/I ran it$/);
  });
  it("a move on a decision's card shows under the decision in the detail view", () => {
    const html = view(asks([ask({ task_id: "m1" })]), groups({ yourMove: [move()] }), { open: "ask:d1" });
    expect(html).toContain('data-move="m1"');
    expect(html).toContain("Run the bundle script");
  });
  it("a stale open key falls back to the list", () => {
    expect(view(asks([ask()]), null, { open: "ask:gone" })).not.toContain("data-inbox-detail");
  });
});
