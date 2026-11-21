// @vitest-environment jsdom
// Bead mk-j5ipa.15 (B7): "Not delivered yet: <subject>" items under Stalled, on the desktop list and in the phone's More waiting row.
// Read-only (no buttons), and never counted as a decision owed.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { AsksPanel, buildAsksView, type AsksData } from "../ui/asks.js";
import { DocReaderHost } from "../ui/docreader.js";
import { MoreWaitingRow, moreWaiting } from "../ui/morewaiting.js";
import { owedCounts } from "../waitingcount.js";
import { MobileInbox } from "../ui/inbox.js";

const base = (over: Partial<AsksData> = {}): AsksData => ({ owed: [], runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {}, ...over });
const nd = (n: number, over: Record<string, string> = {}) => ({ id: `undelivered:t${n}`, subject: `Card ${n}`, reason: "root unverified: serve is down", thread: "thr_a", ...over });

describe("desktop Stalled", () => {
  it("shows one Not delivered yet item with its reason and no action", () => {
    const stalled = buildAsksView(base({ notDelivered: [nd(1)] })).find((s) => s.key === "stalled")!;
    expect(stalled.items).toEqual([{ id: "undelivered:t1", title: "Not delivered yet: Card 1", detail: "root unverified: serve is down", thread: "thr_a", readOnly: true }]);
    expect(stalled.items[0]).not.toHaveProperty("dismiss");
  });
  it("shows nothing for zero, and every one of many, after the other stalled items", () => {
    expect(buildAsksView(base({ notDelivered: [] })).find((s) => s.key === "stalled")).toBeUndefined();
    expect(buildAsksView(base()).find((s) => s.key === "stalled")).toBeUndefined();
    const many = Array.from({ length: 40 }, (_, i) => nd(i));
    const s = buildAsksView(base({ uncertain: [{ id: "u1", kind: "wake", recipient: "x" }], notDelivered: many })).find((x) => x.key === "stalled")!;
    expect(s.items).toHaveLength(41);
    expect(s.items[0]!.id).toBe("uncertain:u1");
  });
  it("draws a file path in a subject or reason as plain text: no read or open button, even inside a reader host", () => {
    const item = nd(1, { subject: "Review /srv/example/m/docs/thr_a/plan.md", reason: "see /srv/example/m/docs/thr_a/why.md" });
    const html = renderToStaticMarkup(
      <DocReaderHost api={{ read: async () => ({ ok: false, error: "x" }), openFile: async () => ({ ok: true }) }}>
        <AsksPanel data={base({ notDelivered: [item] })} onPick={() => {}} onOpen={() => {}} />
      </DocReaderHost>,
    );
    expect(html).toContain("Not delivered yet: Review ");
    expect(html).toContain("/srv/example/m/docs/thr_a/plan.md");
    expect(html).not.toMatch(/data-doc-open|data-file-open|<button/);
  });
  it("does not count toward Waiting on you", () => {
    expect(owedCounts([])).toEqual({ pick: 0, held: 0, later: 0 });
    expect(buildAsksView(base({ notDelivered: [nd(1)] })).find((s) => s.key === "decide")).toBeUndefined();
  });
});

describe("phone More waiting", () => {
  it("lists it under Stalled and counts it in the row total", () => {
    const g = moreWaiting(base({ notDelivered: [nd(1), nd(2)] }));
    expect(g.total).toBe(2);
    expect(g.groups.map((x) => [x.key, x.count])).toEqual([["stalled", 2]]);
    expect(g.groups[0]!.items[0]!.title).toBe("Not delivered yet: Card 1");
  });
  it("copes with data that lacks the list, a long subject and a long reason, with no button", () => {
    expect(moreWaiting(base()).total).toBe(0);
    const long = nd(1, { subject: "S".repeat(300), reason: "r".repeat(600) });
    const g = moreWaiting(base({ notDelivered: [long] }));
    const html = renderToStaticMarkup(<MoreWaitingRow groups={g.groups} total={g.total} open onToggle={() => {}} />);
    expect(html).toContain("Not delivered yet: " + "S".repeat(300));
    expect(html).toContain("r".repeat(600));
    expect(html).toContain("[overflow-wrap:anywhere]");
    expect(html.match(/<button/g)).toHaveLength(1); // only the row's own toggle
  });
});

// An unreadable card (bead mk-j5ipa.14, an open decision) and a not-delivered card (this bead, still in observed) are different states.
// Both stay visible, each under its own wording, and neither takes the other's place.
describe("an unreadable card and a not-delivered card together", () => {
  const KEY = "AUTA-9999999999999999999999999999";
  const bad = { id: "dbad", project: "Clavain", thread: "thr-a", subject: "Bad one", key: KEY, asker: "card", filed_at: "2026-10-09T07:00:00Z", revision: "r1", task_id: "t-dbad", unreadable: true, ask: { question: "", options: [] } };
  const both = base({ owed: [bad as never], notDelivered: [nd(1)] });
  it("desktop: the unreadable card is in Decide, marked, and the not-delivered one is read-only under Stalled", () => {
    const view = buildAsksView(both);
    expect(view.find((s) => s.key === "stalled")!.items.map((i) => i.title)).toEqual(["Not delivered yet: Card 1"]);
    const html = renderToStaticMarkup(<AsksPanel data={both} onPick={() => {}} onOpen={() => {}} />);
    expect(html).toContain(`${KEY} could not be read`);
    expect(html).toContain("Not delivered yet: Card 1");
    expect(html).not.toContain(`Not delivered yet: ${KEY}`);
  });
  it("phone: the unreadable card keeps its row and the not-delivered one is counted in More waiting, not in the decisions", () => {
    expect(moreWaiting(both).total).toBe(1);
    const html = renderToStaticMarkup(<MobileInbox asks={both} waiting={1} onPick={(() => {}) as never} onOpen={() => {}} onNote={() => {}} onLater={() => {}} sinceNode={null} sinceCount={0} onDesktop={() => {}} now={Date.parse("2026-10-09T12:00:00Z")} />);
    expect(html).toContain(`${KEY} could not be read`);
  });
});
