// Moves are retired (bead mk-1k8pg): neither a card's home-move block nor a pick opens one, a move left open from
// before still holds a picked card until it closes, and retireMoves closes every live move once.
// The service wiring these tests began with is plan mk-okek.24.
import { afterEach, describe, expect, it } from "vitest";
import { cleanupEnvs, opened, rig, type Rig } from "./card-rig.js";

afterEach(cleanupEnvs);

const prMove = (n = 12) => ({ schema: "home-move/v1", kind: "pr", pr: { url: `https://github.com/o/r/pull/${n}` } });
const fence = (m: object) => "```home-move\n" + JSON.stringify(m) + "\n```\n";
const kinds = (r: Rig, taskId: string) => (r.db.prepare("SELECT kind FROM card_writes WHERE task_id = ? ORDER BY kind").all(taskId) as { kind: string }[]).map((x) => x.kind);
const closes = (r: Rig) => r.db.prepare("SELECT COUNT(*) AS n FROM move_closes").get() as { n: number };

describe("Home opens no moves", () => {
  it("a home-move block on a card opens none; the card is still a decision", async () => {
    const r = rig();
    const t = r.card({});
    t.description += fence(prMove());
    await r.poll();
    expect(r.gens(t.id)).toHaveLength(1);
    expect(r.svc.store.moves()).toEqual([]);
  });

  it("a new generation supersedes a move left from before and opens none of its own", async () => {
    const r = rig();
    const t = r.card({ key: "k-gen" });
    t.description += fence(prMove(1));
    await r.poll();
    r.svc.store.openMove({ task_id: t.id, generation: 1, kind: "pr", payload: { kind: "pr", url: "https://github.com/o/r/pull/1" }, opened_by: "card" });
    r.edit(t, { description: r.desc({ key: "k-gen", question: "A rewritten question?" }) + fence(prMove(2)) });
    await r.poll();
    const rows = r.svc.store.moves().filter((m) => m.task_id === t.id);
    expect(rows.map((m) => [m.generation, m.state, m.closed_by, m.evidence])).toEqual([[1, "closed", "home", "superseded"]]);
  });

  it("a needs-context pick opens no move and unlabels like any other pick", async () => {
    const r = rig();
    const t = r.card({ ask: { options: [{ id: "a", label: "Need info", kind: "needs-context" }, { id: "b", label: "Per day", kind: "instruction", instruction: "Group per day.", reversible: true }] } });
    await r.poll();
    expect(r.mkPick(`card-${t.id}-g1`, "p1", "a")).toMatchObject({ ok: true });
    expect(r.svc.store.moves()).toEqual([]);
    expect(kinds(r, t.id)).toEqual(["comment", "unlabel"]);
  });

  it("a plain pick on a card with no move unlabels", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    expect(r.mkPick(g1.id)).toMatchObject({ ok: true });
    expect(kinds(r, t.id)).toEqual(["comment", "unlabel"]);
  });
});

describe("a move left open from before", () => {
  it("holds the unlabel of mk's pick, and closing the move on evidence queues it", async () => {
    const r = rig({ legacyMoves: true });
    const t = r.card({});
    t.description += fence(prMove());
    await r.poll();
    expect(r.mkPick(`card-${t.id}-g1`)).toMatchObject({ ok: true });
    expect(kinds(r, t.id)).toEqual(["comment"]);
    expect(r.svc.store.move(t.id, 1)!.state).toBe("open");
    expect(r.svc.closeMoveOn(t.id, 1, "github", "merged")).toBe(true);
    expect(kinds(r, t.id)).toEqual(["comment", "unlabel"]);
    expect(r.svc.closeMoveOn(t.id, 1, "github", "merged")).toBe(false);
    expect(kinds(r, t.id)).toEqual(["comment", "unlabel"]);
  });

  it("is never closed by a claim", async () => {
    const r = rig({ legacyMoves: true });
    const t = r.card({});
    t.description += fence(prMove());
    await r.poll();
    expect(r.svc.store.claimMove(t.id, 1, "mk")).toBe(true);
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "claimed", closed_at: null });
  });
});

describe("retireMoves", () => {
  it("closes open and claimed moves, queues the held unlabel of a picked card only, and closes no tasks card", async () => {
    const r = rig({ legacyMoves: true });
    const picked = r.card({});
    picked.description += fence(prMove(1));
    const claimed = r.card({});
    claimed.description += fence(prMove(2));
    const done = r.card({});
    done.description += fence(prMove(3));
    await r.poll();
    expect(r.mkPick(`card-${picked.id}-g1`)).toMatchObject({ ok: true });
    expect(r.svc.store.claimMove(claimed.id, 1, "mk")).toBe(true);
    expect(r.svc.closeMoveOn(done.id, 1, "github", "merged")).toBe(true);
    const closesBefore = closes(r).n;

    expect(r.svc.retireMoves()).toBe(2);
    const rows = r.svc.store.moves();
    expect(rows.every((m) => m.state === "closed")).toBe(true);
    expect(rows.filter((m) => m.evidence === "moves retired").map((m) => [m.task_id, m.closed_by]).sort()).toEqual([[claimed.id, "home"], [picked.id, "home"]].sort());
    expect(kinds(r, picked.id)).toEqual(["comment", "unlabel"]);
    expect(kinds(r, claimed.id)).toEqual([]);
    expect(closes(r).n).toBe(closesBefore);
  });

  it("is idempotent: a second run closes nothing and queues nothing", async () => {
    const r = rig({ legacyMoves: true });
    const t = r.card({});
    t.description += fence(prMove());
    await r.poll();
    expect(r.mkPick(`card-${t.id}-g1`)).toMatchObject({ ok: true });
    expect(r.svc.retireMoves()).toBe(1);
    expect(r.svc.retireMoves()).toBe(0);
    expect(kinds(r, t.id)).toEqual(["comment", "unlabel"]);
  });
});
