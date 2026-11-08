// The vizier message service, driven through the plugin's real wiring (wireHome and the registered background
// services), with the store and the tasks client coming up late. Synthetic data only.
import Database from "better-sqlite3";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Store, type StoreHandle } from "../store.js";
import { wireHome, type HomeConfig, type VizierSettingsSource } from "../server.js";
import type { ServeClient } from "../serve.js";
import { FakeTasks } from "./tasks-fake.js";

const VIZ = "thr_viz";
const PREFIX = "Masaq' | vizier";
const serve = { projects: async () => [], health: async () => ({ build: { commit: "abc" } }), healthy: async () => true } as unknown as ServeClient;

interface Ev {
  id: string;
  seq: number;
  createdAt: number;
  type: string;
  data: object;
}
const ev = (seq: number, text: string, createdAt: number): Ev => ({
  id: `evt_${seq}`,
  seq,
  createdAt,
  type: "client/turn/requested",
  data: { initiator: "user", senderThreadId: null, input: [{ type: "text", text }] },
});

/** A handle whose store comes up only when the test says so, as a locked database does in the field. */
function lateHandle() {
  const listeners: ((s: Store) => void)[] = [];
  let current: Store | null = null;
  const handle: StoreHandle = {
    ready: () => current !== null,
    error: () => null,
    refusal: () => null,
    store() {
      if (!current) throw new Error("store not ready");
      return current;
    },
    onReady(fn) {
      if (current) fn(current);
      else listeners.push(fn);
    },
    dispose() {},
  };
  return {
    handle,
    open() {
      current = new Store(new Database(":memory:"));
      for (const fn of listeners) fn(current);
      return current;
    },
  };
}

function fixture(cfg: Partial<HomeConfig>, vizierSettings?: VizierSettingsSource) {
  const starts = new Map<string, (signal: AbortSignal) => Promise<void>>();
  const events: Ev[] = [];
  const tasks = new FakeTasks();
  const project = tasks.addProject("Vizier");
  const bb = {
    log: { info: vi.fn(), warn: vi.fn(), error: vi.fn() },
    onDispose: () => undefined,
    background: { service: (name: string, s: { start(signal: AbortSignal): Promise<void> }) => starts.set(name, (sig) => s.start(sig)) },
    events: { on: () => undefined },
    agents: { configure: () => undefined },
    cli: { register: () => undefined },
    realtime: { publish: vi.fn() },
    sdk: {
      plugins: tasks,
      threads: {
        list: async (a: { offset: number }) => (a.offset === 0 ? [{ id: VIZ, title: `${PREFIX} (main)`, pinnedAt: 1, archivedAt: null, deletedAt: null }] : []),
        events: {
          list: async (a: { threadId: string; afterSeq?: string; beforeSeq?: string; limit: string; order: "asc" | "desc" }) => {
            const after = a.afterSeq === undefined ? -Infinity : Number(a.afterSeq);
            const before = a.beforeSeq === undefined ? Infinity : Number(a.beforeSeq);
            const rows = events.filter((e) => a.threadId === VIZ && e.seq > after && e.seq < before).sort((x, y) => (a.order === "asc" ? x.seq - y.seq : y.seq - x.seq));
            return rows.slice(0, Number(a.limit));
          },
        },
      },
    },
  };
  const late = lateHandle();
  const full: HomeConfig = { serveAddr: "127.0.0.1:1", serveTokenFile: "/nope", serveProjectDirs: [], autarchBin: "autarch", vizierMessageCards: true, vizierTitlePrefix: PREFIX, vizierCardProject: "Vizier", ...cfg };
  wireHome(bb as never, late.handle, full, { serve, ...(vizierSettings ? { vizierSettings } : {}) });
  const ac = new AbortController();
  const run = [starts.get("home-queue")!(ac.signal), starts.get("home-vizier-messages")!(ac.signal)];
  const cards = () => tasks.tasks.filter((t) => t.projectId === project.id && t.description.includes("home-vizier-message:"));
  return { events, late, cards, tasks, project, bb, stop: async () => (ac.abort(), void (await Promise.all(run))) };
}

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
});

describe("the vizier message service, started before its store and tasks client are ready", () => {
  it("gives a card to a message sent between the enabled start and readiness, not to older history", async () => {
    vi.setSystemTime(1_000_000);
    const f = fixture({});
    f.events.push(ev(1, "history, typed before the plugin started", 900_000));
    await vi.advanceTimersByTimeAsync(3_000);
    // the operator types while the service is still waiting for the store
    f.events.push(ev(2, "typed while the plugin was coming up", Date.now()));
    await vi.advanceTimersByTimeAsync(4_000);
    expect(f.cards()).toHaveLength(0); // nothing can be filed yet
    f.late.open(); // readiness, several seconds after the setting was first seen on
    await vi.advanceTimersByTimeAsync(40_000);
    expect(f.cards().map((t) => t.title)).toEqual(["Vizier message: typed while the plugin was coming up"]);
    await f.stop();
  });

  it("files nothing while the setting is off", async () => {
    vi.setSystemTime(1_000_000);
    const f = fixture({ vizierMessageCards: false });
    f.late.open();
    f.events.push(ev(1, "typed with the setting off", Date.now()));
    await vi.advanceTimersByTimeAsync(60_000);
    expect(f.cards()).toHaveLength(0);
    expect(f.tasks.callsOf("createTask")).toHaveLength(0);
    await f.stop();
  });
});

/** A settings source the test changes between ticks, like `bb plugin config autarch set` does on the live plugin. */
function liveSettings(initial: { vizierMessageCards: boolean; vizierTitlePrefix: string; vizierCardProject: string }) {
  const state = { ...initial, failing: false, reads: 0 };
  const source: VizierSettingsSource = async () => {
    state.reads++;
    if (state.failing) throw new Error("settings store unavailable: secret-looking text");
    return { vizierMessageCards: state.vizierMessageCards, vizierTitlePrefix: state.vizierTitlePrefix, vizierCardProject: state.vizierCardProject };
  };
  return { state, source };
}
const OFF = { vizierMessageCards: false, vizierTitlePrefix: PREFIX, vizierCardProject: "Vizier" };
/** The saved switch-on time, or null when none is kept. */
const savedOnAt = (store: Store): number | null => {
  const v = JSON.parse(store.setting("vizierMessagePoll") ?? "null") as { onAt?: number } | null;
  return v?.onAt ?? null;
};
const titles = (f: { cards(): { title: string }[] }) => f.cards().map((t) => t.title);

describe("the vizier message service follows its settings on every tick, with no reload", () => {
  it("off to on: filing starts at the next tick, dated at that tick, with no backfill", async () => {
    vi.setSystemTime(1_000_000);
    const live = liveSettings(OFF);
    const f = fixture({ vizierMessageCards: false }, live.source);
    const store = f.late.open();
    f.events.push(ev(1, "typed long before the switch-on", Date.now() - 600_000));
    f.events.push(ev(2, "typed while off", Date.now()));
    await vi.advanceTimersByTimeAsync(40_000);
    expect(f.cards()).toHaveLength(0);
    expect(savedOnAt(store)).toBeNull(); // off keeps no activation time
    live.state.vizierMessageCards = true;
    f.events.push(ev(3, "typed after the flip, before the tick sees it", Date.now()));
    await vi.advanceTimersByTimeAsync(15_000); // one tick
    const onAt = savedOnAt(store)!;
    expect(onAt).toBeGreaterThan(1_040_000); // dated at the tick that saw it on, not at the start
    expect(onAt).toBeLessThanOrEqual(Date.now());
    expect(f.cards()).toHaveLength(0); // everything at or before that tick stays unfiled
    vi.setSystemTime(Date.now() + 1);
    f.events.push(ev(4, "typed after the activation", Date.now()));
    await vi.advanceTimersByTimeAsync(15_000);
    expect(titles(f)).toEqual(["Vizier message: typed after the activation"]);
    await f.stop();
  });

  it("on to off: filing stops at the next tick, the activation time is cleared, and a later on is dated afresh", async () => {
    vi.setSystemTime(1_000_000);
    const live = liveSettings({ ...OFF, vizierMessageCards: true });
    const f = fixture({ vizierMessageCards: true }, live.source);
    const store = f.late.open();
    await vi.advanceTimersByTimeAsync(1_000);
    vi.setSystemTime(Date.now() + 1);
    f.events.push(ev(1, "typed while on", Date.now()));
    await vi.advanceTimersByTimeAsync(15_000);
    expect(titles(f)).toEqual(["Vizier message: typed while on"]);
    live.state.vizierMessageCards = false;
    await vi.advanceTimersByTimeAsync(15_000);
    vi.setSystemTime(Date.now() + 1);
    f.events.push(ev(2, "typed after the flip off", Date.now()));
    await vi.advanceTimersByTimeAsync(45_000);
    expect(titles(f)).toEqual(["Vizier message: typed while on"]);
    expect(savedOnAt(store)).toBeNull();
    live.state.vizierMessageCards = true;
    await vi.advanceTimersByTimeAsync(15_000);
    expect(titles(f)).toEqual(["Vizier message: typed while on"]); // the message typed while off is not backfilled
    await f.stop();
  });

  it("a project change takes effect on the next tick", async () => {
    vi.setSystemTime(1_000_000);
    const live = liveSettings({ ...OFF, vizierMessageCards: true });
    const f = fixture({ vizierMessageCards: true }, live.source);
    const other = f.tasks.addProject("Elsewhere");
    f.late.open();
    await vi.advanceTimersByTimeAsync(1_000);
    vi.setSystemTime(Date.now() + 1);
    f.events.push(ev(1, "first, for the first project", Date.now()));
    await vi.advanceTimersByTimeAsync(15_000);
    expect(f.cards()).toHaveLength(1);
    live.state.vizierCardProject = "Elsewhere";
    vi.setSystemTime(Date.now() + 1);
    f.events.push(ev(2, "second, for the new project", Date.now()));
    await vi.advanceTimersByTimeAsync(15_000);
    const inOther = f.tasks.tasks.filter((t) => t.projectId === other.id && t.description.includes("home-vizier-message:"));
    // The new project has no marker for the first message, which is still inside the window and after the activation, so
    // it gets a card there too (the same as after a reload, which this change replaces). The old project keeps its card.
    expect(inOther.map((t) => t.title).sort()).toEqual(["Vizier message: first, for the first project", "Vizier message: second, for the new project"]);
    expect(f.cards().map((t) => t.title)).toEqual(["Vizier message: first, for the first project"]);
    await f.stop();
  });

  it("a title prefix change takes effect on the next tick", async () => {
    vi.setSystemTime(1_000_000);
    const live = liveSettings({ ...OFF, vizierMessageCards: true, vizierTitlePrefix: "No such thread" });
    const f = fixture({ vizierMessageCards: true }, live.source);
    f.late.open();
    await vi.advanceTimersByTimeAsync(1_000);
    vi.setSystemTime(Date.now() + 1);
    f.events.push(ev(1, "typed while the prefix matches nothing", Date.now()));
    await vi.advanceTimersByTimeAsync(30_000);
    expect(f.cards()).toHaveLength(0);
    live.state.vizierTitlePrefix = PREFIX;
    await vi.advanceTimersByTimeAsync(15_000);
    expect(titles(f)).toEqual(["Vizier message: typed while the prefix matches nothing"]);
    await f.stop();
  });

  it("a failed settings read files nothing, logs once, backs off, and recovers as a fresh switch-on", async () => {
    vi.setSystemTime(1_000_000);
    const live = liveSettings({ ...OFF, vizierMessageCards: true });
    const f = fixture({ vizierMessageCards: true }, live.source);
    const store = f.late.open();
    await vi.advanceTimersByTimeAsync(1_000);
    vi.setSystemTime(Date.now() + 1);
    f.events.push(ev(1, "typed while healthy", Date.now()));
    await vi.advanceTimersByTimeAsync(15_000);
    expect(f.cards()).toHaveLength(1);
    live.state.failing = true;
    await vi.advanceTimersByTimeAsync(15_000); // the tick that fails
    vi.setSystemTime(Date.now() + 1);
    f.events.push(ev(2, "typed while the read fails", Date.now()));
    const readsBefore = live.state.reads;
    await vi.advanceTimersByTimeAsync(5 * 60_000);
    expect(titles(f)).toEqual(["Vizier message: typed while healthy"]);
    expect(savedOnAt(store)).toBeNull(); // treated as off
    const warns = (f.bb.log.warn as ReturnType<typeof vi.fn>).mock.calls.map((c) => String(c[0])).filter((m) => m.includes("could not read the settings"));
    expect(warns).toHaveLength(1); // once, not once per tick
    expect(warns[0]).not.toContain("secret-looking"); // the error class only, never its text
    expect(live.state.reads - readsBefore).toBeLessThan(8); // 5 minutes at 15 s would be 20 reads; the backoff spaces them
    live.state.failing = false;
    await vi.advanceTimersByTimeAsync(5 * 60_000); // past the longest backoff
    expect(titles(f)).toEqual(["Vizier message: typed while healthy"]); // the message typed during the failure is not backfilled
    vi.setSystemTime(Date.now() + 1);
    f.events.push(ev(3, "typed after recovery", Date.now()));
    await vi.advanceTimersByTimeAsync(15_000);
    expect(titles(f)).toEqual(["Vizier message: typed while healthy", "Vizier message: typed after recovery"]);
    await f.stop();
  });

  it("a settings read that fails from the start files nothing", async () => {
    vi.setSystemTime(1_000_000);
    const live = liveSettings({ ...OFF, vizierMessageCards: true });
    live.state.failing = true;
    const f = fixture({ vizierMessageCards: true }, live.source);
    f.late.open();
    f.events.push(ev(1, "typed with the read failing", Date.now()));
    await vi.advanceTimersByTimeAsync(5 * 60_000);
    expect(f.cards()).toHaveLength(0);
    expect(f.tasks.callsOf("createTask")).toHaveLength(0);
    await f.stop();
  });
});
