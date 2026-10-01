import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Catchup } from "../catchup.js";
import { Delegation } from "../delegation.js";
import type { Service } from "../service.js";
import { ask, makeEnv, type Env, verifiedDelegation } from "./service-helpers.js";

let env: Env;
let svc: Service;
let dele: Delegation;
let cu: Catchup;
beforeEach(() => {
  env = makeEnv(["Autarch", "Other"]);
  svc = env.open();
  dele = verifiedDelegation(svc);
  cu = new Catchup(svc, dele);
});
afterEach(() => env.cleanup());

let n = 0;
const file = async (thread: string, over: Record<string, unknown> = {}) => {
  n += 1;
  const r = await svc.file(ask(env, { thread, question: `Question ${n} for ${thread}?`, subject: `s${n}`, ...over }), {});
  if (!r.ok) throw new Error(r.error);
  return r.decision_id;
};
const rev = (id: string) => (svc.store.decision(id) as { revision: string }).revision;
const pick = (id: string, option = "day", by = "mk") => {
  const p = svc.pick(id, option, rev(id), `pk-${id}`, by, by === "mk" ? "home" : "cli");
  if (!p.ok) throw new Error(p.error);
};
const enable = () => {
  dele.setDelegation({ vizierThreadId: "thr-vizier", projects: ["Autarch"], dailyCap: 50 }, {});
  dele.markSeen("mk", dele.latestSettingsItem()!);
};
const kinds = () => cu.items().map((i) => i.kind);

describe("catch-up ranking [V2] [V9]", () => {
  it("ranks failures, then owed asks, then delegated rulings, then routine", async () => {
    enable();
    const routine = await file("thr-a");
    pick(routine);
    const deleg = await file("thr-b");
    pick(deleg, "project", "vizier");
    await file("thr-c"); // owed
    expect(cu.recordTurnFailed("thr-a", "req-1")).toBe(true);
    expect(kinds()).toEqual(["failure", "owed", "delegated", "routine"]);
  });

  it("records a failed run only for a thread that has filed asks", () => {
    expect(cu.recordTurnFailed("thr-nobody", "req-1")).toBe(false);
    expect(cu.items()).toEqual([]);
  });

  it("does not record the same failed request twice", async () => {
    await file("thr-a");
    expect(cu.recordTurnFailed("thr-a", "req-1")).toBe(true);
    expect(cu.recordTurnFailed("thr-a", "req-1")).toBe(false);
    expect(cu.items().filter((i) => i.kind === "failure")).toHaveLength(1);
  });

  it("an undeliverable wake is a failure", async () => {
    const id = await file("thr-a");
    pick(id, "project");
    const wake = svc.store.obligationsFor(id).find((o) => o.kind === "wake")!;
    const c = svc.store.claim(wake.id, 0);
    if (!c.ok) throw new Error("claim");
    svc.store.transition(wake.id, "sending", "undeliverable", c.attempt);
    const f = cu.items().filter((i) => i.kind === "failure");
    expect(f).toHaveLength(1);
    expect(f[0]!.text).toContain("thr-a");
  });

  it("collapses routine items per project", async () => {
    for (let i = 0; i < 3; i++) pick(await file(`thr-${i}`));
    for (let i = 0; i < 2; i++) pick(await file(`thr-o${i}`, { project: "Other", project_root: env.roots.Other }));
    const routine = cu.items().filter((i) => i.kind === "routine");
    expect(routine).toHaveLength(2);
    const by = Object.fromEntries(routine.map((r) => [r.project, r.members!.length]));
    expect(by).toEqual({ Autarch: 3, Other: 2 });
  });

  it("puts a vizier's note after the facts, marked, linking its cited facts", async () => {
    const id = await file("thr-a");
    pick(id);
    expect(cu.addNote("Looks routine.", ["nope"])).toMatchObject({ ok: false });
    const r = cu.addNote("Looks routine.", [id]);
    expect(r).toMatchObject({ ok: true });
    const items = cu.items();
    const note = items[items.length - 1]!;
    expect(note.kind).toBe("note");
    expect(note.text).toContain("vizier's note");
    expect(note.cites).toEqual([id]);
  });
});

describe("markAllSeen(snapshot) [D-12]", () => {
  it("30 failures push two delegated rulings offscreen: a snapshot of the visible items leaves both pinned and unseen", async () => {
    enable();
    const a = await file("thr-a");
    const b = await file("thr-b");
    pick(a, "project", "vizier");
    pick(b, "project", "vizier");
    for (let i = 0; i < 30; i++) cu.recordTurnFailed("thr-a", `req-${i}`);
    const all = cu.items();
    const visible = all.slice(0, 20).map((i) => i.item);
    expect(visible.every((id) => id.startsWith("failed:"))).toBe(true);
    cu.markAllSeen(visible);
    expect(dele.pinned().map((p) => p.decision).sort()).toEqual([a, b].sort());
    const left = cu.items();
    expect(left.filter((i) => i.kind === "failure")).toHaveLength(10);
    expect(left.filter((i) => i.kind === "delegated")).toHaveLength(2);
  });

  it("a ruling inserted after the snapshot stays unseen; ids outside the snapshot are never marked", async () => {
    enable();
    const a = await file("thr-a");
    pick(a, "project", "vizier");
    const snapshot = cu.items().map((i) => i.item);
    const late = await file("thr-late");
    pick(late, "project", "vizier");
    const marked = cu.markAllSeen(snapshot);
    expect(marked).toContain(`ruling:${a}`);
    expect(dele.pinned().map((p) => p.decision)).toEqual([late]);
    expect(cu.markAllSeen(["bogus", `ruling:${late}x`])).toEqual([]);
    expect(svc.store.hasSeen("mk", "bogus")).toBe(false);
    expect(cu.markAllSeen([])).toEqual([]);
    expect(svc.store.hasSeen("mk", `ruling:${late}`)).toBe(false);
  });

  it("never marks an open ask: it stays owed", async () => {
    await file("thr-a");
    const owed = cu.items().filter((i) => i.kind === "owed");
    expect(owed).toHaveLength(1);
    expect(cu.markAllSeen([owed[0]!.item])).toEqual([]);
    expect(cu.items().filter((i) => i.kind === "owed")).toHaveLength(1);
  });

  it("a collapsed group is marked only through the member ids the panel expanded", async () => {
    const ids = [] as string[];
    for (let i = 0; i < 3; i++) {
      const id = await file(`thr-${i}`);
      pick(id);
      ids.push(id);
    }
    const g = cu.items().find((i) => i.kind === "routine")!;
    expect(cu.markAllSeen([g.item])).toEqual([]);
    expect(cu.markAllSeen([g.members![0]!])).toEqual([g.members![0]]);
    expect(cu.items().find((i) => i.kind === "routine")!.members).toHaveLength(2);
  });
});
