// @vitest-environment jsdom
// The phone's "More waiting" row (bead mk-j5ipa.12): one closed row at the bottom of the phone inbox that, opened, lists
// Stalled, Steps for you, Waiting on others and Recorded approvals with their counts, read-only. Before it, the phone had
// no way to reach any of these four lists; the desktop showed them.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { MobileInbox } from "../ui/inbox.js";
import { moreWaiting } from "../ui/morewaiting.js";
import { filterAsks } from "../ui/projectfilterstore.js";
import type { AsksData, OwedAsk } from "../ui/asks.js";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const NOW = Date.parse("2026-10-09T12:00:00Z");
const noop = () => {};
const ask = (id: string, project = "Clavain", over: Partial<OwedAsk> = {}): OwedAsk => ({
  id,
  project,
  thread: "thr-a",
  subject: `Card ${id}`,
  asker: "thread",
  filed_at: "2026-10-09T07:00:00Z",
  revision: "r1",
  task_id: `t-${id}`,
  ask: { question: "Pick one.", recommendation: "a", options: [{ id: "a", label: "A", kind: "ruling-only" }, { id: "b", label: "B", kind: "ruling-only" }] },
  ...over,
});
const base = (over: Partial<AsksData> = {}): AsksData => ({ owed: [ask("d1")], runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {}, ...over });

/** One item in each of the four lists, five in all: two stalled (an undeliverable wake and a stalled ask). */
const full = (): AsksData =>
  base({
    undeliverable: [{ id: "u1", kind: "wake", recipient: "thr-x", last_error: "thread archived", decision_id: "d9" }],
    asks: [{ id: "s1", subject: "Deploy stuck", thread: "thr-s", owner: "thr-o", detail: "no progress for 2 d", updated_at: "2026-10-07T00:00:00Z", label: "stalled" }],
    runbook: [{ thread: "thr-r", items: [{ id: "r1", subject: "Run the install script", question: "Run it as root.", steps: ["bash /x"], filed_at: "2026-10-09T01:00:00Z" }] }],
    lane: [{ id: "l1", subject: "Review of PR 12", thread: "thr-l", owner: "thr-reviewer", detail: "review running", updated_at: "2026-10-09T10:00:00Z" }],
    approvals: [{ approval_id: "ap1", decision_id: "d8", kind: "merge", target: "org/repo#12", identity: "abc1234", minted_at: "2026-10-09T10:00:00Z", expires_at: "2026-10-10T10:00:00Z" }],
  });

const phone = (a: AsksData, initial?: { moreOpen?: boolean; holdOpen?: boolean; tab?: "waiting" | "later" | "since"; open?: string }, extra: Record<string, unknown> = {}) =>
  renderToStaticMarkup(<MobileInbox asks={a} waiting={1} onPick={noop as never} onOpen={noop} onNote={noop} onLater={noop} sinceNode={<p>since</p>} sinceCount={0} onDesktop={noop} now={NOW} {...(initial ? { initial } : {})} {...extra} />);

describe("the phone can reach the four lists", () => {
  it("(was red) a phone inbox with items in every list shows a More waiting row with the total", () => {
    expect(phone(full())).toMatch(/More waiting · 5/);
  });
});

describe("moreWaiting (the groups)", () => {
  it("builds Stalled, Steps for you, Waiting on others and Recorded approvals, in that order, with counts", () => {
    const g = moreWaiting(full());
    expect(g.total).toBe(5);
    expect(g.groups.map((x) => [x.key, x.title, x.count])).toEqual([
      ["stalled", "Stalled", 2],
      ["steps", "Steps for you", 1],
      ["waiting", "Waiting on others", 1],
      ["approvals", "Recorded approvals", 1],
    ]);
  });
  it("omits groups with nothing in them, and has a total of 0 when all are empty", () => {
    const g = moreWaiting(base({ lane: full().lane }));
    expect(g.groups.map((x) => x.key)).toEqual(["waiting"]);
    expect(g.total).toBe(1);
    expect(moreWaiting(base()).total).toBe(0);
    expect(moreWaiting(base()).groups).toEqual([]);
  });
  it("decisions owed are not counted: they are the Waiting and Later lists", () => {
    expect(moreWaiting(base({ owed: [ask("a"), ask("b"), ask("c")] })).total).toBe(0);
  });
  it("a list the data lacks counts as none, and the phone still draws", () => {
    const partial = { owed: [], settled: [], delegation: null, machineOwners: [] } as unknown as AsksData;
    expect(moreWaiting(partial)).toEqual({ total: 0, groups: [] });
    expect(phone(partial)).not.toContain("data-inbox-more");
  });
  it("a missing approvals list counts as none", () => {
    const a = base({ lane: full().lane });
    delete (a as { approvals?: unknown }).approvals;
    expect(moreWaiting(a).total).toBe(1);
  });
});

describe("the row, closed", () => {
  it("is closed by default: the count shows, the items do not", () => {
    const html = phone(full());
    expect(html).toContain("data-inbox-more");
    expect(html.match(/<button[^>]*data-inbox-more-toggle[^>]*>/)![0]).toContain('aria-expanded="false"');
    for (const hidden of ["Deploy stuck", "Run the install script", "Review of PR 12", "org/repo#12", "Undeliverable wake", "data-inbox-more-list"]) expect(html).not.toContain(hidden);
  });
  it("is a 44px-tall tap target", () => {
    expect(phone(full()).match(/<button[^>]*data-inbox-more-toggle[^>]*>/)![0]).toContain("min-h-11");
  });
  it("is left out when the four lists are empty, and the empty Waiting list still says so", () => {
    const html = phone(base({ owed: [] }));
    expect(html).not.toContain("data-inbox-more");
    expect(html).toContain("Nothing is waiting on you.");
  });
  it("shows on its own under the empty Waiting list", () => {
    const html = phone(base({ owed: [], lane: full().lane }));
    expect(html).toContain("Nothing is waiting on you.");
    expect(html).toMatch(/More waiting · 1/);
  });
  it("shows on the Waiting tab only, not on Later, Since you left or an open card", () => {
    expect(phone(full(), { tab: "later" })).not.toContain("data-inbox-more");
    expect(phone(full(), { tab: "since" })).not.toContain("data-inbox-more");
    expect(phone(full(), { open: "ask:d1" })).not.toContain("data-inbox-more");
  });
  it("sits below the On hold row", () => {
    const held = ask("h1", "Clavain", { held: { reason: "vendor", by: "v", at: "z" } });
    const html = phone(base({ owed: [ask("d1"), held], lane: full().lane }));
    expect(html.indexOf("data-inbox-hold")).toBeGreaterThan(-1);
    expect(html.indexOf("data-inbox-more")).toBeGreaterThan(html.indexOf("data-inbox-hold"));
  });
});

describe("the row, open", () => {
  const open = () => phone(full(), { moreOpen: true });
  it("lists the four groups with their counts and every item", () => {
    const html = open();
    expect(html.match(/<button[^>]*data-inbox-more-toggle[^>]*>/)![0]).toContain('aria-expanded="true"');
    for (const t of ["Stalled · 2", "Steps for you · 1", "Waiting on others · 1", "Recorded approvals · 1"]) expect(html).toContain(t);
    for (const t of ["Undeliverable wake to thr-x", "thread archived", "stalled: Deploy stuck", "Run the install script", "Run it as root.", "Review of PR 12", "review running", "merge org/repo#12 at abc1234"]) expect(html).toContain(t);
  });
  it("leaves out a group with nothing in it", () => {
    const html = phone(base({ lane: full().lane }), { moreOpen: true });
    expect(html).toContain("Waiting on others · 1");
    for (const t of ["Stalled", "Steps for you", "Recorded approvals"]) expect(html).not.toContain(t);
  });
  it("is read-only: no Dismiss, Revoke, pick, Later or other buttons inside the list", () => {
    const html = open();
    const from = html.indexOf("data-inbox-more-list");
    const list = html.slice(from, html.indexOf("data-inbox-tabs", from));
    expect(list).not.toContain("<button");
    expect(list).not.toMatch(/data-dismiss|data-confirming|Revoke|Dismiss|data-option|data-bar-|data-later|data-inbox-row/);
  });
  it("gives each group a heading and the approvals their no-authorisation note", () => {
    const html = open();
    expect(html.match(/<h2[^>]*>[^<]*(Stalled|Steps for you|Waiting on others|Recorded approvals)/g)).toHaveLength(4);
    expect(html).toContain("They do not authorize a merge, deploy or release.");
  });
  it("keeps Stalled items in one list a later change can add to (a single place builds them)", () => {
    expect(moreWaiting(full()).groups[0]!.items.map((i) => i.id)).toEqual(["undeliverable:u1", "s1"]);
  });
  it("one item, many items and a very long subject all render without clipping", () => {
    const many = Array.from({ length: 60 }, (_, i) => ({ id: `l${i}`, subject: `Lane ${i}`, thread: "t", owner: "o", detail: "d", updated_at: "2026-10-09T10:00:00Z" }));
    const html = phone(base({ lane: many }), { moreOpen: true });
    expect(html).toMatch(/More waiting · 60/);
    expect(html.match(/data-more-item=/g)).toHaveLength(60);
    const long = "a very long subject ".repeat(30) + "https://example.test/" + "x".repeat(200);
    const lh = phone(base({ lane: [{ id: "l1", subject: long, thread: "t", owner: "o", detail: "d", updated_at: "2026-10-09T10:00:00Z" }] }), { moreOpen: true });
    expect(lh).toContain("a very long subject");
    expect(lh).toMatch(/\[overflow-wrap:anywhere\]/);
  });
});

describe("the project filter", () => {
  const filtered = (excluded: string[]) => filterAsks(full(), new Set(excluded));
  it("the four lists carry no project, so a filter never hides them: the same row and items as with no filter", () => {
    const a = filtered(["Clavain"]);
    expect(a.owed).toHaveLength(0); // the filter did take the card off the list
    expect(moreWaiting(a)).toEqual(moreWaiting(full()));
    expect(phone(a, { moreOpen: true })).toContain("Deploy stuck");
  });
  it("the count follows the existing rule: it counts across all projects even while the filter hides cards", async () => {
    const store = await import("../ui/projectfilterstore.js");
    expect(store.COUNTS_FOLLOW_FILTER).toBe(false);
    expect(phone(filtered(["Clavain"]), undefined, { filter: { node: () => null, hidden: { waiting: 1, later: 0 }, laterCount: 0 } })).toMatch(/More waiting · 5/);
  });
});

describe("opening it (driven as a tap)", () => {
  let host: HTMLDivElement;
  let root: Root;
  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
    root = createRoot(host);
  });
  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });
  it("a tap opens it, a second tap closes it, and the Waiting list stays where it is", () => {
    act(() => root.render(<MobileInbox asks={full()} waiting={1} onPick={noop as never} onOpen={noop} onNote={noop} onLater={noop} sinceNode={null} sinceCount={0} onDesktop={noop} now={NOW} />));
    const toggle = () => host.querySelector<HTMLButtonElement>("[data-inbox-more-toggle]")!;
    expect(host.querySelector("[data-inbox-more-list]")).toBeNull();
    act(() => toggle().click());
    expect(toggle().getAttribute("aria-expanded")).toBe("true");
    expect(host.querySelector("[data-inbox-more-list]")!.textContent).toContain("Deploy stuck");
    expect(host.querySelector('[data-inbox-row="ask:d1"]')).not.toBeNull();
    act(() => toggle().click());
    expect(host.querySelector("[data-inbox-more-list]")).toBeNull();
  });
});
