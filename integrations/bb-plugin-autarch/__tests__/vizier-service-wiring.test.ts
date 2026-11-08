// The vizier message service, driven through the plugin's real wiring (wireHome and the registered background
// services), with the store and the tasks client coming up late. Synthetic data only.
import Database from "better-sqlite3";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Store, type StoreHandle } from "../store.js";
import { wireHome, type HomeConfig } from "../server.js";
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

function fixture(cfg: Partial<HomeConfig>) {
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
  wireHome(bb as never, late.handle, full, { serve });
  const ac = new AbortController();
  const run = [starts.get("home-queue")!(ac.signal), starts.get("home-vizier-messages")!(ac.signal)];
  const cards = () => tasks.tasks.filter((t) => t.projectId === project.id && t.description.includes("home-vizier-message:"));
  return { events, late, cards, tasks, stop: async () => (ac.abort(), void (await Promise.all(run))) };
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
