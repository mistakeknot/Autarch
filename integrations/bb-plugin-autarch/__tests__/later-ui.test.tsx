// Later on screen (bead mk-8741): a one-tap Later on every open card that has a task, a "Later (N)" group below the
// active cards with a Move back button, and the count beside "Waiting on you", never in it.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { AsksPanel, AskCard, owedTasks, laterOutcome, runLater, LaterFailure, type AsksData, type OwedAsk } from "../ui/asks.js";
import { forgetClassicTabs, nextQueueTab, QueueTabs, RETIRED_CLASSIC_KEY } from "../ui/tabs.js";

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

  it("without a handler (the overlay) the group lists the cards and has no Move back button", () => {
    const html = renderToStaticMarkup(<AsksPanel data={data([ask({ later: LATER })])} onPick={noop as never} onOpen={noop} />);
    expect(html).toContain("Later (1)");
    expect(html).not.toContain("data-later-clear");
  });
});

describe("owedTasks", () => {
  it("owedTasks lists every task with an owed decision: Decide, Later and on hold", () => {
    const owed = [ask({ id: "a", task_id: "t1" }), ask({ id: "b", task_id: "t2", later: LATER }), ask({ id: "c", task_id: "t3", held: { reason: "r", by: "v", at: "x" } }), ask({ id: "d", task_id: null })];
    expect([...owedTasks(data(owed))].sort()).toEqual(["t1", "t2", "t3"]);
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
  });
});

describe("a Later card is one row", () => {
  it("title, Open card and Move back share the card's row; opening moves the card body to its own line", () => {
    const html = panel([ask({ id: "b", task_id: "task-2", later: LATER })]);
    const li = html.match(/<li[^>]*data-later="b"[^>]*>/)![0];
    expect(li).toContain("flex-wrap");
    expect(li).not.toMatch(/\bp-2\b/);
    expect(html).toMatch(/<details[^>]*class="[^"]*open:basis-full/);
    // DOM order is visual order (title, Move back, Open card), so focus order matches what is seen, closed or open.
    expect(html.indexOf("data-later-clear")).toBeLessThan(html.indexOf("Open card"));
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
    expect(later).not.toMatch(/Nothing else needs you now/);
    expect(panel([owed[1]!], { part: "later" })).toMatch(/Nothing else needs you now/);
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
  it("the tablist follows the W3C pattern: one tab stop, panel link, arrow, Home and End keys wrap", () => {
    const html = renderToStaticMarkup(<QueueTabs tab="later" onTab={noop} counts={null} />);
    expect(html.match(/tabindex="0"/g)?.length).toBe(1);
    expect(html.match(/tabindex="-1"/g)?.length).toBe(3);
    expect(html.match(/aria-controls="queue-tabpanel"/g)?.length).toBe(4);
    expect(nextQueueTab("decide", "ArrowRight")).toBe("later");
    expect(nextQueueTab("catchup", "ArrowRight")).toBe("decide");
    expect(nextQueueTab("decide", "ArrowLeft")).toBe("catchup");
    expect(nextQueueTab("later", "Home")).toBe("decide");
    expect(nextQueueTab("later", "End")).toBe("catchup");
    expect(nextQueueTab("later", "a")).toBeNull();
  });
});

describe("the retired Classic tabs key", () => {
  it("is cleared on load, and blocked or missing storage is not an error", () => {
    const removed: string[] = [];
    forgetClassicTabs(() => ({ removeItem: (k: string) => void removed.push(k) }));
    expect(removed).toEqual([RETIRED_CLASSIC_KEY]);
    expect(RETIRED_CLASSIC_KEY).toBe("home.classicTabs");
    expect(() => forgetClassicTabs(() => { throw new Error("SecurityError"); })).not.toThrow();
    expect(() => forgetClassicTabs(() => null)).not.toThrow();
  });
});
