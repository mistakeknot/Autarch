// The four Citadel UX rulings (bead mk-qrp5d): tap again to confirm, the Blocked tab count equals its rows, each
// Blocked row collapses to one line, and the Since you left count agrees with the Mark N seen button.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { ApprovalsList, AskCard, type OwedAsk } from "../ui/asks.js";
import { BlocksPanel, blocksCount, withoutOwed, type QueueRowView, type QueueView } from "../ui/blocks.js";
import { CatchupPanel, markedRows, markPlan, type CatchupEntry } from "../ui/catchup.js";
import { CONFIRM_LABEL, CONFIRM_MS, ConfirmTap, tapStep } from "../ui/confirmtap.js";
import { BindingsPanel } from "../ui/settings.js";
import { QueueTabs } from "../ui/tabs.js";

const noop = () => {};
const NOW = Date.parse("2026-10-01T00:00:00.000Z");

const ask = (options: OwedAsk["ask"]["options"], over: Partial<OwedAsk> = {}): OwedAsk => ({
  id: "dec1", project: "Autarch", thread: "thr-a", subject: "Ship it?", asker: "thread", filed_at: "2026-09-29T10:00:00Z", revision: "rev1", mentions: 0,
  ask: { question: "What now?", options },
  ...over,
});

describe("AUTA-454: tap again to confirm", () => {
  it("the first tap arms a destructive action and runs nothing; the second runs it and disarms", () => {
    expect(tapStep(false, true)).toEqual({ armed: true, run: false });
    expect(tapStep(true, true)).toEqual({ armed: false, run: true });
  });

  it("an action that needs no confirmation runs on the first tap and never arms", () => {
    expect(tapStep(false, false)).toEqual({ armed: false, run: true });
    expect(tapStep(true, false)).toEqual({ armed: false, run: true });
  });

  it("the window is about four seconds", () => {
    expect(CONFIRM_MS).toBe(4000);
  });

  const probe = (props: { needsConfirm: boolean; initialArmed?: boolean }) =>
    renderToStaticMarkup(
      <ConfirmTap {...props} onConfirmed={noop}>{({ armed, onClick, onBlur }) => <button type="button" onClick={onClick} onBlur={onBlur} data-armed={armed ? "true" : "false"}>{armed ? CONFIRM_LABEL : "Delete"}</button>}</ConfirmTap>,
    );

  it("starts disarmed with the option's own label", () => {
    const html = probe({ needsConfirm: true });
    expect(html).toContain("Delete");
    expect(html).toContain('data-armed="false"');
  });

  it("once armed the button says Tap again to confirm", () => {
    const html = probe({ needsConfirm: true, initialArmed: true });
    expect(html).toContain("Tap again to confirm");
    expect(html).toContain('data-armed="true"');
  });

  it("an action that needs no confirmation is never shown armed", () => {
    expect(probe({ needsConfirm: false, initialArmed: true })).toContain('data-armed="false"');
  });

  it("a destructive option on an AskCard starts as its own label; a reversible one is not wrapped in a confirm", () => {
    const html = renderToStaticMarkup(
      <AskCard ask={ask([{ id: "no", label: "Reject and close the PR", kind: "ruling-only", reversible: false }, { id: "yes", label: "Merge it", kind: "ruling-only", reversible: true }])} onPick={noop as never} onOpen={noop} />,
    );
    expect(html).toContain("Reject and close the PR");
    expect(html).toMatch(/data-tone="destructive"[^>]*data-option="no"|data-option="no"[^>]*data-tone="destructive"/);
    expect(html).not.toContain("data-confirming");
    expect(html).not.toContain(CONFIRM_LABEL);
  });

  it("Revoke and binding Reject start with their own labels", () => {
    const approvals = renderToStaticMarkup(<ApprovalsList approvals={[{ approval_id: "a1", decision_id: "d1", kind: "merge", target: "repo#9", identity: "abc", minted_at: "x", expires_at: "2026-10-08T10:00:00.000Z" }]} onRevoke={noop} nowMs={NOW} />);
    expect(approvals).toContain(">Revoke</button>");
    expect(approvals).not.toContain(CONFIRM_LABEL);
    const bindings = renderToStaticMarkup(<BindingsPanel bindings={[{ tasks_project_id: "tp1", home_project: "Autarch", state: "suggested", suggested_at: null, confirmed_at: null }]} inactive={[]} legacyCount={0} onBind={noop} />);
    expect(bindings).toContain(">Reject</button>");
    expect(bindings).toContain(">Confirm</button>");
    expect(bindings).not.toContain(CONFIRM_LABEL);
  });
});

const row = (o: Partial<QueueRowView> = {}): QueueRowView => ({
  id: "d1", decision_id: "d1", task_id: "t1", card_key: "K-1", binding_state: "confirmed", project: "Autarch", title: "Which order?",
  refs: [], blocks_count: 1, created_at: "2026-09-28T00:00:00.000Z", thread: "thr-a", pinned: false,
  ask: { question: "Which order?", options: [{ id: "a", label: "A", kind: "ruling-only" }, { id: "b", label: "B", kind: "ruling-only" }] } as never, revision: "rv1", mentions: 0,
  display_reason: null, display_only: false, overrides_generation: null, changed_after_ruling: false, root: { state: "verified", reason: null }, ...o,
});
const held = { reason: "waiting on a script", by: "thr_viz", at: "2026-10-07T00:00:00.000Z" };
const legacy = (count: number, owed: QueueView["legacy"]["owed"] = []): QueueView["legacy"] => ({ count, owed, runbook: [], machine: { lane: [], asks: [] } });
const q = (rows: QueueRowView[], l = legacy(0)): QueueView => ({ rows, legacy: l, bindings: [], inactive_projects: [] });

describe("AUTA-455: the Blocked on others count is the rows the tab lists", () => {
  const rows = [
    row({ id: "r1", decision_id: "d1" }),
    row({ id: "r2", decision_id: null, ask: null, revision: null, display_only: true, display_reason: "no home-ask block", title: "Display only" }),
    row({ id: "r3", decision_id: "d3", root: { state: "unverified", reason: "no root" } }),
    row({ id: "r4", decision_id: null, ask: null, revision: null, title: "On hold", held }),
  ];

  it("counts display-only, unbound and held rows, and the legacy asks", () => {
    expect(blocksCount(q(rows, legacy(2)))).toBe(6);
    expect(blocksCount(q([], legacy(0)))).toBe(0);
  });

  it("counts what is left after the asks above it are dropped", () => {
    const view = withoutOwed(q(rows, legacy(1)), { owed: [{ id: "d1" }] });
    expect(blocksCount(view)).toBe(4);
  });

  it("equals the rows the panel renders", () => {
    const view = q(rows, legacy(1, [{ id: "l1", project: "Autarch", thread: "thr-a", subject: "Old ask", filed_at: "2026-09-01T00:00:00Z", revision: "x", mentions: 0, ask: { question: "?", options: [] } }]));
    const html = renderToStaticMarkup(<BlocksPanel data={view} nowMs={NOW} onPick={noop as never} onOpen={noop} />);
    const rendered = (html.match(/data-row="/g) ?? []).length + (html.match(/data-decision="l1"/g) ?? []).length;
    expect(rendered).toBe(blocksCount(view));
  });

  it("is the number the tab shows", () => {
    const html = renderToStaticMarkup(<QueueTabs tab="decide" onTab={noop} counts={{ decide: 0, later: 0, blocks: blocksCount(q(rows, legacy(2))), catchup: 0 }} />);
    expect(html).toContain("Blocked on others (6)");
  });
});

describe("AUTA-456: each Blocked row collapses to one line", () => {
  const html = renderToStaticMarkup(<BlocksPanel data={q([row()])} nowMs={NOW} onPick={noop as never} onOpen={noop} />);

  it("shows the key and title as a summary with an Open card disclosure", () => {
    expect(html).toContain("data-row-card");
    expect(html).toMatch(/<summary[^>]*data-row-summary[^>]*>[\s\S]*K-1[\s\S]*Which order\?[\s\S]*Open card[\s\S]*<\/summary>/);
  });

  it("keeps the full card, with its options, inside the disclosure", () => {
    const inside = html.slice(html.indexOf("</summary>"), html.indexOf("</details>"));
    expect(inside).toContain('data-decision="d1"');
    expect(inside).toContain('data-option="a"');
  });

  it("keeps only the line outside the disclosure: meta, refs, flags and the root state are behind Open card", () => {
    const flagged = renderToStaticMarkup(<BlocksPanel data={q([row({ binding_state: null, refs: [{ ref: "K-9", counted: true }], overrides_generation: 2, changed_after_ruling: true })])} nowMs={NOW} onPick={noop as never} onOpen={noop} />);
    const outside = flagged.slice(0, flagged.indexOf("<details")) + flagged.slice(flagged.indexOf("</details>"));
    expect(outside).not.toContain("owner thr-a");
    expect(outside).not.toContain("data-refs");
    expect(outside).not.toContain("data-marker");
    expect(outside).not.toContain("data-binding-flag");
    expect(outside).not.toContain("data-root");
    const summary = flagged.slice(flagged.indexOf("<summary"), flagged.indexOf("</summary>"));
    expect(summary).toContain("data-row-flags");
    expect(summary).toMatch(/not bound, overrides, changed/);
    const inside = flagged.slice(flagged.indexOf("</summary>"), flagged.indexOf("</details>"));
    expect(inside).toContain("owner thr-a");
    expect(inside).toContain("data-binding-flag");
    expect(inside).toContain('data-marker="override"');
  });

  it("the summary is one truncated line", () => {
    const summary = html.slice(html.indexOf("<summary"), html.indexOf("</summary>"));
    expect(summary).toContain("truncate");
  });

  it("puts no card in front of a held or display-only row, which are already short", () => {
    const short = renderToStaticMarkup(
      <BlocksPanel data={q([row({ id: "h", decision_id: null, ask: null, revision: null, title: "Held", held }), row({ id: "o", decision_id: null, ask: null, revision: null, display_only: true, title: "Prose" })])} nowMs={NOW} onPick={noop as never} onOpen={noop} />,
    );
    expect(short).not.toContain("data-row-card");
    expect(short).toContain("On hold: waiting on a script");
  });
});

describe("AUTA-451: Since you left and Mark N seen count the same rows", () => {
  const items: CatchupEntry[] = [
    { item: "failed:1", kind: "failure", at: "1", text: "A ruling file failed" },
    { item: "routine:Autarch", kind: "routine", at: "2", text: "2 routine updates in Autarch", project: "Autarch", members: ["ruling:a", "ruling:b"], lines: ["a", "b"] },
    { item: "owed:d2", kind: "owed", at: "3", text: "Waiting for you: X", decision: "d2" },
    { item: "note:n1", kind: "note", at: "4", text: "a note" },
  ];
  const all = new Set(items.map((i) => i.item));
  // The server's tab count: every unseen row that is not an owed ask or a notice. A routine group is one row.
  const tabCount = items.filter((i) => i.kind !== "owed" && i.kind !== "notice").length;

  it("counts a routine group as one row, though marking it sends every member", () => {
    const plan = markPlan(items, new Set(["routine:Autarch"]), all);
    expect(plan.count).toBe(3);
    expect(plan.ids.sort()).toEqual(["failed:1", "note:n1", "ruling:a", "ruling:b"]);
    expect(plan.count).toBe(tabCount);
  });

  it("the button reads the same number as the tab when every row is on screen", () => {
    const html = renderToStaticMarkup(<CatchupPanel items={items} expanded={new Set(["routine:Autarch"])} visible={all} onToggle={noop} onOverride={noop} onMarkAll={noop} onMarkOne={noop} />);
    expect(html).toContain(`Mark ${tabCount} seen`);
    expect(html).not.toContain("Mark 4 seen");
  });

  it("a collapsed group is left out of both the count and the marking", () => {
    const plan = markPlan(items, new Set(), all);
    expect(plan.count).toBe(2);
    expect(plan.routineLeft).toBe(1);
  });

  it("the result after a click is said in rows too", () => {
    const plan = markPlan(items, new Set(["routine:Autarch"]), all);
    expect(markedRows(plan.rows, plan.ids)).toBe(3);
    expect(markedRows(plan.rows, ["ruling:a"])).toBe(1);
    expect(markedRows(plan.rows, [])).toBe(0);
  });
});
