// Fixes for the cross-lab review of the Home UX pass (bead mk-yjp7): legacy notices, fully-displayed marking,
// continued commands, prose false positives, and a waiting count that follows move actions.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { displayedTracker, isNoticeItem, markPlan, normalizeCatchup, SeenTracker, type CatchupEntry } from "../ui/catchup.js";
import { buildMoveHandlers } from "../movehandlers.js";
import { splitCommands, flagsOf } from "../ui/commandtext.js";
import { WaitingStrip } from "../ui/waiting.js";
import type { Waiting } from "../waiting.js";

describe("an old server's notices stay notices", () => {
  const legacy: CatchupEntry[] = [
    { item: "vizier-adopted:7", kind: "delegated", at: "1", text: "Home adopted thr_x as the vizier thread; delegated rulings stay suspended until you see this." },
    { item: "delegation-settings:3", kind: "delegated", at: "2", text: "Delegation settings changed." },
    { item: "ruling:d1", kind: "delegated", at: "3", text: "The vizier ruled on Ship it?", decision: "d1" },
  ];
  it("recognises them by id as well as by kind", () => {
    expect(isNoticeItem(legacy[0]!)).toBe(true);
    expect(isNoticeItem(legacy[1]!)).toBe(true);
    expect(isNoticeItem(legacy[2]!)).toBe(false);
    expect(isNoticeItem({ item: "x", kind: "notice" })).toBe(true);
  });
  it("normalizes legacy delegated notices to kind notice and leaves the rest", () => {
    const n = normalizeCatchup(legacy);
    expect(n.map((e) => e.kind)).toEqual(["notice", "notice", "delegated"]);
  });
  it("mark all never includes them, even un-normalized", () => {
    const plan = markPlan(legacy, new Set(), new Set(legacy.map((e) => e.item)));
    expect(plan.ids).toEqual(["ruling:d1"]);
  });
  it("dwell never marks them", () => {
    vi.useFakeTimers();
    const marked: string[] = [];
    const t = new SeenTracker((id) => marked.push(id));
    t.setActive(true);
    t.setVisible("vizier-adopted:7", true);
    t.expand("vizier-adopted:7");
    t.expand("delegation-settings:3");
    vi.advanceTimersByTime(5000);
    vi.useRealTimers();
    expect(marked).toEqual([]);
  });
});

describe("fully displayed", () => {
  const entry = (o: { ratio: number; top: number; bottom: number; rootTop?: number; rootBottom?: number; intersecting?: boolean }) =>
    ({
      isIntersecting: o.intersecting ?? o.ratio > 0,
      intersectionRatio: o.ratio,
      intersectionRect: { top: Math.max(o.top, o.rootTop ?? 0), bottom: Math.min(o.bottom, o.rootBottom ?? 800), height: o.ratio * (o.bottom - o.top) },
      boundingClientRect: { top: o.top, bottom: o.bottom, height: o.bottom - o.top },
      rootBounds: { top: o.rootTop ?? 0, bottom: o.rootBottom ?? 800, height: (o.rootBottom ?? 800) - (o.rootTop ?? 0) },
    }) as unknown as IntersectionObserverEntry;

  it("a row that fits counts only when all of it is in view", () => {
    const d = displayedTracker();
    expect(d(entry({ ratio: 0.01, top: 790, bottom: 890 }))).toBe(false);
    expect(d(entry({ ratio: 0.6, top: 600, bottom: 700 }))).toBe(false);
    expect(d(entry({ ratio: 1, top: 100, bottom: 200 }))).toBe(true);
    expect(d(entry({ ratio: 0.5, top: 750, bottom: 850 }))).toBe(false);
  });
  it("a row taller than the screen counts once both its top and its bottom have been in view", () => {
    const d = displayedTracker();
    // top in view, bottom far below
    expect(d(entry({ ratio: 0.3, top: 100, bottom: 2500 }))).toBe(false);
    // scrolled to the middle: neither edge in view
    expect(d(entry({ ratio: 0.33, top: -800, bottom: 1600 }))).toBe(false);
    // bottom edge now in view
    expect(d(entry({ ratio: 0.3, top: -1700, bottom: 700 }))).toBe(true);
  });
  it("leaving the screen forgets what was seen", () => {
    const d = displayedTracker();
    d(entry({ ratio: 0.3, top: 100, bottom: 2500 }));
    expect(d(entry({ ratio: 0, top: -3000, bottom: -500, intersecting: false }))).toBe(false);
    expect(d(entry({ ratio: 0.3, top: -1700, bottom: 700 }))).toBe(false); // bottom alone is not enough after the reset
  });
});

describe("continued commands are one command", () => {
  const text = "Run this:\nbash /tmp/install.sh \\\n  --apply --accept-install-diff\nthen check.";
  it("keeps the continuation lines in the command, byte for byte", () => {
    const cmds = splitCommands(text).filter((p) => p.type === "command");
    expect(cmds).toHaveLength(1);
    expect(cmds[0]!.text).toBe("bash /tmp/install.sh \\\n  --apply --accept-install-diff");
    expect(flagsOf(cmds[0]!.text)).toEqual(["--apply", "--accept-install-diff"]);
  });
  it("the text after it stays prose", () => {
    const parts = splitCommands(text);
    expect(parts[parts.length - 1]).toEqual({ type: "prose", text: "then check." });
  });
  it("a chain of continuations and a trailing backslash with nothing after it do not lose text", () => {
    const [c] = splitCommands("bash x.sh \\\n --a \\\n --b").filter((p) => p.type === "command");
    expect(c!.text).toBe("bash x.sh \\\n --a \\\n --b");
    const [d] = splitCommands("bash x.sh --a \\").filter((p) => p.type === "command");
    expect(d!.text).toBe("bash x.sh --a \\");
  });
});

describe("prose is not promoted to a command", () => {
  it("a sentence ending in a period is prose even with a flag in it", () => {
    expect(splitCommands("bb supports --json output.").every((p) => p.type === "prose")).toBe(true);
  });
  it("a path or file at the end keeps a command a command", () => {
    expect(splitCommands("bash /tmp/x.sh --out=/tmp/o.").some((p) => p.type === "command")).toBe(true);
    expect(splitCommands("bash x.sh --check").some((p) => p.type === "command")).toBe(true);
  });
  it("an explicit $ prompt accepts any command", () => {
    const [c] = splitCommands("$ echo ready").filter((p) => p.type === "command");
    expect(c!.text).toBe("echo ready");
    const [d] = splitCommands("$ systemctl --user restart foo.").filter((p) => p.type === "command");
    expect(d!.text).toBe("systemctl --user restart foo.");
  });
});

describe("the waiting count follows move actions", () => {
  const move = { task_id: "t1", generation: 2 } as never;
  const rpcFor = (calls: string[]) => ({ call: (m: string) => (calls.push(m), Promise.resolve({ ok: true })) }) as never;
  it("claim, skip and check refresh the moves and the totals", async () => {
    for (const [name, key] of [["claimMove", "onClaim"], ["skipMove", "onSkip"], ["checkMove", "onCheck"]] as const) {
      const calls: string[] = [];
      const moves = vi.fn();
      const totals = vi.fn();
      const h = buildMoveHandlers(rpcFor(calls), moves, totals, async () => ({ ok: true }));
      await h[key](move);
      expect(calls).toEqual([name]);
      expect(moves).toHaveBeenCalledTimes(1);
      expect(totals).toHaveBeenCalledTimes(1);
    }
  });
  it("still refreshes when the call fails", async () => {
    const moves = vi.fn();
    const totals = vi.fn();
    const h = buildMoveHandlers({ call: () => Promise.reject(new Error("down")) } as never, moves, totals, async () => ({ ok: true }));
    expect(await h.onClaim(move)).toMatchObject({ ok: false });
    expect(totals).toHaveBeenCalledTimes(1);
  });
});

describe("the definition is a tap target", () => {
  it("What this counts has a 44px minimum on a phone", () => {
    const w: Waiting = { total: 1, decide: 1, moves: 0, notices: 0, noticeItems: [], updates: 0, held: 0, suspended: false, definition: "d" } as never;
    const html = renderToStaticMarkup(<WaitingStrip waiting={w} onJump={() => {}} />);
    expect(html).toMatch(/<summary[^>]*class="[^"]*min-h-11[^"]*sm:min-h-0/);
  });
});

describe("second review: container height, changed content, unpunctuated prose", () => {
  it("a row's reading evidence is tied to its content, so a poll that adds members starts it over", async () => {
    const { rowContentKey } = await import("../ui/catchup.js");
    const g: CatchupEntry = { item: "routine:A", kind: "routine", at: "1", text: "2 routine updates in A", members: ["ruling:a", "ruling:b"], lines: ["x", "y"] };
    expect(rowContentKey(g)).toBe(rowContentKey({ ...g }));
    expect(rowContentKey(g)).not.toBe(rowContentKey({ ...g, text: "3 routine updates in A", members: [...g.members!, "ruling:c"], lines: [...g.lines!, "z"] }));
  });
  it("the observer's root is the scroll container the row lives in, not the whole window", async () => {
    const { scrollParent } = await import("../ui/catchup.js");
    expect(scrollParent({} as Element)).toBeNull();
    vi.stubGlobal("getComputedStyle", (el: { style?: { overflowY?: string } }) => ({ overflowY: el.style?.overflowY ?? "visible" }));
    const panel = { style: { overflowY: "auto" }, parentElement: null };
    const row = { style: {}, parentElement: { style: {}, parentElement: panel } };
    expect(scrollParent(row as unknown as Element)).toBe(panel);
    vi.unstubAllGlobals();
  });
  it("unpunctuated explanatory text is not a command", () => {
    for (const t of ["bb supports --json output", "bb home stats reports the waiting count", "bash is great", "gh shows the diff of a pull request"]) {
      expect(splitCommands(t).every((p) => p.type === "prose")).toBe(true);
    }
  });
  it("real commands stay commands, quoted words included", () => {
    for (const t of ['bb tasks comment AUTA-154 --body "The plan is done and the tests pass"', "gh pr view 12 --web", "bash /tmp/x.sh --apply --accept-install-diff", "sonnerie register mistakeknot/Autarch#34", "bb home stats --json"]) {
      expect(splitCommands(t).some((p) => p.type === "command")).toBe(true);
    }
  });
});

describe("third review: valid commands, containment, one unit, badge refresh", () => {
  it("ordinary subcommands and argument values stay commands", () => {
    for (const t of ["gh pr list --state all", "sudo apt install git curl", "bb tasks update AUTA-154 --status done", "ssh host systemctl status foo"]) {
      expect(splitCommands(t).some((p) => p.type === "command")).toBe(true);
    }
    expect(splitCommands("bb supports --json output").every((p) => p.type === "prose")).toBe(true);
  });
  it("a row clipped by a few pixels is not fully displayed", () => {
    const e = (top: number, bottom: number) =>
      ({ isIntersecting: true, intersectionRatio: 0.99, intersectionRect: { height: Math.min(bottom, 800) - top }, boundingClientRect: { top, bottom, height: bottom - top }, rootBounds: { top: 0, bottom: 800, height: 800 } }) as unknown as IntersectionObserverEntry;
    expect(displayedTracker()(e(300, 805))).toBe(false); // 500px row, 5px below the edge
    expect(displayedTracker()(e(300, 800.5))).toBe(true); // sub-pixel rounding
    const clippedByAncestor = { isIntersecting: true, intersectionRatio: 0.9, intersectionRect: { height: 90 }, boundingClientRect: { top: 100, bottom: 200, height: 100 }, rootBounds: { top: 0, bottom: 800, height: 800 } } as unknown as IntersectionObserverEntry;
    expect(displayedTracker()(clippedByAncestor)).toBe(false);
  });
  it("a tall row whose edge is cut off by an ancestor does not count as displayed", () => {
    const d = displayedTracker();
    const tall = (top: number, bottom: number, ir: { top: number; bottom: number }) =>
      ({ isIntersecting: true, intersectionRatio: 0.3, intersectionRect: { ...ir, height: ir.bottom - ir.top }, boundingClientRect: { top, bottom, height: bottom - top }, rootBounds: { top: 0, bottom: 800, height: 800 } }) as unknown as IntersectionObserverEntry;
    // top edge inside the root but hidden by an overflow:hidden ancestor (intersection starts 40px lower)
    expect(d(tall(100, 2500, { top: 140, bottom: 800 }))).toBe(false);
    // bottom edge inside the root but cut off 40px short
    expect(d(tall(-1700, 700, { top: 0, bottom: 660 }))).toBe(false);
    // genuinely seen: top, then bottom
    const e = displayedTracker();
    e(tall(100, 2500, { top: 100, bottom: 800 }));
    expect(e(tall(-1700, 700, { top: 0, bottom: 700 }))).toBe(true);
  });
  it("commands whose arguments hold ordinary words stay commands", () => {
    for (const t of ["gh issue list --label support", "python3 -m pip install requests numpy pandas scipy matplotlib", "gh pr list --state all --json number,title --limit 50"]) {
      expect(splitCommands(t).some((p) => p.type === "command")).toBe(true);
    }
    expect(splitCommands("bb tasks list is empty").every((p) => p.type === "prose")).toBe(true);
  });
  it("a row cut off sideways is not fully displayed", () => {
    const side = (iw: number) =>
      ({ isIntersecting: true, intersectionRatio: 0.5, intersectionRect: { top: 100, bottom: 200, height: 100, width: iw }, boundingClientRect: { top: 100, bottom: 200, height: 100, width: 600 }, rootBounds: { top: 0, bottom: 800, height: 800 } }) as unknown as IntersectionObserverEntry;
    expect(displayedTracker()(side(300))).toBe(false);
    expect(displayedTracker()(side(600))).toBe(true);
  });
  it("a tall row cut off sideways never records an edge", () => {
    const d = displayedTracker();
    const tall = (top: number, bottom: number, iw: number) =>
      ({ isIntersecting: true, intersectionRatio: 0.15, intersectionRect: { top: Math.max(top, 0), bottom: Math.min(bottom, 800), height: 700, width: iw }, boundingClientRect: { top, bottom, height: bottom - top, width: 600 }, rootBounds: { top: 0, bottom: 800, height: 800 } }) as unknown as IntersectionObserverEntry;
    d(tall(100, 2500, 300));
    expect(d(tall(-1700, 700, 300))).toBe(false);
    const ok = displayedTracker();
    ok(tall(100, 2500, 600));
    expect(ok(tall(-1700, 700, 600))).toBe(true);
  });
  it("more valid commands stay commands, and an escaped trailing space is kept", () => {
    for (const t of ["sudo which bash", "gh api repos/o/r --jq ."]) expect(splitCommands(t).some((p) => p.type === "command")).toBe(true);
    const parts = splitCommands("bash /tmp/x.sh --name=foo\\ ");
    expect(parts).toEqual([{ type: "command", text: "bash /tmp/x.sh --name=foo\\ ", role: "final" }]);
    const two = splitCommands("bash /tmp/x.sh --a \\\n  --b");
    expect(two).toEqual([{ type: "command", text: "bash /tmp/x.sh --a \\\n  --b", role: "final" }]);
  });
  it("the button and the confirmation count the same thing: updates", () => {
    const group: CatchupEntry = { item: "routine:A", kind: "routine", at: "1", text: "3 routine updates in A", members: ["ruling:a", "ruling:b", "ruling:c"], lines: ["a", "b", "c"] };
    const plan = markPlan([group], new Set(["routine:A"]), new Set(["routine:A"]));
    expect(plan.count).toBe(3);
    expect(plan.ids).toHaveLength(plan.count);
  });
  it("acknowledging a notice tells every open surface, so the badge follows", async () => {
    const Database = (await import("better-sqlite3")).default;
    const { createStoreHandle } = await import("../store.js");
    const { wireHome } = await import("../server.js");
    const { makeEnv } = await import("./service-helpers.js");
    void makeEnv;
    const db = new Database(":memory:");
    const handle = createStoreHandle(() => db, {});
    const publish = vi.fn();
    const bb = { log: { info: vi.fn(), warn: vi.fn(), error: vi.fn() }, onDispose: () => {}, background: { service: () => {} }, events: { on: () => {} }, agents: { configure: () => {} }, cli: { register: () => {} }, sdk: { threads: {} }, realtime: { publish } };
    const serve = { projects: async () => [], health: async () => ({ build: { commit: "abc" } }), healthy: async () => true } as never;
    const home = wireHome(bb as never, handle, { serveAddr: "127.0.0.1:8110", serveTokenFile: "/nope", serveProjectDirs: [], autarchBin: "autarch" }, { serve });
    await home.handlers.markSeen({ item: "vizier-adopted:1" } as never);
    expect(publish).toHaveBeenCalledWith("home-queue-changed", {});
    publish.mockClear();
    await home.handlers.markAllSeen({ ids: [] } as never);
    expect(publish).not.toHaveBeenCalled();
  });
});
