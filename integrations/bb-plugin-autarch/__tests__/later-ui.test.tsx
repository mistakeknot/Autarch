// Later on screen (bead mk-8741): a one-tap Later on every open card that has a task, a "Later (N)" group below the
// active cards with a Move back button, and the count beside "Waiting on you", never in it.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { AsksPanel, AskCard, owedTasks, laterOutcome, runLater, LaterFailure, type AsksData, type OwedAsk } from "../ui/asks.js";
import { MoveCard, moveButtons, omitDecisionPrMoves, omitLaterTasks, YourMovePanel } from "../ui/yourmove.js";
import type { MoveView } from "../moveview.js";
import { QueueTabs } from "../ui/tabs.js";
import { waitingSummary, WaitingStrip } from "../ui/waiting.js";
import type { Waiting } from "../waiting.js";

const ask = (over: Partial<OwedAsk> = {}): OwedAsk => ({
  id: "dec1",
  project: "Autarch",
  thread: "thr-a",
  subject: "Ship it?",
  asker: "thread",
  filed_at: "2026-09-29T10:00:00Z",
  revision: "rev1",
  mentions: 0,
  task_id: "task-1",
  ask: { question: "Which day?", options: [{ id: "day", label: "Day", kind: "ruling-only" }] },
  ...over,
});
const data = (owed: OwedAsk[]): AsksData => ({ owed, runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {} });
const noop = () => {};
const panel = (owed: OwedAsk[], extra: Partial<Parameters<typeof AsksPanel>[0]> = {}) => renderToStaticMarkup(<AsksPanel data={data(owed)} onPick={noop as never} onOpen={noop} onLater={noop} {...extra} />);
const LATER = { at: "2026-10-08T09:00:00.000Z", by: "mk" };

describe("Later button", () => {
  it("is on every open card that has a task, with a tap-height target, and says what it does", () => {
    const html = renderToStaticMarkup(<AskCard ask={ask()} onPick={noop as never} onOpen={noop} onLater={noop} />);
    expect(html).toContain('data-later-set="dec1"');
    const btn = html.match(/<button[^>]*data-later-set="dec1"[^>]*>/)![0];
    expect(btn).toContain("min-h-11");
    expect(html).toMatch(/stays open/i);
  });

  it("is absent on a card with no task, and when the surface has no handler", () => {
    expect(renderToStaticMarkup(<AskCard ask={ask({ task_id: null })} onPick={noop as never} onOpen={noop} onLater={noop} />)).not.toContain("data-later-set");
    expect(renderToStaticMarkup(<AskCard ask={ask()} onPick={noop as never} onOpen={noop} />)).not.toContain("data-later-set");
  });
});

describe("Later group", () => {
  it("moves a Later card out of Decide into 'Later (N)', below the active cards", () => {
    const html = panel([ask({ id: "a", subject: "Fast one" }), ask({ id: "b", subject: "QA session", task_id: "task-2", later: LATER })]);
    expect(html).toContain("Decide (1)");
    expect(html).toContain("Later (1)");
    expect(html.indexOf("Decide (1)")).toBeLessThan(html.indexOf("Later (1)"));
    // the Later card is not in the Decide list
    const decide = html.slice(html.indexOf('data-section="decide"'), html.indexOf('data-section="later"'));
    expect(decide).toContain("Fast one");
    expect(decide).not.toContain("QA session");
    const later = html.slice(html.indexOf('data-section="later"'));
    expect(later).toContain("QA session");
  });

  it("each Later card has a Move back button at tap height, and the card can still be ruled from there", () => {
    const html = panel([ask({ id: "b", subject: "QA session", task_id: "task-2", later: LATER })]);
    const btn = html.match(/<button[^>]*data-later-clear="b"[^>]*>/)![0];
    expect(btn).toContain("min-h-11");
    expect(html).toContain("Move back");
    expect(html).toContain("data-option=\"day\""); // the full card, so mk can rule it without moving it back first
    expect(html).not.toContain("data-later-set"); // inside Later the only toggle is Move back
  });

  it("says the group is still open and not in the Waiting count", () => {
    const html = panel([ask({ later: LATER })]);
    expect(html).toMatch(/not in .?Waiting on you/i);
  });

  it("only Later cards: not 'Nothing needs you', and no empty Decide section", () => {
    const html = panel([ask({ later: LATER })]);
    expect(html).not.toContain("Nothing needs you.");
    expect(html).not.toContain("Decide (");
    expect(html).toContain("Later (1)");
    expect(html).toMatch(/Nothing else needs you now/);
  });

  it("no Later cards: no Later section", () => {
    const html = panel([ask()]);
    expect(html).not.toContain('data-section="later"');
    expect(html).toContain('data-later-set="dec1"');
  });

  it("a held card stays under On hold even if it also carries a Later", () => {
    const html = panel([ask({ id: "h", subject: "Held one", later: LATER, held: { reason: "superseded", by: "thr_viz", at: "2026-10-08T00:00:00.000Z" } })]);
    expect(html).toContain("On hold (1)");
    expect(html).not.toContain('data-section="later"');
  });

  it("a Later card that also has a move keeps the move inside its card (plan link, read action)", () => {
    const html = panel([ask({ later: LATER })], { renderMoves: (t) => (t === "task-1" ? <p data-move-slot>I read it</p> : null) });
    const later = html.slice(html.indexOf('data-section="later"'));
    expect(later).toContain("data-move-slot");
    expect(later.match(/data-later-clear=/g)).toHaveLength(1);
  });

  it("without a handler (the overlay) the group lists the cards and has no Move back button", () => {
    const html = renderToStaticMarkup(<AsksPanel data={data([ask({ later: LATER })])} onPick={noop as never} onOpen={noop} />);
    expect(html).toContain("Later (1)");
    expect(html).not.toContain("data-later-clear");
  });
});

describe("Later beside the count", () => {
  const W: Waiting = { total: 3, decide: 3, moves: 0, notices: 0, noticeItems: [], updates: 0, held: 0, later: 2, suspended: false, definition: "Counts open decisions." };
  it("reads '2 for later' beside the total, never in it", () => {
    expect(waitingSummary(W)).toEqual({ headline: "Waiting on you: 3", parts: "3 to decide", beside: ["2 for later"] });
    const html = renderToStaticMarkup(<WaitingStrip waiting={W} onJump={noop} />);
    expect(html).toContain("Waiting on you: 3");
    expect(html).toContain("2 for later");
  });
  it("is absent at zero", () => {
    expect(waitingSummary({ ...W, later: 0 }).beside).toEqual([]);
  });
  it("sits after updates and holds, each with its own jump", () => {
    const jumps: string[] = [];
    const html = renderToStaticMarkup(<WaitingStrip waiting={{ ...W, updates: 1, held: 1 }} onJump={(t) => void jumps.push(t)} />);
    expect(waitingSummary({ ...W, updates: 1, held: 1 }).beside).toEqual(["1 update to read", "1 on hold", "2 for later"]);
    expect(html.indexOf("1 update to read")).toBeLessThan(html.indexOf("1 on hold"));
    expect(html.indexOf("1 on hold")).toBeLessThan(html.indexOf("2 for later"));
  });
});

describe("a move on a Later card", () => {
  const mv: MoveView = {
    task_id: "t1", generation: 1, kind: "read", state: "open", title: "Read the plan", owner: "thr-a", opened_at: "2026-10-06T10:00:00.000Z",
    claimed_at: null, skipped_at: null, card_later: true, checked_at: null, report_deadline_at: null, url: "https://x.test/doc", need: null, pr: null,
    script: null, commands: [], report: null, closed_at: null, closed_by: null, evidence: null,
  };
  const h = { onCheck: noop, onClaim: noop, onSkip: noop, onNote: noop };
  it("offers Move back at tap height (not a second Later / skip), and says the card is set aside", () => {
    const html = renderToStaticMarkup(<MoveCard m={mv} h={{ ...h, onUnlater: noop }} section="later" />);
    const btn = html.match(/<button[^>]*data-later-clear="t1"[^>]*>/)![0];
    expect(btn).toContain("min-h-11");
    expect(btn).toContain("sm:!min-h-11");
    expect(html).toContain("Move back");
    expect(html).not.toContain("Later / skip");
    expect(html).toContain("data-card-later");
    expect(moveButtons(mv).map((b) => b.label)).toEqual(["I read it"]);
  });
  it("without the handler there is no Move back", () => {
    expect(renderToStaticMarkup(<MoveCard m={mv} h={h} section="later" />)).not.toContain("data-later-clear");
  });
  it("a skipped move (no card-level Later) also gets Move back, so every move in Later can return", () => {
    const skipped = { ...mv, card_later: false, skipped_at: "2026-10-07T10:00:00.000Z" };
    const html = renderToStaticMarkup(<MoveCard m={skipped} h={{ ...h, onUnlater: noop }} section="later" />);
    expect(html).toContain('data-later-clear="t1"');
    expect(html).not.toContain("Later / skip");
  });
  it("a skipped context move, which has no other button, still has Move back", () => {
    const ctx = { ...mv, kind: "context" as const, url: null, need: "pick a date", card_later: false, skipped_at: "2026-10-07T10:00:00.000Z" };
    expect(moveButtons(ctx)).toEqual([]);
    expect(renderToStaticMarkup(<MoveCard m={ctx} h={{ ...h, onUnlater: noop }} section="later" />)).toContain('data-later-clear="t1"');
  });
});

describe("Your move panel split (the move Later group sits below Decide)", () => {
  const groups = (over: object) => ({ yourMove: [], reported: [], later: [], hidden: [], audit: [], ...over }) as never;
  const mv: MoveView = {
    task_id: "t1", generation: 1, kind: "read", state: "open", title: "Read the plan", owner: "thr-a", opened_at: "2026-10-06T10:00:00.000Z",
    claimed_at: null, skipped_at: null, card_later: true, checked_at: null, report_deadline_at: null, url: "https://x.test/doc", need: null, pr: null,
    script: null, commands: [], report: null, closed_at: null, closed_by: null, evidence: null,
  };
  const h = { onCheck: noop, onClaim: noop, onSkip: noop, onNote: noop, onUnlater: noop };
  it("part=active leaves the Later group out, part=later renders only it", () => {
    const d = groups({ yourMove: [{ ...mv, task_id: "a", card_later: false }], later: [mv] });
    const active = renderToStaticMarkup(<YourMovePanel data={d} handlers={h} part="active" />);
    const later = renderToStaticMarkup(<YourMovePanel data={d} handlers={h} part="later" />);
    expect(active).toContain('data-section="move-yourMove"');
    expect(active).not.toContain('data-section="move-later"');
    expect(later).toContain('data-section="move-later"');
    expect(later).not.toContain('data-section="move-yourMove"');
  });
  it("renders nothing for a part with nothing in it", () => {
    expect(renderToStaticMarkup(<YourMovePanel data={groups({ later: [mv] })} handlers={h} part="active" />)).toBe("");
    expect(renderToStaticMarkup(<YourMovePanel data={groups({ yourMove: [mv] })} handlers={h} part="later" />)).toBe("");
  });
  it("omitLaterTasks drops a task already shown in the Decide Later group, so a card is not listed twice", () => {
    const d = groups({ later: [mv, { ...mv, task_id: "t2" }], yourMove: [mv] });
    const out = omitLaterTasks(d, new Set(["t1"]));
    expect(out.later.map((m: MoveView) => m.task_id)).toEqual(["t2"]);
    expect(out.yourMove).toHaveLength(1); // only the Later group is deduplicated
  });
  it("omitDecisionPrMoves drops PR moves on cards with a decision from every group, and keeps other moves", () => {
    const pr = { ...mv, kind: "pr" as const };
    const script = { ...mv, kind: "script" as const, task_id: "t1", generation: 2 };
    const d = groups({ yourMove: [pr, script, { ...pr, task_id: "t2" }], reported: [pr], later: [pr], hidden: [pr] });
    const out = omitDecisionPrMoves(d, new Set(["t1"]));
    expect(out.yourMove.map((m: MoveView) => `${m.kind}:${m.task_id}`)).toEqual(["script:t1", "pr:t2"]);
    expect(out.reported).toHaveLength(0);
    expect(out.later).toHaveLength(0);
    expect(out.hidden).toHaveLength(1); // what mk hid stays in Hidden
    expect(renderToStaticMarkup(<YourMovePanel data={omitDecisionPrMoves(groups({ yourMove: [pr] }), new Set(["t1"]))} handlers={h} part="active" />)).toBe("");
  });
  it("omitDecisionPrMoves drops closed PR moves on cards with a decision from the Closed rows too", () => {
    const row = (kind: string, task_id: string) => ({ task_id, generation: 1, kind, closed_at: "2026-10-08T00:00:00.000Z", closed_by: "superseded", evidence: null });
    const d = groups({ audit: [row("pr", "t1"), row("script", "t1"), row("pr", "t2")] });
    const out = omitDecisionPrMoves(d, new Set(["t1"]));
    expect(out.audit.map((r: { kind: string; task_id: string }) => `${r.kind}:${r.task_id}`)).toEqual(["script:t1", "pr:t2"]);
  });
  it("owedTasks lists every task with an owed decision: Decide, Later and on hold", () => {
    const owed = [ask({ id: "a", task_id: "t1" }), ask({ id: "b", task_id: "t2", later: LATER }), ask({ id: "c", task_id: "t3", held: { reason: "r", by: "v", at: "x" } }), ask({ id: "d", task_id: null })];
    expect([...owedTasks(data(owed))].sort()).toEqual(["t1", "t2", "t3"]);
  });
  it("a held move shows its hold reason and no skip note", () => {
    const m = { ...mv, card_later: false, skipped_at: "2026-10-07T10:00:00.000Z", held: { reason: "script superseded", by: "thr_viz", at: "2026-10-08T00:00:00.000Z" } };
    const html = renderToStaticMarkup(<MoveCard m={m} h={h} section="yourMove" />);
    expect(html).toContain("data-held");
    expect(html).toContain("script superseded");
    expect(html).not.toContain("data-skipped");
  });
});

describe("tap targets at every width (44px, not just on phones)", () => {
  it("Later and Move back keep 44px at sm and wider, and so do the Open card summary and the strip jumps", () => {
    const html = panel([ask({ id: "a" }), ask({ id: "b", task_id: "task-2", later: LATER })]);
    for (const attr of ['data-later-set="a"', 'data-later-clear="b"']) {
      const btn = html.match(new RegExp(`<button[^>]*${attr}[^>]*>`))![0];
      expect(btn).toContain("min-h-11");
      expect(btn).toContain("sm:!min-h-11");
    }
    const summary = html.match(/<summary[^>]*>Open card/)![0];
    expect(summary).toContain("min-h-11");
    expect(summary).not.toContain("sm:min-h-0");
    const W: Waiting = { total: 1, decide: 1, moves: 0, notices: 0, noticeItems: [], updates: 1, held: 1, later: 1, suspended: false, definition: "x" };
    const strip = renderToStaticMarkup(<WaitingStrip waiting={W} onJump={noop} />);
    const jump = strip.match(/<button[^>]*>1 for later/)![0];
    expect(jump).toContain("sm:!min-h-11");
    expect(jump).not.toContain("sm:min-h-0");
  });
});

describe("every Your move action keeps 44px at sm and wider", () => {
  it("check, claim and Later / skip are tap height, not only Move back", () => {
    const mv = { task_id: "t1", generation: 1, kind: "script", subject: "s", key: null, need: null, pr: null, script: null, commands: [], report: null, url: null, held: null, claimed_at: null, skipped_at: null, card_later: false, checked_at: null, report_deadline_at: null, closed_at: null, closed_by: null, evidence: null } as unknown as Parameters<typeof MoveCard>[0]["m"];
    const html = renderToStaticMarkup(<MoveCard m={mv} h={{ onCheck: noop, onClaim: noop, onSkip: noop, onNote: noop }} section="yourMove" />);
    const actions = html.match(/<button[^>]*data-move-action="[a-z]+"[^>]*>/g) ?? [];
    expect(actions.length).toBeGreaterThanOrEqual(3);
    for (const b of actions) expect(b).toContain("sm:!min-h-11");
  });
});

describe("a failed Later or Move back is shown", () => {
  it("runLater turns a handler that throws into an error, and a retry clears it first", async () => {
    const seen: (string | null)[] = [];
    await runLater(async () => { throw new Error("offline"); }, "t", true, (e) => void seen.push(e));
    await runLater(async () => ({ ok: true }), "t", true, (e) => void seen.push(e));
    expect(seen).toEqual([null, "offline", null]);
  });
  it("laterOutcome: ok is ok; a not-ok result or a thrown error carries a reason", async () => {
    expect(await laterOutcome(Promise.resolve({ ok: true, was: false }))).toEqual({ ok: true });
    expect(await laterOutcome(Promise.resolve({ ok: false, error: "no such card" }))).toEqual({ ok: false, error: "no such card" });
    expect(await laterOutcome(Promise.resolve(null))).toMatchObject({ ok: false });
    expect(await laterOutcome(Promise.reject(new Error("offline")))).toEqual({ ok: false, error: "offline" });
  });
  it("runLater clears the error first, then sets it from a failed outcome, and tolerates a handler that returns nothing", async () => {
    const seen: (string | null)[] = [];
    await runLater(async () => ({ ok: false, error: "no such card" }), "t", true, (e) => void seen.push(e));
    expect(seen).toEqual([null, "no such card"]);
    const ok: (string | null)[] = [];
    await runLater(() => undefined, "t", false, (e) => void ok.push(e));
    await runLater(async () => ({ ok: true }), "t", false, (e) => void ok.push(e));
    expect(ok).toEqual([null, null]);
  });
  it("LaterFailure is an alert that says the card did not move and can be retried", () => {
    expect(renderToStaticMarkup(<LaterFailure error={null} />)).toBe("");
    const html = renderToStaticMarkup(<LaterFailure error="no such card" />);
    expect(html).toContain('role="alert"');
    expect(html).toContain("no such card");
    expect(html).toMatch(/try again/i);
  });
});

describe("Queue tabs", () => {
  const owed = [ask({ id: "live", subject: "Live one" }), ask({ id: "set-aside", subject: "Aside one", later: LATER })];
  it("the decide part shows the decision list and no Later section; the later part shows only Later", () => {
    const decide = panel(owed, { part: "decide" });
    expect(decide).toContain("Live one");
    expect(decide).not.toContain('data-section="later"');
    expect(decide).not.toContain("Aside one");
    const later = panel(owed, { part: "later" });
    expect(later).toContain('data-section="later"');
    expect(later).toContain("Aside one");
    expect(later).not.toContain('data-section="decide"');
    expect(later).not.toContain("Live one");
  });
  it("with no part both still show, as on the Asks page", () => {
    const both = panel(owed);
    expect(both).toContain('data-section="decide"');
    expect(both).toContain('data-section="later"');
  });
  it("says so when a tab is empty", () => {
    expect(panel([ask()], { part: "later" })).toContain("Nothing is set aside.");
    expect(panel([ask({ later: LATER })], { part: "decide" })).toContain("Nothing needs you.");
  });
  it("the tab row has the four tabs with their counts, the chosen one selected and tap height", () => {
    const html = renderToStaticMarkup(<QueueTabs tab="later" onTab={noop} counts={{ decide: 4, later: 3, blocks: 0, catchup: 26 }} />);
    for (const [id, label] of [["decide", "Needs you now (4)"], ["later", "Later (3)"], ["blocks", "Blocked on others (0)"], ["catchup", "Since you left (26)"]]) {
      expect(html).toMatch(new RegExp(`<button[^>]*data-queue-tab="${id}"[^>]*>${label.replace(/[()]/g, "\\$&")}</button>`));
    }
    expect(html.match(/aria-selected="true"/g)?.length).toBe(1);
    expect(html.match(/min-h-11/g)?.length).toBe(4);
    expect(renderToStaticMarkup(<QueueTabs tab="decide" onTab={noop} counts={null} />)).toContain(">Later</button>");
  });
});
