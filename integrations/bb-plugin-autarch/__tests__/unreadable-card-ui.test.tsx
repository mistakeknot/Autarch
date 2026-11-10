// An unreadable card on the phone and the desktop (bead mk-j5ipa.14): it keeps its place, says "<KEY> could not be read",
// links to the card, has no answer buttons, and the good cards around it still show.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { DecideQueue, type AsksData, type OwedAsk } from "../ui/asks.js";
import { inboxRows, MobileInbox } from "../ui/inbox.js";
import { BlocksRow } from "../ui/blocks.js";

const NOW = Date.parse("2026-10-09T12:00:00Z");
const KEY = "AUTA-9999999999999999999999999999";
const good = (id: string, key: string, over: Partial<OwedAsk> = {}): OwedAsk => ({
  id,
  project: "Clavain",
  thread: "thr-a",
  subject: `Good ${id}`,
  key,
  asker: "card",
  filed_at: "2026-10-09T07:00:00Z",
  revision: "r1",
  task_id: `t-${id}`,
  ask: { question: "Pick one.", recommendation: "a", options: [{ id: "a", label: "A: go", kind: "ruling-only" }, { id: "b", label: "B: stop", kind: "ruling-only" }] },
  ...over,
});
const bad = (): OwedAsk => good("dbad", KEY, { subject: "Bad one", unreadable: true, ask: { question: "", options: [] } });
const owed = [good("d1", "AUTA-1"), bad(), good("d3", "AUTA-3")];
const asks = (o: OwedAsk[]): AsksData => ({ owed: o, runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {} });
const noop = () => {};

describe("phone", () => {
  const view = (open?: string) =>
    renderToStaticMarkup(<MobileInbox asks={asks(owed)} waiting={3} onPick={noop as never} onOpen={noop} onNote={noop} onLater={noop} sinceNode={null} sinceCount={0} onDesktop={noop} now={NOW} {...(open ? { initial: { open } } : {})} />);
  it("lists every card in place; the bad one reads '<KEY> could not be read' and shows no recommendation", () => {
    expect(inboxRows(asks(owed), NOW).waiting.map((r) => r.key)).toEqual(["ask:d1", "ask:dbad", "ask:d3"]);
    const html = view();
    expect(html).toContain(`${KEY} could not be read`);
    expect(html).toContain("Good d1");
    expect(html).toContain("Good d3");
    const bad = html.match(/<li[^>]*>(?:(?!<\/li>).)*dbad(?:(?!<\/li>).)*<\/li>/)![0];
    expect(bad).not.toContain("Rec:");
    expect(bad).toContain("[overflow-wrap:anywhere]");
  });
  it("its detail has the Open card link at tap height, no option or other buttons, and no main action in the bar", () => {
    const html = view("ask:dbad");
    expect(html).toContain(`${KEY} could not be read`);
    expect(html).toMatch(new RegExp(`<a[^>]*href="/plugins/autarch/home/${KEY}"[^>]*min-h-11[^>]*>Open card</a>|<a[^>]*min-h-11[^>]*href="/plugins/autarch/home/${KEY}"[^>]*>Open card</a>`));
    expect(html).not.toContain("data-bar-main");
    expect(html).not.toContain("data-option");
    expect(html).not.toContain("<textarea");
    expect(html).not.toContain("Choose an option above");
    expect(html).toContain("overflow-wrap:anywhere");
  });
});

describe("desktop", () => {
  it("the Decide list keeps the bad card's place and its card is the marked notice with the Open card link", () => {
    const html = renderToStaticMarkup(<DecideQueue owed={[owed[1]!, owed[0]!, owed[2]!]} onPick={noop as never} onOpen={noop} nowMs={NOW} />);
    expect(html).toContain(`${KEY} could not be read`);
    expect(html).toContain("Good d1");
    expect(html).toContain("min-h-11");
    expect(html).toContain(`href="/plugins/autarch/home/${KEY}"`);
    expect(html).not.toContain("data-option");
    expect(html).not.toContain("<textarea");
  });
  it("a good card selected beside the bad one still has its option buttons", () => {
    const html = renderToStaticMarkup(<DecideQueue owed={[owed[0]!, owed[1]!]} onPick={noop as never} onOpen={noop} nowMs={NOW} />);
    expect(html).toContain("A: go");
    expect(html).toContain(`${KEY} could not be read`);
  });
  it("the blocks panel row for a bad card is the marked notice, not a card with buttons", () => {
    const row = { id: "dbad", decision_id: "dbad", task_id: "t-dbad", card_key: KEY, binding_state: "confirmed", project: "Clavain", title: "Bad one", refs: [], blocks_count: 0, created_at: "2026-10-09T07:00:00Z", thread: "thr-a", pinned: false, ask: { question: "", options: [] }, revision: "r1", mentions: 0, display_reason: null, display_only: false, overrides_generation: null, changed_after_ruling: false, root: { state: "verified", reason: null }, unreadable: true };
    const html = renderToStaticMarkup(<BlocksRow row={row as never} nowMs={NOW} onPick={noop as never} onOpen={noop} />);
    expect(html).toContain(`${KEY} could not be read`);
    expect(html).toContain(`href="/plugins/autarch/home/${KEY}"`);
    expect(html).not.toContain("data-option");
  });
});
