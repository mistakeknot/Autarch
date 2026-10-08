import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({
  ThreadChat: (p: { threadId: string; variant?: string; permissionPolicy?: string }) => (
    <div data-testid="thread-chat" data-thread={p.threadId} data-variant={p.variant} data-policy={p.permissionPolicy} />
  ),
}));

import { BlocksPanel, BlocksRow, groupRows, QueueRefresher, type QueueRowView, type QueueView } from "../ui/blocks.js";
import { ageText, AsksPanel, optionEffect, currentAsk, isOldAsk, buildAsksView, PickController, pickOutcome, type AsksData } from "../ui/asks.js";
import { CatchupPanel, observeVisibility, SeenTracker, snapshotIds, type CatchupEntry } from "../ui/catchup.js";
import { MapPlaceholder } from "../ui/map-placeholder.js";
import { BindingsPanel, parseDelegationForm, SettingsPanel } from "../ui/settings.js";
import { layoutStack, stackReducer, type Panel, type StackState } from "../ui/stack.js";
import { TellVizier, ThreadPanel, VizierPanel } from "../ui/vizier.js";
import { RootRunSection, statusLine, type RootRunPanelView } from "../ui/rootrun.js";

const ask = (over: Record<string, unknown> = {}) => ({
  id: "dec1",
  project: "Autarch",
  thread: "thr-a",
  subject: "Ship it?",
  asker: "thread",
  filed_at: "2026-09-29T10:00:00Z",
  revision: "rev1",
  mentions: 0,
  ask: {
    question: "Which day?",
    recommendation: "day",
    options: [
      { id: "day", label: "Day", kind: "instruction", reversible: true, instruction: "Ship on the day <b>exactly</b> & tell mk" },
      { id: "wk", label: "Week", kind: "needs-context", reversible: false },
    ],
  },
  ...over,
});
const data = (over: Partial<AsksData> = {}): AsksData => ({
  owed: [],
  runbook: [],
  lane: [],
  asks: [],
  undeliverable: [],
  failures: [],
  uncertain: [],
  delegation: { settings: {}, suspended: false },
  machineOwners: {},
  ...over,
});

describe("Asks ordering", () => {
  it("puts operator-action items first: decide, runbook, then stalled, then waiting", () => {
    const view = buildAsksView(
      data({
        owed: [ask()],
        runbook: [{ thread: "thr-r", items: [{ id: "s1", subject: "Steps", question: "Do these", steps: ["a", "b"], filed_at: "x" }] }],
        lane: [{ id: "m1", subject: "Owned", thread: "thr-a", owner: "thr-o", detail: "d", updated_at: "x" }],
        asks: [{ id: "m2", subject: "Nobody owns", thread: "thr-a", owner: null, detail: "d", updated_at: "x", label: "unowned machine blocker" }],
        undeliverable: [{ id: "o1", decision_id: "dec1", kind: "ruling-wake", recipient: "thr-a" }],
      }),
    );
    expect(view.map((s) => s.key)).toEqual(["decide", "runbook", "stalled", "waiting"]);
    expect(view[2]!.items.map((i) => i.id)).toEqual(["undeliverable:o1", "m2"]);
  });

  it("omits empty sections", () => {
    expect(buildAsksView(data({ owed: [ask()] })).map((s) => s.key)).toEqual(["decide"]);
  });

  it("a held ask leaves the Decide queue and shows greyed under On hold with its reason and no pick buttons", () => {
    const html = renderToStaticMarkup(<AsksPanel data={data({ owed: [ask({ id: "live", subject: "Live one" }), ask({ id: "held1", subject: "Held one", held: { reason: "script superseded", by: "thr_viz", at: "2026-10-07T00:00:00.000Z" } })] })} onPick={() => {}} onOpen={() => {}} />);
    expect(html).toContain("On hold (1)");
    expect(html).toContain("On hold: script superseded");
    expect(html).toContain('data-held="held1"');
    expect(html).toContain("Decide (1)");
    const only = renderToStaticMarkup(<AsksPanel data={data({ owed: [ask({ id: "h", held: { reason: "r", by: "v", at: "t" } })] })} onPick={() => {}} onOpen={() => {}} />);
    expect(only).not.toContain("Nothing needs you");
    expect(only).not.toContain("data-decide-list");
  });

  it("shows the task key in the queue row and the item header, and nothing for an ask with no key", () => {
    const withKey = renderToStaticMarkup(<AsksPanel data={data({ owed: [ask({ key: "AUTA-24" })] })} onPick={() => {}} onOpen={() => {}} />);
    expect(withKey.match(/data-task-key="true">AUTA-24</g)?.length).toBe(2);
    const without = renderToStaticMarkup(<AsksPanel data={data({ owed: [ask({ key: null })] })} onPick={() => {}} onOpen={() => {}} />);
    expect(without).not.toContain("data-task-key");
  });

  it("renders the question, the mention count and one effect line per option, with no instruction box or kind label", () => {
    const html = renderToStaticMarkup(<AsksPanel data={data({ owed: [ask({ mentions: 2 })] })} onPick={() => {}} onOpen={() => {}} />);
    expect(html).toContain("Which day?");
    expect(html).toContain("also mentioned in 2 threads");
    expect(html).not.toContain("what the agent is told");
    expect(html).not.toContain("<details");
    expect(html).not.toContain("needs-context</span>");
    expect(html).toContain("Tells thr-a: Ship on the day &lt;b&gt;exactly&lt;/b&gt; &amp; tell mk");
    expect(html).toContain("Asks you for the missing context first, then tells thr-a. Cannot be undone.");
    expect(html.match(/data-effect/g)?.length).toBe(2);
    expect(html).toContain("sent to thr-a as written; the agent acts on it under its own permissions");
    expect(html).toContain("recommended");
  });

  it("optionEffect is never empty, only an irreversible option line says so, and a ruling-only option has no instruction footer", () => {
    expect(optionEffect({ kind: "ruling-only" }, "t")).toBe("Records your pick only; nothing is sent.");
    expect(optionEffect({ kind: "ruling-only", reversible: true }, "t")).toBe("Records your pick only; nothing is sent.");
    expect(optionEffect({ kind: "instruction", instruction: "   " }, "t")).toBe("Records your pick only; nothing is sent. Cannot be undone.");
    expect(optionEffect({ kind: "instruction", instruction: "First. Second." }, "t")).toBe("Tells t: First. Cannot be undone.");
    expect(optionEffect({ kind: "instruction", instruction: "x".repeat(300) }, "t").length).toBeLessThan(170);
    const html = renderToStaticMarkup(
      <AsksPanel data={data({ owed: [ask({ ask: { question: "Q?", options: [{ id: "a", label: "Not now", kind: "ruling-only" }, { id: "b", label: "Later", kind: "ruling-only" }] } })] })} onPick={() => {}} onOpen={() => {}} />,
    );
    expect(html).toContain("Records your pick only; nothing is sent.");
    expect(html).not.toContain("An instruction is sent");
  });

  it("lists every ask as a short row with project, age and recommendation, and shows the first in full", () => {
    const now = Date.parse("2026-10-06T10:00:00Z");
    const html = renderToStaticMarkup(
      <AsksPanel
        nowMs={now}
        data={data({ owed: [ask({ filed_at: "2026-10-06T07:00:00Z" }), ask({ id: "dec2", subject: "Old one", filed_at: "2026-10-01T10:00:00Z", ask: { question: "Second?", options: [{ id: "x", label: "X", kind: "ruling-only" }] } })] })}
        onPick={() => {}}
        onOpen={() => {}}
      />,
    );
    expect(html).toContain("Decide (2)");
    expect(html.match(/<li><button/g)?.length).toBe(2);
    expect(html).toContain(">Autarch</span>");
    expect(html).toContain("3 h");
    const metaClasses = [...html.matchAll(/<span data-ask-meta="true" class="([^"]*)"/g)].map((m) => m[1]);
    expect(metaClasses.length).toBe(2);
    for (const c of metaClasses) expect(c).not.toContain("truncate");
    expect(html).toMatch(/<span class="[^"]*\btruncate\b[^"]*" data-ask-project="true">Autarch<\/span>/);
    expect(html).toContain("rec: Day");
    expect(html.match(/data-old-ask/g)?.length).toBe(1);
    expect(html).toContain("Which day?");
    expect(html).not.toContain("Second?");
  });

  it("says the instruction boilerplate once per card, not once per option", () => {
    const two = ask({ ask: { question: "q", options: [{ id: "a", label: "A", kind: "instruction", instruction: "do a" }, { id: "b", label: "B", kind: "instruction", instruction: "do b" }] } });
    const html = renderToStaticMarkup(<AsksPanel data={data({ owed: [two] })} onPick={() => {}} onOpen={() => {}} />);
    expect(html.match(/acts on it under its own permissions/g)?.length).toBe(1);
  });

  it("shows paths, shas and URLs in an ask as code that can break anywhere", () => {
    const sha = "32109540d47aaaafb3628b9e62275477ca817942ef0ea3dcb0" + "0".repeat(14);
    const q = `Script: /srv/runs/walk-ward.sh, sha256 ${sha} (see https://github.com/o/r/pull/4). Feels right and/or not.`;
    const html = renderToStaticMarkup(<AsksPanel data={data({ owed: [ask({ ask: { question: q, options: [{ id: "x", label: "X", kind: "ruling-only" }] } })] })} onPick={() => {}} onOpen={() => {}} />);
    const refs = [...html.matchAll(/<code[^>]*data-ref="[^"]*">([^<]*)<\/code>/g)].map((m) => m[1]);
    expect(refs).toEqual(["/srv/runs/walk-ward.sh", sha, "https://github.com/o/r/pull/4"]);
  });

  it("ageText reads minutes, hours, then days", () => {
    const now = Date.parse("2026-10-06T10:00:00Z");
    expect(ageText("2026-10-06T09:55:00Z", now)).toBe("5 min");
    expect(ageText("2026-10-05T10:00:00Z", now)).toBe("24 h");
    expect(ageText("2026-10-02T10:00:00Z", now)).toBe("4 d");
    expect(ageText("garbage", now)).toBe("now");
    expect(ageText("2026-10-07T10:00:00Z", now)).toBe("now");
  });

  it("flags an ask as old only past three days", () => {
    const now = Date.parse("2026-10-06T10:00:00Z");
    expect(isOldAsk("2026-10-03T10:00:00Z", now)).toBe(false);
    expect(isOldAsk("2026-10-03T09:59:59Z", now)).toBe(true);
    expect(isOldAsk("2026-10-07T10:00:00Z", now)).toBe(false);
    expect(isOldAsk("garbage", now)).toBe(false);
  });

  it("shows the selected ask, and falls back to the first once it is no longer owed", () => {
    const owed = [{ id: "a" }, { id: "b" }];
    expect(currentAsk(owed, null)?.id).toBe("a");
    expect(currentAsk(owed, "b")?.id).toBe("b");
    expect(currentAsk(owed, "gone")?.id).toBe("a");
    expect(currentAsk([], "b")).toBeUndefined();
  });
});

describe("pick", () => {
  it("sends the rendered revision and reuses pick_id on retry; a 409 re-reads", async () => {
    let n = 0;
    const c = new PickController(() => `pk${++n}`);
    const calls: unknown[] = [];
    let mode: "throw" | "ok" | "409" = "throw";
    const rpc = async (req: unknown) => {
      calls.push(req);
      if (mode === "throw") throw new Error("network");
      return mode === "ok" ? { ok: true } : { ok: false, status: 409, error: "already ruled" };
    };
    const reread = vi.fn();
    await expect(c.send(rpc, { decision_id: "dec1", option_id: "day", revision: "rev1" }, reread)).rejects.toThrow("network");
    mode = "ok";
    await c.send(rpc, { decision_id: "dec1", option_id: "day", revision: "rev1" }, reread);
    expect(calls).toEqual([
      { decision_id: "dec1", option_id: "day", revision: "rev1", pick_id: "pk1" },
      { decision_id: "dec1", option_id: "day", revision: "rev1", pick_id: "pk1" },
    ]);
    // A new click after success gets a new id.
    mode = "409";
    await c.send(rpc, { decision_id: "dec1", option_id: "day", revision: "rev1" }, reread);
    expect((calls[2] as { pick_id: string }).pick_id).toBe("pk2");
    expect(reread).toHaveBeenCalledTimes(1);
    // After a 409 the id is forgotten too.
    await c.send(rpc, { decision_id: "dec1", option_id: "day", revision: "rev1" }, reread);
    expect((calls[3] as { pick_id: string }).pick_id).toBe("pk3");
  });
});

describe("stack", () => {
  const p = (id: string): Panel => ({ id, kind: "decision", title: id });
  const start: StackState = { panels: [], width: "third" };

  it("keeps the newest panel at 1/3 and collapses older ones to 34px spines", () => {
    let s = stackReducer(start, { type: "push", panel: p("a") });
    s = stackReducer(s, { type: "push", panel: p("b") });
    s = stackReducer(s, { type: "push", panel: p("c") });
    const l = layoutStack(s);
    expect(l.map((x) => [x.panel.id, x.collapsed, x.width])).toEqual([
      ["a", true, "34px"],
      ["b", true, "34px"],
      ["c", false, "33.333%"],
    ]);
    expect(l.map((x) => x.grow)).toEqual([false, false, true]);
  });

  it("[ and ] move between 1/4, 1/3 and 1/2; Esc closes the top panel; pushing an open panel moves it to the top", () => {
    let s = stackReducer(start, { type: "push", panel: p("a") });
    expect(layoutStack(stackReducer(s, { type: "width", key: "[" }))[0]!.width).toBe("25%");
    expect(layoutStack(stackReducer(s, { type: "width", key: "]" }))[0]!.width).toBe("50%");
    s = stackReducer(s, { type: "push", panel: p("b") });
    s = stackReducer(s, { type: "push", panel: p("a") });
    expect(s.panels.map((x) => x.id)).toEqual(["b", "a"]);
    s = stackReducer(s, { type: "close" });
    expect(s.panels.map((x) => x.id)).toEqual(["b"]);
    expect(stackReducer(stackReducer(s, { type: "close" }), { type: "close" }).panels).toEqual([]);
  });
});

describe("seen marker", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());
  const tracker = () => {
    const seen: string[] = [];
    return { t: new SeenTracker((id) => seen.push(id)), seen };
  };

  it("marks an item only after it has been expanded and active for 1 s", () => {
    const { t, seen } = tracker();
    t.setActive(true);
    t.setVisible("ruling:d1", true);
    t.expand("ruling:d1");
    vi.advanceTimersByTime(999);
    expect(seen).toEqual([]);
    vi.advanceTimersByTime(2);
    expect(seen).toEqual(["ruling:d1"]);
  });

  it("collapsing before 1 s cancels; count-strip style items never mark", () => {
    const { t, seen } = tracker();
    t.setActive(true);
    t.setVisible("ruling:d1", true);
    t.expand("ruling:d1");
    vi.advanceTimersByTime(500);
    t.collapse("ruling:d1");
    vi.advanceTimersByTime(5000);
    expect(seen).toEqual([]);
  });

  it("with the document hidden nothing is marked; the timer pauses and resumes", () => {
    const { t, seen } = tracker();
    t.setActive(false);
    t.setVisible("ruling:d1", true);
    t.expand("ruling:d1");
    vi.advanceTimersByTime(10_000);
    expect(seen).toEqual([]);
    t.setActive(true);
    vi.advanceTimersByTime(600);
    t.setActive(false);
    vi.advanceTimersByTime(10_000);
    expect(seen).toEqual([]);
    t.setActive(true);
    vi.advanceTimersByTime(399);
    expect(seen).toEqual([]);
    vi.advanceTimersByTime(2);
    expect(seen).toEqual(["ruling:d1"]);
  });

  it("an item scrolled out of view stops its clock and is not visible to mark-all", () => {
    const { t, seen } = tracker();
    t.setActive(true);
    t.setVisible("ruling:d1", true);
    t.expand("ruling:d1");
    vi.advanceTimersByTime(600);
    t.setVisible("ruling:d1", false);
    expect(t.isVisible("ruling:d1")).toBe(false);
    vi.advanceTimersByTime(10_000);
    expect(seen).toEqual([]);
    t.setVisible("ruling:d1", true);
    vi.advanceTimersByTime(399);
    expect(seen).toEqual([]);
    vi.advanceTimersByTime(2);
    expect(seen).toEqual(["ruling:d1"]);
  });

  it("an item expanded while offscreen does not start its clock until it is seen", () => {
    const { t, seen } = tracker();
    t.setActive(true);
    t.setVisible("ruling:d1", false);
    t.expand("ruling:d1");
    vi.advanceTimersByTime(10_000);
    expect(seen).toEqual([]);
    t.setVisible("ruling:d1", true);
    vi.advanceTimersByTime(1001);
    expect(seen).toEqual(["ruling:d1"]);
  });

  it("an expanded routine group marks its members; an owed item is never marked", () => {
    const { t, seen } = tracker();
    t.setActive(true);
    t.setVisible("routine:Autarch", true);
    t.setVisible("owed:d9", true);
    t.expand("routine:Autarch", ["ruling:a", "closed:b"]);
    t.expand("owed:d9");
    vi.advanceTimersByTime(1100);
    expect(seen.sort()).toEqual(["closed:b", "ruling:a"]);
  });
});

describe("seen marker before the first observer report", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("an item is not visible until the observer reports it, and its clock does not run", () => {
    const seen: string[] = [];
    const t = new SeenTracker((id) => seen.push(id));
    t.setActive(true);
    expect(t.isVisible("ruling:d1")).toBe(false);
    t.expand("ruling:d1");
    vi.advanceTimersByTime(10_000);
    expect(seen).toEqual([]);
    t.setVisible("ruling:d1", true);
    expect(t.isVisible("ruling:d1")).toBe(true);
    vi.advanceTimersByTime(1001);
    expect(seen).toEqual(["ruling:d1"]);
  });

  it("an unreported item is excluded from mark all seen", () => {
    const t = new SeenTracker(() => {});
    const items: CatchupEntry[] = [{ item: "ruling:d1", kind: "delegated", at: "1", text: "x", decision: "d1" }];
    const vis = new Set(items.filter((c) => t.isVisible(c.item)).map((c) => c.item));
    expect(snapshotIds(items, new Set(["ruling:d1"]), vis)).toEqual([]);
  });
});

describe("observeVisibility", () => {
  class FakeIO {
    static all: FakeIO[] = [];
    disconnected = false;
    observed: unknown[] = [];
    constructor(public cb: (e: { isIntersecting: boolean }[]) => void) {
      FakeIO.all.push(this);
    }
    observe(el: unknown) {
      this.observed.push(el);
    }
    disconnect() {
      this.disconnected = true;
    }
  }
  beforeEach(() => {
    FakeIO.all = [];
    vi.stubGlobal("IntersectionObserver", FakeIO);
  });
  afterEach(() => vi.unstubAllGlobals());

  it("reports what the observer sees, and on cleanup disconnects and reports not visible", () => {
    const reports: [string, boolean][] = [];
    const stop = observeVisibility({} as Element, "ruling:d1", (i, v) => reports.push([i, v]));
    FakeIO.all[0]!.cb([{ isIntersecting: true }]);
    expect(reports).toEqual([["ruling:d1", true]]);
    stop();
    expect(FakeIO.all[0]!.disconnected).toBe(true);
    expect(reports.at(-1)).toEqual(["ruling:d1", false]);
  });

  it("a queued true delivered after cleanup is ignored and restarts no timer", () => {
    vi.useFakeTimers();
    try {
      const marked: string[] = [];
      const tr = new SeenTracker((id) => marked.push(id));
      tr.setActive(true);
      tr.expand("ruling:d1");
      const stop = observeVisibility({} as Element, "ruling:d1", (i, v) => tr.setVisible(i, v));
      FakeIO.all[0]!.cb([{ isIntersecting: true }]);
      vi.advanceTimersByTime(400);
      stop();
      FakeIO.all[0]!.cb([{ isIntersecting: true }]); // queued entry delivered late
      expect(tr.isVisible("ruling:d1")).toBe(false);
      expect(vi.getTimerCount()).toBe(0);
      vi.advanceTimersByTime(5000);
      expect(marked).toEqual([]);
    } finally {
      vi.useRealTimers();
    }
  });

  it("without IntersectionObserver the item is reported visible", () => {
    vi.unstubAllGlobals();
    vi.stubGlobal("IntersectionObserver", undefined);
    const reports: [string, boolean][] = [];
    const stop = observeVisibility({} as Element, "ruling:d1", (i, v) => reports.push([i, v]));
    expect(reports).toEqual([["ruling:d1", true]]);
    stop();
    expect(reports.at(-1)).toEqual(["ruling:d1", false]);
  });
});

describe("catch-up panel", () => {
  const items: CatchupEntry[] = [
    { item: "ruling:d1", kind: "delegated", at: "1", text: "The vizier ruled on Ship it?", decision: "d1" },
    { item: "routine:Autarch", kind: "routine", at: "2", text: "2 routine updates in Autarch", project: "Autarch", members: ["ruling:a", "ruling:b"] },
    { item: "owed:d2", kind: "owed", at: "3", text: "Waiting for you: X", decision: "d2" },
    { item: "note:n1", kind: "note", at: "4", text: "vizier's note: fine", cites: ["ruling:d1"] },
  ];

  it("a delegated ruling shows Override and stays pinned while unseen", () => {
    const html = renderToStaticMarkup(<CatchupPanel items={items} expanded={new Set()} onToggle={() => {}} onOverride={() => {}} onMarkAll={() => {}} />);
    expect(html).toContain("The vizier ruled on Ship it?");
    expect(html).toContain("Override");
    expect(html).toContain("mark all seen");
    expect(html).toContain("vizier&#x27;s note");
  });

  it("mark all seen sends only the ids of expanded, visible items", () => {
    const ids = snapshotIds(items, new Set(["ruling:d1", "routine:Autarch", "owed:d2"]), new Set(["ruling:d1", "routine:Autarch", "owed:d2", "note:n1"]));
    expect(ids.sort()).toEqual(["ruling:a", "ruling:b", "ruling:d1"]);
    expect(snapshotIds(items, new Set(["ruling:d1"]), new Set())).toEqual([]);
    expect(snapshotIds(items, new Set(), new Set(["ruling:d1"]))).toEqual([]);
  });
});

describe("vizier, thread, settings and map", () => {
  it("the vizier panel embeds the full ThreadChat that inherits permissions", () => {
    const html = renderToStaticMarkup(<VizierPanel threadId="thr-v" />);
    expect(html).toContain('data-thread="thr-v"');
    expect(html).toContain('data-variant="full"');
    expect(html).toContain('data-policy="inherit"');
    expect(renderToStaticMarkup(<VizierPanel threadId={undefined} />)).toContain("No vizier thread");
    expect(renderToStaticMarkup(<ThreadPanel threadId="thr-a" />)).toContain('data-variant="compact"');
  });

  it("settings show suspension and validate the delegation form", () => {
    const html = renderToStaticMarkup(
      <SettingsPanel delegation={{ settings: { vizierThreadId: "thr-v", projects: ["Autarch"], dailyCap: 5 }, suspended: true }} machineOwners={{ "test-host": "thr-o" }} onSave={() => {}} />,
    );
    expect(html).toContain("delegation suspended until you see this change");
    expect(html).toContain("test-host");
    expect(parseDelegationForm({ vizierThreadId: " thr-v ", projects: "Autarch, Other", dailyCap: "3" })).toEqual({ ok: true, value: { vizierThreadId: "thr-v", projects: ["Autarch", "Other"], dailyCap: 3 } });
    expect(parseDelegationForm({ vizierThreadId: "", projects: "", dailyCap: "3" }).ok).toBe(false);
    expect(parseDelegationForm({ vizierThreadId: "t", projects: "", dailyCap: "-1" }).ok).toBe(false);
  });

  it("the map is a labelled placeholder with the four lenses", () => {
    const html = renderToStaticMarkup(<MapPlaceholder lens="attention" onLens={() => {}} />);
    expect(html).toContain("placeholder");
    for (const l of ["attention", "allocation", "dependencies", "neglect"]) expect(html).toContain(l);
  });
});

const NOW = Date.parse("2026-10-01T00:00:00.000Z");
const row = (o: Partial<QueueRowView> = {}): QueueRowView => ({
  id: "d1", decision_id: "d1", task_id: "t1", card_key: "k1", binding_state: "confirmed", project: "Autarch", title: "Which order?",
  refs: [{ ref: "bead:a", counted: true }], blocks_count: 1, created_at: "2026-09-28T00:00:00.000Z", thread: "thr-a", pinned: false,
  ask: { question: "Which order?", options: [{ id: "a", label: "A" }, { id: "b", label: "B" }] } as never, revision: "rv1", mentions: 0,
  display_reason: null, display_only: false, overrides_generation: null, changed_after_ruling: false, root: { state: "verified", reason: null }, ...o,
});
const emptyLegacy = { count: 0, owed: [], runbook: [], machine: { lane: [], asks: [] } };
const q = (rows: QueueRowView[], legacy = emptyLegacy): QueueView => ({ rows, legacy, bindings: [], inactive_projects: [] });
const panel = (v: QueueView, thread?: string) => renderToStaticMarkup(<BlocksPanel data={v} nowMs={NOW} {...(thread ? { thread } : {})} onPick={() => {}} onOpen={() => {}} />);

describe("blocks panel", () => {
  it("renders rows in server order and pins this thread's cards in their own section first", () => {
    const html = panel(q([row({ id: "p", pinned: true, title: "pinned one" }), row({ id: "r", title: "rest one" })]), "thr-a");
    expect(html.indexOf('data-section="this-thread"')).toBeLessThan(html.indexOf('data-section="blocking"'));
    expect(html).toContain('data-row="p" data-pinned="true"');
    expect(html).toContain('data-row="r" data-pinned="false"');
    expect(groupRows([row({ id: "x", pinned: true }), row({ id: "y" })]).pinned.map((r) => r.id)).toEqual(["x"]);
  });
  it("shows age, the Blocks count, and which refs count", () => {
    const html = panel(q([row({ refs: [{ ref: "bead:a", counted: true }, { ref: "ticket:1", counted: false }], blocks_count: 1 })]));
    expect(html).toContain("age 3 d");
    expect(html).toContain("owner thr-a");
    expect(html).toContain("blocks 1");
    expect(html).toContain('data-counted="false"');
  });
  it("a row with no asking thread says the owner is unknown", () => {
    const html = renderToStaticMarkup(<BlocksRow row={row({ thread: null })} nowMs={NOW} onPick={() => {}} onOpen={() => {}} />);
    expect(html).toContain("owner unknown");
  });
  it("a free-form card is display-only with its reason and no pick buttons", () => {
    const html = renderToStaticMarkup(<BlocksRow row={row({ id: "card:t2", decision_id: null, ask: null, revision: null, display_only: true, display_reason: "no home-ask block", title: "Prose card" })} nowMs={NOW} onPick={() => {}} onOpen={() => {}} />);
    expect(html).toContain('data-display-only="true"');
    expect(html).toContain("display only: no home-ask block");
    expect(html).toContain("Prose card");
    expect(html).not.toContain("<button type=\"button\" data-option");
  });
  it.each([
    ["invalid home-ask JSON: Unexpected token"],
    ["duplicate Request key k1"],
    ["project mismatch: Other"],
    ["asking thread is not in this project"],
    ["changed after ruling"],
  ])("shows the display reason %s", (reason) => {
    const html = renderToStaticMarkup(<BlocksRow row={row({ decision_id: null, ask: null, revision: null, display_only: true, display_reason: reason })} nowMs={NOW} onPick={() => {}} onOpen={() => {}} />);
    expect(html).toContain(`display only: ${reason}`);
  });
  it("shows an unverified root with its reason", () => {
    const html = renderToStaticMarkup(<BlocksRow row={row({ root: { state: "unverified", reason: "sha mismatch" } })} nowMs={NOW} onPick={() => {}} onOpen={() => {}} />);
    expect(html).toContain('data-root="unverified"');
    expect(html).toContain("sha mismatch");
  });
  it("marks an override generation and a changed-after-ruling card", () => {
    expect(renderToStaticMarkup(<BlocksRow row={row({ overrides_generation: 2 })} nowMs={NOW} onPick={() => {}} onOpen={() => {}} />)).toContain("overrides vizier ruling g2");
    expect(renderToStaticMarkup(<BlocksRow row={row({ changed_after_ruling: true, display_only: true, display_reason: "changed after ruling" })} nowMs={NOW} onPick={() => {}} onOpen={() => {}} />)).toContain('data-marker="changed"');
  });
  it("renders the legacy group with a steps ask and a machine ask", () => {
    const legacy = {
      count: 2, owed: [],
      runbook: [{ thread: "thr-l", items: [{ id: "s1", subject: "steps subj", question: "do it", steps: ["one", "two"], filed_at: "2026-09-01T00:00:00.000Z" }] }],
      machine: { lane: [], asks: [{ id: "m1", subject: "ci down", thread: "thr-l", detail: "disk full", label: "unowned machine blocker" }] },
    } as never;
    const html = panel(q([], legacy));
    expect(html).toContain('data-section="legacy"');
    expect(html).toContain('data-legacy="steps"');
    expect(html).toContain("<li>one</li>");
    expect(html).toContain('data-legacy="machine"');
    expect(html).toContain("unowned machine blocker: ci down");
    expect(html).toContain("disk full");
  });
  it("says so when nothing blocks", () => {
    expect(panel(q([]))).toContain("Nothing is blocking");
  });
});

describe("queue refresh", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());
  it("refetches on the realtime event and on the fallback interval, and stops on cleanup", () => {
    const refetch = vi.fn();
    const r = new QueueRefresher(refetch, 30_000);
    const stop = r.start();
    r.onEvent();
    expect(refetch).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(30_000);
    expect(refetch).toHaveBeenCalledTimes(2);
    vi.advanceTimersByTime(60_000);
    expect(refetch).toHaveBeenCalledTimes(4);
    stop();
    vi.advanceTimersByTime(120_000);
    expect(refetch).toHaveBeenCalledTimes(4);
  });
});

describe("bindings settings (Q5)", () => {
  it("lists each binding with its state and offers Confirm on a suggested one only", () => {
    const html = renderToStaticMarkup(
      <BindingsPanel bindings={[{ tasks_project_id: "tp1", home_project: "Autarch", state: "suggested", suggested_at: null, confirmed_at: null }, { tasks_project_id: "tp2", home_project: "Other", state: "confirmed", suggested_at: null, confirmed_at: null }]} inactive={["Ghost"]} legacyCount={3} onBind={() => {}} />,
    );
    expect(html).toContain('data-binding="tp1" data-state="suggested"');
    expect(html).toContain("Inactive delegation projects");
    expect(html).toContain('data-legacy-count="3"');
    const tp2 = html.slice(html.indexOf('data-binding="tp2"'));
    expect(tp2).not.toContain("Confirm");
  });
});

describe("root-run section (Task 2.8)", () => {
  const view = (o: Partial<RootRunPanelView> = {}): RootRunPanelView => ({
    present: true, badge: "run by paste, not authenticated", state: "match", problems: [], tuple: { script: "/opt/run.sh", sha256: "a".repeat(64), timeout: 60, set: "s1" },
    actual_sha256: null, owner_thread: "thr_a", item_json: '{"run_as":"mk"}', command: "todo-add --set 's1' --from-card 'card-X.json'", card_file: "card-X.json", status: { kind: "none" }, ...o,
  });
  it("shows the badge, the command and no approval wording", () => {
    const html = renderToStaticMarkup(<RootRunSection view={view()} />);
    expect(html).toContain("run by paste, not authenticated");
    expect(html).toContain("todo-add --set");
    expect(html).toContain("no run record yet");
    expect(html).not.toMatch(/approv/i);
  });
  it("shows the hash-pinned command, the verified sha256, and that it works only once Aleph's slice is live", () => {
    const sha = "ab".repeat(32);
    const html = renderToStaticMarkup(<RootRunSection view={view({ tuple: { script: "/s.sh", sha256: sha, timeout: 60, set: "s1" }, command: `todo-add --set 's1' --from-card 'card-X.json' --expect-sha256 '${sha}'` })} />);
    expect(html).toContain('data-rootrun-pinned="true"');
    expect(html).toContain("run by paste, not authenticated");
    expect(html).toContain(`verified: ${sha}`);
    expect(html).toMatch(/only once Aleph/);
    expect(html).toContain(`--expect-sha256 &#x27;${sha}&#x27;`);
    expect(html).not.toContain("data-rootrun-unpinned");
    expect(html).not.toMatch(/does not pin/);
    const none = renderToStaticMarkup(<RootRunSection view={view({ command: null, item_json: null, card_file: null })} />);
    expect(none).not.toContain("data-rootrun-pinned");
  });
  it("shows the status paste command, and says status is not read automatically", () => {
    const html = renderToStaticMarkup(<RootRunSection view={view({ status: null, status_command: "todo-run --status 's1' 'bbtask-X-1'" })} />);
    expect(html).toContain("todo-run --status");
    expect(html).toContain("not read automatically");
    expect(html).toContain("status not read by Home");
  });
  it("shows the reason and no command when suppressed, and escapes hostile text", () => {
    const html = renderToStaticMarkup(<RootRunSection view={view({ state: "mismatch", command: null, item_json: null, problems: [{ field: "set", why: "<img src=x onerror=alert(1)>" }], tuple: { script: "/a/<b>.sh", sha256: "a".repeat(64), timeout: 1, set: "S" } })} />);
    expect(html).not.toContain("todo-add");
    expect(html).toContain('data-problem="set"');
    expect(html).not.toContain("<img");
    expect(html).not.toContain("<b>");
  });
  it("renders unavailable status and nothing when the card has no root-run block", () => {
    expect(statusLine({ kind: "unavailable", why: "x" })).toBe("status unavailable");
    expect(renderToStaticMarkup(<RootRunSection view={view({ present: false })} />)).toContain("no root-run block");
  });
  it("a run record renders its phase, exit and coded reason", () => {
    expect(statusLine({ kind: "run", attempt: 2, phase: "finalizing", terminal: "killed", exit: null, signal: "SIGKILL", reason: "supervisor-died", started: null, ended: null, log_complete: true, owner_alive: false })).toBe(
      "attempt 2, phase finalizing, ended: killed, signal SIGKILL, reason: the supervisor died, owner gone, log complete",
    );
  });
});

describe("unbound projects in settings", () => {
  it("lists a project with cards and no binding row, with a picker of serve projects and a Confirm", () => {
    const html = renderToStaticMarkup(
      <BindingsPanel bindings={[]} unbound={[{ tasks_project_id: "tp9", cards: 2, targets: ["Sylveste"] }]} serveProjects={["Autarch", "Sylveste"]} inactive={[]} legacyCount={0} onBind={() => {}} />,
    );
    expect(html).toContain('data-unbound="tp9"');
    expect(html).toContain("2 open cards, no binding; asks target Sylveste");
    expect(html).toMatch(/<option value="Sylveste"[^>]*>Sylveste<\/option>/);
    expect(html).toContain("Confirm");
    expect(html).not.toContain("No tasks project has been seen yet");
  });
});

describe("unbound picker default", () => {
  it("defaults to the asked-for project once serve lists it, and Confirm stays disabled until then", () => {
    const u = [{ tasks_project_id: "tp9", cards: 1, targets: ["Sylveste"] }];
    const down = renderToStaticMarkup(<BindingsPanel bindings={[]} unbound={u} serveProjects={[]} inactive={[]} legacyCount={0} onBind={() => {}} />);
    expect(down).toMatch(/<button[^>]*disabled[^>]*>Confirm/);
    const up = renderToStaticMarkup(<BindingsPanel bindings={[]} unbound={u} serveProjects={["Sylveste"]} inactive={[]} legacyCount={0} onBind={() => {}} />);
    expect(up).not.toMatch(/<button[^>]*disabled[^>]*>Confirm/);
  });
});

describe("pickOutcome: a failed pick is shown, never swallowed", () => {
  it("ok result is ok", async () => {
    expect(await pickOutcome(Promise.resolve({ ok: true }))).toEqual({ ok: true });
  });
  it("a thrown error becomes a visible failure", async () => {
    const r = await pickOutcome(Promise.reject(new Error("socket hang up")));
    expect(r.ok).toBe(false);
    expect(r.error).toContain("socket hang up");
  });
  it("a non-ok result carries its error, and a 409 says to reread", async () => {
    expect(await pickOutcome(Promise.resolve({ ok: false, error: "ruled already" }))).toEqual({ ok: false, error: "ruled already" });
    const r = await pickOutcome(Promise.resolve({ status: 409 }));
    expect(r.ok).toBe(false);
    expect(r.error).toMatch(/changed/);
  });
});

describe("project binding flag (fail open)", () => {
  const html = (over: Partial<QueueRowView>, onBind?: () => void) =>
    renderToStaticMarkup(<BlocksRow row={row({ tasks_project_id: "tp-1", project: "shadow-work", ...over })} nowMs={NOW} onPick={() => {}} onOpen={() => {}} {...(onBind ? { onBind } : {})} />);
  it("flags an unbound or only-suggested project, with a Bind button naming the Home project", () => {
    expect(html({ binding_state: null }, () => {})).toMatch(/data-binding-flag="unbound"[^>]*>project not bound.*Bind to shadow-work/);
    expect(html({ binding_state: "suggested" }, () => {})).toMatch(/data-binding-flag="suggested"/);
  });
  it("shows no flag for a confirmed or rejected binding, and no button without a handler", () => {
    expect(html({ binding_state: "confirmed" })).not.toMatch(/data-binding-flag/);
    expect(html({ binding_state: "rejected" })).not.toMatch(/data-binding-flag/);
    expect(html({ binding_state: null })).not.toMatch(/data-bind=/);
  });
});

describe("Tell the vizier box", () => {
  it("is a closed disclosure over the vizier thread, and says so when no vizier is set", () => {
    const html = renderToStaticMarkup(<TellVizier threadId={undefined} />);
    expect(html).toContain("data-tell-vizier");
    expect(html).toContain("Tell the vizier");
    expect(html).not.toContain("<details open");
    expect(html).toContain("No vizier thread is set");
  });
});
