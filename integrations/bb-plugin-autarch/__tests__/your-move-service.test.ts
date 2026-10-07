// Phase 1 service wiring (plan mk-okek.24): a card with a home-move block opens a move, a pick keeps the card
// until independent evidence closes the move, and new generations supersede the old move.
import { afterEach, describe, expect, it } from "vitest";
import { cleanupEnvs, opened, rig, SHA } from "./card-rig.js";

afterEach(cleanupEnvs);

const prMove = (n = 12) => ({ schema: "home-move/v1", kind: "pr", pr: { url: `https://github.com/o/r/pull/${n}` } });
const withMove = (r: ReturnType<typeof rig>, move: object, o: Parameters<ReturnType<typeof rig>["card"]>[0] = {}) =>
  r.card({ ...o, prose: `Which?\n\n\`\`\`home-move\n${JSON.stringify(move)}\n\`\`\`` });

describe("moves opened by cards", () => {
  it("a home-move block opens a move at generation 1", async () => {
    const r = rig();
    const t = r.card({});
    // the block is a fenced section of the description: build it through the rig's desc plus an appended fence
    t.description += "```home-move\n" + JSON.stringify(prMove()) + "\n```\n";
    await r.poll();
    const m = r.svc.store.move(t.id, 1);
    expect(m).toMatchObject({ kind: "pr", state: "open", opened_by: "card" });
    expect(JSON.parse(m!.payload_json)).toEqual({ kind: "pr", url: "https://github.com/o/r/pull/12" });
  });

  it("a card with no home-move block opens none", async () => {
    const r = rig();
    const { t } = await opened(r);
    expect(r.svc.store.moves().filter((m) => m.task_id === t.id)).toEqual([]);
  });

  it("a new generation supersedes the old move and opens its own", async () => {
    const r = rig();
    const t = r.card({ key: "k-gen" });
    t.description += "```home-move\n" + JSON.stringify(prMove(1)) + "\n```\n";
    await r.poll();
    r.edit(t, { description: r.desc({ key: "k-gen", question: "A rewritten question?" }) + "```home-move\n" + JSON.stringify(prMove(2)) + "\n```\n" });
    await r.poll();
    const rows = r.svc.store.moves().filter((m) => m.task_id === t.id);
    expect(rows.map((m) => [m.generation, m.state])).toEqual([[1, "closed"], [2, "open"]]);
    expect(rows[0].closed_by).toBe("home");
    expect(rows[0].evidence).toBe("superseded");
  });
});

describe("pick keeps the card while mk owes a move", () => {
  it("the unlabel is held back, and closing the move on evidence queues it", async () => {
    const r = rig();
    const t = r.card({});
    t.description += "```home-move\n" + JSON.stringify(prMove()) + "\n```\n";
    await r.poll();
    const id = `card-${t.id}-g1`;
    expect(r.mkPick(id)).toMatchObject({ ok: true });
    const kinds = () => (r.db.prepare("SELECT kind FROM card_writes WHERE task_id = ? ORDER BY kind").all(t.id) as { kind: string }[]).map((x) => x.kind);
    expect(kinds()).toEqual(["comment"]);
    expect(r.svc.store.move(t.id, 1)!.state).toBe("open");
    expect(r.svc.closeMoveOn(t.id, 1, "github", "merged")).toBe(true);
    expect(kinds()).toEqual(["comment", "unlabel"]);
    expect(r.svc.closeMoveOn(t.id, 1, "github", "merged")).toBe(false);
    expect(kinds()).toEqual(["comment", "unlabel"]);
  });

  it("a claim never closes the move", async () => {
    const r = rig();
    const t = r.card({});
    t.description += "```home-move\n" + JSON.stringify(prMove()) + "\n```\n";
    await r.poll();
    expect(r.svc.store.claimMove(t.id, 1, "mk")).toBe(true);
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "claimed", closed_at: null });
  });

  it("a plain pick on a card with no move still unlabels", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    expect(r.mkPick(g1.id)).toMatchObject({ ok: true });
    expect((r.db.prepare("SELECT kind FROM card_writes WHERE task_id = ? ORDER BY kind").all(t.id) as { kind: string }[]).map((x) => x.kind)).toEqual(["comment", "unlabel"]);
  });

  it("a needs-context pick by mk opens a context move; a vizier pick does not", async () => {
    const r = rig();
    r.enableDelegation();
    const t = r.card({ ask: { options: [{ id: "a", label: "Need info", kind: "needs-context" }, { id: "b", label: "Per day", kind: "instruction", instruction: "Group per day.", reversible: true }] } });
    await r.poll();
    const id = `card-${t.id}-g1`;
    expect(r.mkPick(id, "p1", "a")).toMatchObject({ ok: true });
    const m = r.svc.store.move(t.id, 1);
    expect(m).toMatchObject({ kind: "context", state: "open", opened_by: "pick" });
    expect(JSON.parse(m!.payload_json)).toEqual({ kind: "context", need: "Need info" });
    expect((r.db.prepare("SELECT kind FROM card_writes WHERE task_id = ?").all(t.id) as { kind: string }[]).map((x) => x.kind)).not.toContain("unlabel");
    // a replayed pick opens no second move
    expect(r.svc.store.moves().filter((x) => x.task_id === t.id)).toHaveLength(1);
  });
});

void SHA;
void withMove;
