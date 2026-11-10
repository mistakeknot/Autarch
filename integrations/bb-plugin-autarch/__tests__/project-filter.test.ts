// The project filter's logic (bead mk-sufr9): which cards a ticked-out project hides, what is kept on the device, and
// the one switch that decides whether the tab counts follow the filter.
import { describe, expect, it } from "vitest";
import {
  COUNTS_FOLLOW_FILTER,
  filterAsks,
  filterQueue,
  hiddenIn,
  hiddenText,
  loadExcluded,
  PROJECT_FILTER_KEY,
  projectChoices,
  saveExcluded,
  shownCount,
  shownIn,
  toggleProject,
} from "../ui/projectfilterstore.js";
import type { AsksData, OwedAsk } from "../ui/asks.js";
import type { QueueView, QueueRowView } from "../ui/blocks.js";

const card = (id: string, project: string | null) => ({ id, project });
const memory = (initial?: string) => {
  const m = new Map<string, string>(initial === undefined ? [] : [[PROJECT_FILTER_KEY, initial]]);
  return { m, getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
};

describe("shownIn and hiddenIn", () => {
  const cards = [card("a", "Autarch"), card("b", "Clavain"), card("c", "Autarch"), card("d", null), card("e", "")];
  it("keeps everything when nothing is excluded", () => {
    expect(shownIn(cards, new Set())).toEqual(cards);
    expect(hiddenIn(cards, new Set())).toBe(0);
  });
  it("drops the cards of an excluded project, keeping order", () => {
    expect(shownIn(cards, new Set(["Autarch"])).map((c) => c.id)).toEqual(["b", "d", "e"]);
    expect(hiddenIn(cards, new Set(["Autarch"]))).toBe(2);
  });
  it("never hides a card that has no project: there is nothing to untick", () => {
    expect(shownIn(cards, new Set(["Autarch", "Clavain"])).map((c) => c.id)).toEqual(["d", "e"]);
  });
  it("ignores an excluded name that no card carries", () => {
    expect(shownIn(cards, new Set(["Gone"]))).toEqual(cards);
  });
});

describe("projectChoices", () => {
  it("lists each project once with its card count, sorted without regard to case", () => {
    const c = projectChoices([card("1", "clavain"), card("2", "Autarch"), card("3", "clavain"), card("4", null), card("5", "")]);
    expect(c).toEqual([{ name: "Autarch", count: 1 }, { name: "clavain", count: 2 }]);
  });
  it("is empty when no card has a project", () => {
    expect(projectChoices([card("1", null)])).toEqual([]);
  });
});

describe("toggleProject", () => {
  it("excludes an included project and includes it again, without changing the input", () => {
    const start: ReadonlySet<string> = new Set();
    const one = toggleProject(start, "Autarch");
    expect([...one]).toEqual(["Autarch"]);
    expect(start.size).toBe(0);
    expect(toggleProject(one, "Autarch").size).toBe(0);
  });
});

describe("persistence", () => {
  it("round-trips the excluded set", () => {
    const s = memory();
    saveExcluded(() => s, new Set(["B", "A"]));
    expect([...loadExcluded(() => s)].sort()).toEqual(["A", "B"]);
  });
  it("starts with nothing excluded when nothing is stored, so a new project is included", () => {
    expect(loadExcluded(() => memory()).size).toBe(0);
  });
  it("starts with nothing excluded when the stored value is damaged or the wrong shape", () => {
    for (const bad of ["{not json", "42", "null", '{"excluded":"A"}', '{"excluded":[1,2]}', '["A"]']) {
      expect(loadExcluded(() => memory(bad)).size).toBe(0);
    }
  });
  it("keeps only the strings of a mixed list", () => {
    expect([...loadExcluded(() => memory('{"excluded":["A",3,null,"B"]}'))].sort()).toEqual(["A", "B"]);
  });
  it("stores nothing for an empty set, so All projects leaves no trace", () => {
    const s = memory();
    saveExcluded(() => s, new Set(["A"]));
    saveExcluded(() => s, new Set());
    expect(s.m.has(PROJECT_FILTER_KEY)).toBe(false);
  });
  it("survives storage that is missing or throws", () => {
    expect(loadExcluded(() => null).size).toBe(0);
    expect(loadExcluded(() => { throw new Error("blocked"); }).size).toBe(0);
    expect(() => saveExcluded(() => { throw new Error("blocked"); }, new Set(["A"]))).not.toThrow();
    expect(() => saveExcluded(() => ({ getItem: () => null, setItem: () => { throw new Error("full"); } }), new Set(["A"]))).not.toThrow();
  });
});

const owed = (id: string, project: string, over: Partial<OwedAsk> = {}): OwedAsk => ({
  id, project, thread: "t", subject: id, asker: "thread", filed_at: "2026-11-01T00:00:00Z", revision: "r", task_id: `task-${id}`,
  ask: { question: "q", options: [] }, ...over,
});
const asks = (o: OwedAsk[]): AsksData => ({ owed: o, runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {} });

describe("filterAsks", () => {
  it("filters the owed cards only and leaves every other part of the data as it was", () => {
    const data = asks([owed("1", "A"), owed("2", "B")]);
    const out = filterAsks(data, new Set(["A"]));
    expect(out.owed.map((o) => o.id)).toEqual(["2"]);
    expect(out.delegation).toBe(data.delegation);
    expect(data.owed).toHaveLength(2);
  });
  it("returns the same object when nothing is excluded", () => {
    const data = asks([owed("1", "A")]);
    expect(filterAsks(data, new Set())).toBe(data);
  });
});

const row = (id: string, project: string | null): QueueRowView => ({
  id, decision_id: null, task_id: id, card_key: null, binding_state: "confirmed", project, title: id, refs: [], blocks_count: 0,
  created_at: "2026-11-01T00:00:00Z", thread: null, pinned: false, ask: null, revision: null, mentions: 0, display_reason: null,
  display_only: false, overrides_generation: null, changed_after_ruling: false, root: { state: null, reason: null },
});
const queue = (rows: QueueRowView[], legacyOwed: QueueView["legacy"]["owed"] = [], count = legacyOwed.length): QueueView => ({
  rows, legacy: { count, owed: legacyOwed, runbook: [], machine: { lane: [], asks: [] } }, bindings: [], inactive_projects: [],
});

describe("filterQueue", () => {
  it("drops the Blocked rows and legacy asks of an excluded project and lowers the legacy count by what it dropped", () => {
    const lo = (id: string, project: string) => ({ id, project, thread: "t", subject: id, filed_at: "x", revision: "r", mentions: 0, ask: { question: "q", options: [] } });
    const q = queue([row("r1", "A"), row("r2", "B"), row("r3", null)], [lo("l1", "A"), lo("l2", "B")], 5);
    const out = filterQueue(q, new Set(["A"]));
    expect(out.rows.map((r) => r.id)).toEqual(["r2", "r3"]);
    expect(out.legacy.owed.map((o) => o.id)).toEqual(["l2"]);
    expect(out.legacy.count).toBe(4);
  });
  it("returns the same object when nothing is excluded", () => {
    const q = queue([row("r1", "A")]);
    expect(filterQueue(q, new Set())).toBe(q);
  });
});

describe("the count switch", () => {
  it("is off: the tab and badge counts stay counted across all projects", () => {
    expect(COUNTS_FOLLOW_FILTER).toBe(false);
    expect(shownCount(10, 3)).toBe(10);
  });
});

describe("hiddenText", () => {
  it("says plainly how many the filter hides", () => {
    expect(hiddenText(3)).toBe("3 hidden by the project filter");
    expect(hiddenText(1)).toBe("1 hidden by the project filter");
  });
});
