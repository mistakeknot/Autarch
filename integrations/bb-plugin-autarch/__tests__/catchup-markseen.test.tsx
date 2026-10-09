// "Mark all seen" must say what it does (bead mk-yjp7): it marks what the panel showed in full on screen, says how
// many that is, and is never a silent no-op. Each unread row also has its own Mark seen.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CatchupPanel, markPlan, snapshotIds, type CatchupEntry } from "../ui/catchup.js";
import { NoticeBanner } from "../ui/notices.js";

const items: CatchupEntry[] = [
  { item: "failed:1", kind: "failure", at: "5", text: "A run failed on thread thr-a (request r1)." },
  { item: "ruling:d1", kind: "delegated", at: "1", text: "The vizier ruled on Ship it?", decision: "d1" },
  { item: "routine:Autarch", kind: "routine", at: "2", text: "2 routine updates in Autarch", project: "Autarch", members: ["ruling:a", "ruling:b"], lines: ["You chose Per day for Collapse?", "Closed: Old blocker"] },
  { item: "owed:d2", kind: "owed", at: "3", text: "Waiting for you: X", decision: "d2" },
  { item: "note:n1", kind: "note", at: "4", text: "vizier's note: fine", cites: ["ruling:d1"] },
];
const onScreen = new Set(items.map((i) => i.item));
const noop = () => {};

describe("what mark all seen marks", () => {
  it("nothing expanded: marks the rows shown in full on screen, never a collapsed routine group or an owed ask", () => {
    const plan = markPlan(items, new Set(), onScreen);
    expect(plan.ids.sort()).toEqual(["failed:1", "note:n1", "ruling:d1"]);
    expect(plan.count).toBe(3);
    expect(plan.routineLeft).toBe(1);
    expect(plan.reason).toBeNull();
    expect(snapshotIds(items, new Set(), onScreen).sort()).toEqual(["failed:1", "note:n1", "ruling:d1"]);
  });

  it("an expanded routine group is marked with its member lines showing", () => {
    const plan = markPlan(items, new Set(["routine:Autarch"]), onScreen);
    expect(plan.ids.sort()).toEqual(["failed:1", "note:n1", "ruling:a", "ruling:b", "ruling:d1"]);
    expect(plan.routineLeft).toBe(0);
  });

  it("a row off screen is not marked", () => {
    expect(markPlan(items, new Set(), new Set(["failed:1"])).ids).toEqual(["failed:1"]);
  });

  it("nothing on screen: zero, with the reason spelled out", () => {
    const plan = markPlan(items, new Set(), new Set());
    expect(plan.count).toBe(0);
    expect(plan.reason).toMatch(/on screen/i);
  });

  it("a notice is acknowledged, never marked by mark all", () => {
    const withNotice: CatchupEntry[] = [...items, { item: "vizier-adopted:3", kind: "notice", at: "6", text: "Home adopted thr_x" }];
    const plan = markPlan(withNotice, new Set(), new Set(withNotice.map((i) => i.item)));
    expect(plan.ids).not.toContain("vizier-adopted:3");
  });
});

describe("the catch-up panel controls", () => {
  const render = (over: Partial<Parameters<typeof CatchupPanel>[0]> = {}) =>
    renderToStaticMarkup(<CatchupPanel items={items} expanded={new Set()} visible={onScreen} onToggle={noop} onOverride={noop} onMarkAll={noop} onMarkOne={noop} {...over} />);

  it("the button says how many it will mark", () => {
    const html = render();
    expect(html).toContain("Mark 3 seen");
    expect(html).not.toContain("mark all seen");
  });

  it("with nothing markable the button is disabled and a visible reason is next to it", () => {
    const html = render({ visible: new Set() });
    expect(html).toMatch(/disabled=""[^>]*data-mark-all|data-mark-all[^>]*disabled=""/);
    expect(html).toMatch(/data-mark-reason[^>]*>[^<]*on screen/i);
  });

  it("each unread row has its own Mark seen, visible without expanding; an owed row has none", () => {
    const html = render();
    for (const id of ["failed:1", "ruling:d1", "note:n1"]) expect(html).toContain(`data-mark-one="${id}"`);
    expect(html).not.toContain('data-mark-one="owed:d2"');
  });

  it("a routine group lists its member lines only once opened, and only then offers Mark seen", () => {
    const closed = render();
    expect(closed).not.toContain("You chose Per day");
    expect(closed).not.toContain('data-mark-one="routine:Autarch"');
    const open = render({ expanded: new Set(["routine:Autarch"]) });
    expect(open).toContain("You chose Per day for Collapse?");
    expect(open).toContain('data-mark-one="routine:Autarch"');
  });

  it("the result of the last click is shown, never silent", () => {
    expect(render({ result: "Marked 3. 1 routine group left: open it to mark it." })).toContain("Marked 3. 1 routine group left");
  });

  it("the buttons are tap-sized on phones", () => {
    expect(render()).toContain("min-h-11");
  });
});

describe("the Needs your eyes banner", () => {
  const notice = { item: "vizier-adopted:3", at: "2026-10-07T12:00:00Z", text: "Home adopted thr_viz as the vizier thread; delegated rulings stay suspended until you see this." };

  it("states the consequence, the notice, and an explicit Acknowledge", () => {
    const html = renderToStaticMarkup(<NoticeBanner notices={[notice]} suspended onAcknowledge={noop} />);
    expect(html).toContain("Needs your eyes");
    expect(html).toContain("Delegated rulings are suspended");
    expect(html).toContain("Home adopted thr_viz");
    expect(html).toContain('data-acknowledge="vizier-adopted:3"');
    expect(html).toContain("Acknowledge");
  });

  it("renders nothing when there is nothing to acknowledge", () => {
    expect(renderToStaticMarkup(<NoticeBanner notices={[]} suspended={false} onAcknowledge={noop} />)).toBe("");
  });
});
