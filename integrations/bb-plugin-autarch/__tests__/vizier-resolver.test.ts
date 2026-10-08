import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Delegation, type ThreadRow } from "../delegation.js";
import type { Service } from "../service.js";
import { ask, makeEnv, type Env } from "./service-helpers.js";

const T = "Masaq' | vizier";
const th = (id: string, over: Partial<ThreadRow> = {}): ThreadRow => ({ id, title: T, pinnedAt: 1, archivedAt: null, deletedAt: null, ...over });

let env: Env;
let svc: Service;
let rows: ThreadRow[];
let dele: Delegation;
let listFails = false;
beforeEach(() => {
  env = makeEnv(["Autarch", "Other"]);
  svc = env.open();
  rows = [];
  listFails = false;
  dele = new Delegation(svc, async () => {
    if (listFails) throw new Error("down");
    return rows;
  });
  dele.applyProjectList(["Autarch", "Other"]);
});
afterEach(() => env.cleanup());

const eventsOf = (type: string) => svc.store.events().filter((e) => e.type === type).map((e) => JSON.parse(e.detail_json));
const stored = () => svc.store.setting("vizierThreadId");
/** mk configures delegation from the panel and sees the change. */
function enable(vizier: string) {
  expect(dele.setDelegation({ vizierThreadId: vizier, projects: ["Autarch"], dailyCap: 5 }, {})).toMatchObject({ ok: true });
  dele.markSeen("mk", dele.latestSettingsItem()!);
}
const fileId = async () => {
  const r = await svc.file(ask(env, {}), {});
  if (!r.ok) throw new Error(r.error);
  return r.decision_id;
};

describe("vizier resolver", () => {
  it("a stored live id stays the vizier, even beside a pinned title match; nothing is recorded", async () => {
    rows = [th("thr_stored", { title: "work", pinnedAt: null }), th("thr_title")];
    enable("thr_stored");
    expect(await dele.resolveVizier()).toEqual({ ok: true, id: "thr_stored", via: "stored" });
    expect(stored()).toBe("thr_stored");
    expect(eventsOf("vizier-adopted")).toEqual([]);
  });

  it("the vizier hands off to a live thread: stored id moves, one event, no suspension", async () => {
    rows = [th("thr_old"), th("thr_new", { title: "next", pinnedAt: null })];
    enable("thr_old");
    const r = await dele.handoff("thr_new", { threadId: "thr_old" });
    expect(r).toEqual({ ok: true, from: "thr_old", to: "thr_new" });
    expect(stored()).toBe("thr_new");
    expect(eventsOf("vizier-handoff")).toEqual([{ from: "thr_old", to: "thr_new", by: "thr_old" }]);
    expect(dele.suspendedNow()).toBe(false);
    expect((await dele.resolveVizier()).ok && (await dele.resolveVizier())).toMatchObject({ id: "thr_new" });
    // the old vizier lost its role
    expect(await dele.handoff("thr_old", { threadId: "thr_old" })).toMatchObject({ ok: false, status: 403 });
  });

  it("a handoff by a non-vizier or with no caller thread is refused and changes nothing", async () => {
    rows = [th("thr_old"), th("thr_x", { title: "x", pinnedAt: null }), th("thr_y", { title: "y", pinnedAt: null })];
    enable("thr_old");
    expect(await dele.handoff("thr_y", { threadId: "thr_x" })).toMatchObject({ ok: false, status: 403 });
    expect(await dele.handoff("thr_y", {})).toMatchObject({ ok: false, status: 403 });
    expect(stored()).toBe("thr_old");
    expect(eventsOf("vizier-handoff")).toEqual([]);
  });

  it("a handoff refuses a malformed id, the caller's own id, and an archived, deleted or unknown thread", async () => {
    rows = [th("thr_old"), th("thr_arch", { title: "a", pinnedAt: null, archivedAt: 5 }), th("thr_del", { title: "d", pinnedAt: null, deletedAt: 5 })];
    enable("thr_old");
    const by = { threadId: "thr_old" };
    expect(await dele.handoff("not a thread", by)).toMatchObject({ ok: false, status: 400 });
    expect(await dele.handoff("vizier", by)).toMatchObject({ ok: false, status: 400 });
    expect(await dele.handoff("thr_old", by)).toMatchObject({ ok: false, status: 400 });
    expect(await dele.handoff("thr_arch", by)).toMatchObject({ ok: false, status: 404 });
    expect(await dele.handoff("thr_del", by)).toMatchObject({ ok: false, status: 404 });
    expect(await dele.handoff("thr_nope", by)).toMatchObject({ ok: false, status: 404 });
    expect(stored()).toBe("thr_old");
    expect(eventsOf("vizier-handoff")).toEqual([]);
  });

  it("a handoff is refused when threads cannot be listed (the successor cannot be checked)", async () => {
    rows = [th("thr_old"), th("thr_new", { title: "n", pinnedAt: null })];
    enable("thr_old");
    listFails = true;
    expect(await dele.handoff("thr_new", { threadId: "thr_old" })).toMatchObject({ ok: false, status: 503 });
    expect(stored()).toBe("thr_old");
  });

  it("a stored id that is archived, with exactly one pinned match, is replaced by it: stored and recorded", async () => {
    rows = [th("thr_gone", { archivedAt: 9 }), th("thr_live")];
    enable("thr_gone");
    expect(await dele.resolveVizier()).toEqual({ ok: true, id: "thr_live", via: "adopted" });
    expect(stored()).toBe("thr_live");
    expect(eventsOf("vizier-adopted")).toEqual([{ from: "thr_gone", to: "thr_live", reason: "stored-thread-archived-or-gone" }]);
    // asking again adopts nothing more
    expect(await dele.resolveVizier()).toEqual({ ok: true, id: "thr_live", via: "stored" });
    expect(eventsOf("vizier-adopted")).toHaveLength(1);
  });

  it("an unset id is adopted from the single pinned match and recorded with from:null", async () => {
    rows = [th("thr_live"), th("thr_old_vizier", { archivedAt: 9 }), th("thr_unpinned", { pinnedAt: null })];
    expect(await dele.resolveVizier()).toEqual({ ok: true, id: "thr_live", via: "adopted" });
    expect(eventsOf("vizier-adopted")).toEqual([{ from: null, to: "thr_live", reason: "unset" }]);
  });

  it("a stored id missing from the thread list is treated as gone", async () => {
    rows = [th("thr_live")];
    enable("thr_deleted_long_ago");
    expect(await dele.resolveVizier()).toMatchObject({ ok: true, id: "thr_live", via: "adopted" });
  });

  it("zero matches refuse and store nothing", async () => {
    rows = [th("thr_a", { title: "work", pinnedAt: null }), th("thr_b", { archivedAt: 3 }), th("thr_c", { pinnedAt: null })];
    expect(await dele.resolveVizier()).toMatchObject({ ok: false, status: 409 });
    enable("thr_gone");
    expect(await dele.resolveVizier()).toMatchObject({ ok: false, status: 409 });
    expect(stored()).toBe("thr_gone");
    expect(eventsOf("vizier-adopted")).toEqual([]);
  });

  it("two matches refuse, name both, and store nothing", async () => {
    rows = [th("thr_a"), th("thr_b")];
    const r = await dele.resolveVizier();
    expect(r).toMatchObject({ ok: false, status: 409 });
    expect(!r.ok && r.error).toContain("thr_a, thr_b");
    expect(stored()).toBeUndefined();
    expect(eventsOf("vizier-adopted")).toEqual([]);
  });

  it("fails closed: when threads cannot be listed, neither a stored id nor an unset one is the vizier", async () => {
    listFails = true;
    expect(await dele.resolveVizier()).toMatchObject({ ok: false, status: 503 });
    enable("thr_kept");
    expect(await dele.resolveVizier()).toMatchObject({ ok: false, status: 503 });
    expect(stored()).toBe("thr_kept");
    expect(eventsOf("vizier-adopted")).toEqual([]);
    listFails = false;
    rows = [th("thr_kept")];
    expect(await dele.resolveVizier()).toMatchObject({ ok: true, id: "thr_kept" });
  });

  it("an adoption never overwrites a choice made while it was in flight", async () => {
    rows = [th("thr_gone", { archivedAt: 9 }), th("thr_live")];
    enable("thr_gone");
    // a handoff-like write lands between the listing and the adoption
    let calls = 0;
    const racing = new Delegation(svc, async () => {
      calls++;
      if (calls === 2) svc.store.setSetting("vizierThreadId", "thr_someone_else");
      return rows;
    });
    racing.applyProjectList(["Autarch"]);
    expect(await racing.resolveVizier()).toMatchObject({ ok: false, status: 409 });
    expect(stored()).toBe("thr_someone_else");
    expect(eventsOf("vizier-adopted")).toEqual([]);
  });

  it("two listings that disagree refuse the adoption", async () => {
    let calls = 0;
    const flaky = new Delegation(svc, async () => (++calls === 1 ? [th("thr_a")] : [th("thr_a"), th("thr_b")]));
    expect(await flaky.resolveVizier()).toMatchObject({ ok: false, status: 409, error: expect.stringContaining("changed") });
    expect(stored()).toBeUndefined();
  });

  it("a direct thread read decides liveness: archived falls back, live stays, a throw falls back to the list", async () => {
    rows = [th("thr_live")];
    const withGetter = (get: (id: string) => Promise<ThreadRow | null>) => {
      const d = new Delegation(svc, async () => rows, get);
      d.applyProjectList(["Autarch"]);
      return d;
    };
    enable("thr_stored");
    // the list does not hold thr_stored, but a direct read says it is live: it stays the vizier
    expect(await withGetter(async (id) => th(id, { title: "work", pinnedAt: null })).resolveVizier()).toEqual({ ok: true, id: "thr_stored", via: "stored" });
    // a direct read says archived: fall back
    expect(await withGetter(async (id) => th(id, { archivedAt: 4 })).resolveVizier()).toMatchObject({ ok: true, id: "thr_live", via: "adopted" });
    enable("thr_stored");
    // a throw: the list decides (thr_stored absent: gone)
    expect(await withGetter(async () => { throw new Error("404"); }).resolveVizier()).toMatchObject({ ok: true, id: "thr_live", via: "adopted" });
  });

  it("a peek shows the candidate and writes nothing", async () => {
    rows = [th("thr_live")];
    expect(await dele.resolveVizier({ adopt: false })).toEqual({ ok: true, id: "thr_live", via: "candidate" });
    expect(stored()).toBeUndefined();
    expect(eventsOf("vizier-adopted")).toEqual([]);
    expect(dele.suspendedNow()).toBe(false);
  });

  it("isStoredVizier reads the stored id synchronously", () => {
    expect(dele.isStoredVizier("thr_x")).toBe(false);
    enable("thr_x");
    expect(dele.isStoredVizier("thr_x")).toBe(true);
    expect(dele.isStoredVizier("thr_y")).toBe(false);
    expect(dele.isStoredVizier(undefined)).toBe(false);
  });

  it("a delegate without a thread lister never adopts", async () => {
    const bare = new Delegation(svc);
    expect(await bare.resolveVizier()).toMatchObject({ ok: false });
    enable("thr_kept");
    expect(await bare.resolveVizier()).toMatchObject({ ok: false, status: 503 });
  });
});

describe("fallback adoption and delegated rule [D-1]", () => {
  it("adoption suspends delegated rule until mk has seen it; a Settings save of the same id does not clear it", async () => {
    rows = [th("thr_gone", { archivedAt: 9 }), th("thr_live")];
    enable("thr_gone");
    const id = await fileId();
    expect(await dele.resolveVizier()).toMatchObject({ via: "adopted" });
    expect(dele.suspendedNow()).toBe(true);
    const pinned = dele.pinned().find((p) => p.item.startsWith("vizier-adopted:"));
    expect(pinned?.text).toContain("thr_live");
    expect(dele.rule(id, "project", "reversible and low risk", { threadId: "thr_live" })).toMatchObject({ ok: false, status: 403, error: expect.stringContaining("suspended") });
    // mk's own Settings save does not make the adoption seen
    enable("thr_live");
    expect(dele.suspendedNow()).toBe(true);
    dele.markSeen("mk", pinned!.item);
    expect(dele.suspendedNow()).toBe(false);
    expect(dele.rule(id, "project", "reversible and low risk", { threadId: "thr_live" })).toMatchObject({ ok: true });
  });

  it("ruleResolved adopts first, so the pinned vizier is recognised and is then suspended, not refused as a stranger", async () => {
    rows = [th("thr_live")];
    enable("thr_gone");
    const id = await fileId();
    const r = await dele.ruleResolved(id, "project", "reversible and low risk", { threadId: "thr_live" });
    expect(r).toMatchObject({ ok: false, status: 403, error: expect.stringContaining("suspended") });
    expect(stored()).toBe("thr_live");
    // a thread that is not the resolved vizier is refused as such
    expect(await dele.ruleResolved(id, "project", "reversible and low risk", { threadId: "thr_other" })).toMatchObject({ ok: false, status: 403, error: "only the vizier thread may rule" });
  });

  it("isLive reads liveness fresh: live, archived, and undefined when nothing can say", async () => {
    rows = [th("thr_a")];
    expect(await dele.isLive("thr_a", false)).toBe(true);
    expect(await dele.isLive("thr_b", false)).toBe(false);
    rows = [th("thr_a", { archivedAt: 3 })];
    expect(await dele.isLive("thr_a", false)).toBe(false);
    listFails = true;
    expect(await dele.isLive("thr_a", false)).toBeUndefined();
  });

  it("a handoff refuses when the caller is archived while the successor is checked", async () => {
    rows = [th("thr_old"), th("thr_new", { title: "next", pinnedAt: null })];
    enable("thr_old");
    let calls = 0;
    const d = new Delegation(svc, async () => {
      // 1: resolve stored, 2: successor, 3: caller re-confirm: the caller is archived by then
      if (++calls === 3) rows = [th("thr_old", { archivedAt: 5 }), th("thr_new", { title: "next", pinnedAt: null })];
      return rows;
    });
    d.applyProjectList(["Autarch"]);
    expect(await d.handoff("thr_new", { threadId: "thr_old" })).toMatchObject({ ok: false, status: 409 });
    expect(stored()).toBe("thr_old");
    expect(eventsOf("vizier-handoff")).toEqual([]);
  });

  it("two scans with the same matches but a different thread order refuse adoption", async () => {
    let calls = 0;
    const d = new Delegation(svc, async () => (++calls === 1 ? [th("thr_a"), th("thr_x", { title: "w", pinnedAt: null })] : [th("thr_x", { title: "w", pinnedAt: null }), th("thr_a")]));
    expect(await d.resolveVizier()).toMatchObject({ ok: false, status: 409 });
  });

  it("a thread list that shifts between the two scans (same matches, different size) refuses adoption", async () => {
    let calls = 0;
    const shifty = new Delegation(svc, async () => (++calls === 1 ? [th("thr_a")] : [th("thr_a"), th("thr_x", { title: "work", pinnedAt: null })]));
    expect(await shifty.resolveVizier()).toMatchObject({ ok: false, status: 409 });
    expect(stored()).toBeUndefined();
  });

  it("ruleResolved refuses when the resolver does, instead of falling through to the stored id", async () => {
    rows = [th("thr_a", { archivedAt: 1 })];
    enable("thr_a");
    const id = await fileId();
    expect(await dele.ruleResolved(id, "project", "reversible and low risk", { threadId: "thr_a" })).toMatchObject({ ok: false, status: 409 });
    listFails = true;
    expect(await dele.ruleResolved(id, "project", "reversible and low risk", { threadId: "thr_a" })).toMatchObject({ ok: false, status: 503 });
    expect(svc.store.pick(id)).toBeUndefined();
  });

  it("a Settings save is the existing D-1 settings suspension, unrelated to adoption: unseen it suspends, seen it does not", async () => {
    rows = [th("thr_v")];
    expect(dele.setDelegation({ vizierThreadId: "thr_v", projects: ["Autarch"], dailyCap: 5 }, {})).toMatchObject({ ok: true });
    expect(dele.suspendedNow()).toBe(true);
    dele.markSeen("mk", dele.latestSettingsItem()!);
    expect(dele.suspendedNow()).toBe(false);
    expect(eventsOf("vizier-adopted")).toEqual([]);
  });

  it("a vizier handoff does not suspend delegated rule", async () => {
    rows = [th("thr_old"), th("thr_new", { title: "n", pinnedAt: null })];
    enable("thr_old");
    const id = await fileId();
    expect(await dele.handoff("thr_new", { threadId: "thr_old" })).toMatchObject({ ok: true });
    expect(await dele.ruleResolved(id, "project", "reversible and low risk", { threadId: "thr_new" })).toMatchObject({ ok: true });
  });

  it("delegation projects and dailyCap are untouched by adoption: with none set, rule stays off", async () => {
    rows = [th("thr_live")];
    await dele.resolveVizier();
    dele.markSeen("mk", dele.pinned()[0]!.item);
    const id = await fileId();
    expect(await dele.ruleResolved(id, "project", "reversible and low risk", { threadId: "thr_live" })).toMatchObject({ ok: false, status: 403, error: expect.stringContaining("not enabled") });
    expect(dele.settings()).toEqual({ vizierThreadId: "thr_live" });
  });
});
