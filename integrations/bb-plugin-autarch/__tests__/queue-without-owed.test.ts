import { describe, expect, it } from "vitest";
import { withoutOwed, type QueueView } from "../ui/blocks.js";

const row = (decision_id: string | null) => ({ decision_id }) as never;
const legacy = (id: string) => ({ id }) as never;
const q = (): QueueView => ({ rows: [row("a"), row("b"), row(null)], legacy: { count: 2, owed: [legacy("a"), legacy("c")], runbook: [], machine: { lane: [], asks: [] } }, bindings: [], inactive_projects: [] });

describe("withoutOwed", () => {
  it("drops card rows and legacy asks that Asks already owes, and fixes the legacy count", () => {
    const r = withoutOwed(q(), { owed: [{ id: "a" }] });
    expect(r.rows.map((x) => x.decision_id)).toEqual(["b", null]);
    expect(r.legacy.owed.map((x) => x.id)).toEqual(["c"]);
    expect(r.legacy.count).toBe(1);
  });
  it("hiding every legacy ask leaves count 0 so the empty state shows", () => {
    expect(withoutOwed(q(), { owed: [{ id: "a" }, { id: "c" }] }).legacy.count).toBe(0);
  });
  it("is a no-op when nothing overlaps", () => {
    expect(withoutOwed(q(), { owed: [{ id: "z" }] })).toEqual(q());
  });
});
