import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Delegation } from "../delegation.js";
import { FeedCaches } from "../feed.js";
import type { Service } from "../service.js";
import { ask, makeEnv, OPTIONS, type Env } from "./service-helpers.js";

let env: Env;
let svc: Service;
let caches: FeedCaches;
let dele: Delegation;
const now = () => env.clock.t;
beforeEach(() => {
  env = makeEnv(["Autarch", "Other"]);
  caches = new FeedCaches(() => svc.store.db, now);
  svc = env.open({ nudge: () => caches.invalidate() });
  dele = new Delegation(svc);
});
afterEach(() => env.cleanup());

let n = 0;
const fileAndPick = async (thread: string, option = "day", over: Record<string, unknown> = {}, by = "mk") => {
  n += 1;
  const r = await svc.file(ask(env, { thread, question: `Question ${n} for ${thread}?`, subject: `s${n}`, ...over }), {});
  if (!r.ok) throw new Error(r.error);
  const rev = (svc.store.decision(r.decision_id) as { revision: string }).revision;
  const p = svc.pick(r.decision_id, option, rev, `pk-${n}`, by, "home");
  if (!p.ok) throw new Error(p.error);
  return r.decision_id;
};

describe("configure [D-17]", () => {
  it("two threads in one project each get the project lines and only their own, own first", async () => {
    const a = await fileAndPick("thr-a");
    const b = await fileAndPick("thr-b");
    const ta = caches.configure("Autarch", "thr-a")!;
    const tb = caches.configure("Autarch", "thr-b")!;
    for (const t of [ta, tb]) {
      expect(t).toContain(a);
      expect(t).toContain(b);
      expect(t.startsWith("Recent rulings (label only):")).toBe(true);
    }
    expect(ta.indexOf(`(${a})`)).toBeLessThan(ta.indexOf(`(${b})`));
    expect(tb.indexOf(`(${b})`)).toBeLessThan(tb.indexOf(`(${a})`));
    const tc = caches.configure("Autarch", "thr-c")!;
    expect(tc).toContain(a);
    expect(tc).toContain(b);
  });

  it("carries the filing note and never a question or instruction", async () => {
    await fileAndPick("thr-a", "project");
    const t = caches.configure("Autarch", "thr-a")!;
    expect(t).toContain("bb home ask --request-stdin");
    expect(t).toContain("needs-context");
    expect(t).toContain("reversible");
    expect(t).not.toContain("Question 1");
    expect(t).not.toContain("run npm test");
  });

  it("a thread starting before any refresh still gets its project's existing rulings", async () => {
    const a = await fileAndPick("thr-a");
    const fresh = new FeedCaches(() => svc.store.db, now);
    expect(fresh.configure("Autarch", "thr-brand-new")).toContain(a);
  });

  it("stays within 4096 UTF-16 units with non-BMP labels", async () => {
    for (let i = 0; i < 30; i++) {
      await fileAndPick("thr-a", "day", { options: [{ id: "day", label: "\u{1F600}".repeat(39), kind: "ruling-only" }, { id: "x", label: "X", kind: "ruling-only" }] });
    }
    const t = caches.configure("Autarch", "thr-a")!;
    expect(t.length).toBeLessThanOrEqual(4096);
    expect(t.isWellFormed()).toBe(true);
    expect(t).toContain("bb home ask --request-stdin");
  });

  it("an overridden ruling leaves the project lines at once, without waiting 30 s", async () => {
    dele.setDelegation({ vizierThreadId: "thr-vizier", projects: ["Autarch"], dailyCap: 5 }, {});
    dele.markSeen("mk", dele.latestSettingsItem()!);
    const w = await fileAndPick("thr-a", "project", {}, "vizier");
    expect(caches.configure("Autarch", "thr-c")).toContain(`(${w})`);
    const o = dele.override(w, {});
    expect(o.ok).toBe(true);
    const after = caches.configure("Autarch", "thr-c");
    expect(after === undefined || !after.includes(`(${w})`)).toBe(true);
  });

  it("returns nothing only when both caches are empty for that project", async () => {
    expect(caches.configure("Autarch", "thr-a")).toBeUndefined();
    await fileAndPick("thr-a");
    expect(caches.configure("Other", "thr-a")).toBeUndefined();
    expect(caches.configure("Autarch", "thr-zzz")).toBeDefined();
  });

  it("refresh rebuilds the caches it holds after the window moves", async () => {
    const a = await fileAndPick("thr-a");
    expect(caches.configure("Autarch", "thr-c")).toContain(a);
    env.clock.t += 20 * 86_400_000;
    caches.refresh();
    expect(caches.configure("Autarch", "thr-c")).toBeUndefined();
    void OPTIONS;
  });
});
