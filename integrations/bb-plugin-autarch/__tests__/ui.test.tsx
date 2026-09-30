import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({
  ThreadChat: (p: { threadId: string; variant?: string; permissionPolicy?: string }) => (
    <div data-testid="thread-chat" data-thread={p.threadId} data-variant={p.variant} data-policy={p.permissionPolicy} />
  ),
}));

import { AsksPanel, buildAsksView, PickController, type AsksData } from "../ui/asks.js";
import { CatchupPanel, SeenTracker, snapshotIds, type CatchupEntry } from "../ui/catchup.js";
import { MapPlaceholder } from "../ui/map-placeholder.js";
import { parseDelegationForm, SettingsPanel } from "../ui/settings.js";
import { layoutStack, stackReducer, type Panel, type StackState } from "../ui/stack.js";
import { ThreadPanel, VizierPanel } from "../ui/vizier.js";

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
  it("puts stalled first, then decide, then the runbook, then waiting", () => {
    const view = buildAsksView(
      data({
        owed: [ask()],
        runbook: [{ thread: "thr-r", items: [{ id: "s1", subject: "Steps", question: "Do these", steps: ["a", "b"], filed_at: "x" }] }],
        lane: [{ id: "m1", subject: "Owned", thread: "thr-a", owner: "thr-o", detail: "d", updated_at: "x" }],
        asks: [{ id: "m2", subject: "Nobody owns", thread: "thr-a", owner: null, detail: "d", updated_at: "x", label: "unowned machine blocker" }],
        undeliverable: [{ id: "o1", decision_id: "dec1", kind: "ruling-wake", recipient: "thr-a" }],
      }),
    );
    expect(view.map((s) => s.key)).toEqual(["stalled", "decide", "runbook", "waiting"]);
    expect(view[0]!.items.map((i) => i.id)).toEqual(["undeliverable:o1", "m2"]);
  });

  it("omits empty sections", () => {
    expect(buildAsksView(data({ owed: [ask()] })).map((s) => s.key)).toEqual(["decide"]);
  });

  it("renders the question, the mention count, each option with its kind and reversible mark, and the instruction exactly", () => {
    const html = renderToStaticMarkup(<AsksPanel data={data({ owed: [ask({ mentions: 2 })] })} onPick={() => {}} onOpen={() => {}} />);
    expect(html).toContain("Which day?");
    expect(html).toContain("also mentioned in 2 threads");
    expect(html).toContain("instruction");
    expect(html).toContain("needs-context");
    expect(html).toContain("reversible");
    expect(html.match(/data-reversible="true"/g)?.length).toBe(1);
    expect(html.match(/data-reversible="false"/g)?.length).toBe(1);
    expect(html).toContain("Ship on the day &lt;b&gt;exactly&lt;/b&gt; &amp; tell mk");
    expect(html).toContain("sent to thr-a as written; the agent acts on it under its own permissions");
    expect(html).toContain("recommended");
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
    t.expand("ruling:d1");
    vi.advanceTimersByTime(999);
    expect(seen).toEqual([]);
    vi.advanceTimersByTime(2);
    expect(seen).toEqual(["ruling:d1"]);
  });

  it("collapsing before 1 s cancels; count-strip style items never mark", () => {
    const { t, seen } = tracker();
    t.setActive(true);
    t.expand("ruling:d1");
    vi.advanceTimersByTime(500);
    t.collapse("ruling:d1");
    vi.advanceTimersByTime(5000);
    expect(seen).toEqual([]);
  });

  it("with the document hidden nothing is marked; the timer pauses and resumes", () => {
    const { t, seen } = tracker();
    t.setActive(false);
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

  it("an expanded routine group marks its members; an owed item is never marked", () => {
    const { t, seen } = tracker();
    t.setActive(true);
    t.expand("routine:Autarch", ["ruling:a", "closed:b"]);
    t.expand("owed:d9");
    vi.advanceTimersByTime(1100);
    expect(seen.sort()).toEqual(["closed:b", "ruling:a"]);
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
