// A catch-up reply stays small however many routine rulings pile up (bead mk-ykk6r), and one failed catch-up read
// leaves the rest of the panel up.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { capRoutine, ROUTINE_GROUP_MAX, ROUTINE_TOTAL_MAX, type CatchupItem } from "../catchup.js";
import { CatchupPanel, olderCount, settleCatchup } from "../ui/catchup.js";

const group = (project: string, n: number): CatchupItem => ({
  item: `routine:${project}`,
  kind: "routine",
  project,
  at: "1",
  text: `${n} routine updates in ${project}`,
  members: Array.from({ length: n }, (_, i) => `ruling:${project}-${i}`),
  lines: Array.from({ length: n }, (_, i) => `line ${project}-${i}`),
});

describe("capRoutine", () => {
  it("keeps the newest members of a big group, paired with their lines, and counts the whole", () => {
    const g = group("Sylveste", 1228);
    capRoutine([g]);
    expect(g.total).toBe(1228);
    expect(g.members).toHaveLength(ROUTINE_GROUP_MAX);
    expect(g.lines).toHaveLength(ROUTINE_GROUP_MAX);
    expect(g.members![ROUTINE_GROUP_MAX - 1]).toBe("ruling:Sylveste-1227");
    expect(g.members![0]).toBe(`ruling:Sylveste-${1228 - ROUTINE_GROUP_MAX}`);
    expect(g.lines![0]).toBe(`line Sylveste-${1228 - ROUTINE_GROUP_MAX}`);
    expect(g.text).toBe("1228 routine updates in Sylveste");
  });

  it("leaves a small group whole", () => {
    const g = group("Autarch", 3);
    capRoutine([g]);
    expect(g.members).toHaveLength(3);
    expect(g.total).toBe(3);
  });

  it("holds the groups together to a total, newest group first, and an unfunded group keeps its count", () => {
    const gs = [group("a", 10), group("b", 10), group("c", 10)];
    capRoutine(gs, 10, 15);
    expect(gs.map((g) => g.members!.length)).toEqual([10, 5, 0]);
    expect(gs.map((g) => g.total)).toEqual([10, 10, 10]);
    expect(gs[2]!.lines).toEqual([]);
  });

  it("the default reply for the live shape (1228 + 116 + 64) is bounded by the total", () => {
    const gs = [group("Sylveste", 1228), group("shadow-work", 116), group("jawnomicon", 64)];
    capRoutine(gs);
    const sent = gs.reduce((n, g) => n + g.members!.length, 0);
    expect(sent).toBeLessThanOrEqual(ROUTINE_TOTAL_MAX);
    expect(JSON.stringify(gs).length).toBeLessThan(40_000);
  });
});

describe("a capped group in the panel", () => {
  const g = group("Sylveste", 1228);
  capRoutine([g]);
  it("says how many older updates are not shown, once opened", () => {
    expect(olderCount(g)).toBe(1228 - ROUTINE_GROUP_MAX);
    const html = renderToStaticMarkup(<CatchupPanel items={[g]} expanded={new Set([g.item])} onToggle={() => {}} onOverride={() => {}} onMarkAll={() => {}} />);
    expect(html).toContain("1178 older not shown here");
    expect(html).toContain("1228 routine updates in Sylveste");
  });
  it("says nothing of older ones for a whole group", () => {
    expect(olderCount({ members: ["a"], total: 1 })).toBe(0);
    expect(olderCount({})).toBe(0);
  });
});

describe("a failed catch-up read", () => {
  it("settles to an error, not an exception", async () => {
    expect(await settleCatchup(Promise.reject(new Error('rpc "catchup" failed (HTTP 200)')))).toEqual({ ok: false, error: 'rpc "catchup" failed (HTTP 200)' });
    expect(await settleCatchup(Promise.resolve({}))).toMatchObject({ ok: false });
    expect(await settleCatchup(Promise.resolve({ items: [] }))).toEqual({ ok: true, items: [] });
  });
  it("lets the other reads of the panel's Promise.all land", async () => {
    const [a, c, h] = await Promise.all([Promise.resolve("asks"), settleCatchup(Promise.reject(new Error("cut off"))), Promise.resolve("health")]);
    expect([a, h]).toEqual(["asks", "health"]);
    expect(c.ok).toBe(false);
  });
  it("shows a line in place of the section, and does not say you are caught up", () => {
    const html = renderToStaticMarkup(<CatchupPanel items={[]} loadError="cut off" expanded={new Set()} onToggle={() => {}} onOverride={() => {}} onMarkAll={() => {}} />);
    expect(html).toContain("data-catchup-error");
    expect(html).toContain("Catch-up could not be loaded: cut off");
    expect(html).not.toContain("You are caught up.");
  });
});
