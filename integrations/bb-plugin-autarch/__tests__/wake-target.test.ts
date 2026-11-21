import { createWakeTargetResolver } from "../server.js";
import { describe, expect, it, vi } from "vitest";
import type { ThreadRow, VizierResolution } from "../delegation.js";
import { rerouteError, resolveWakeTarget, type RerouteDiagnostic } from "../wake-target.js";

const thread = (id: string, extra: Partial<ThreadRow> = {}): ThreadRow => ({ id, title: "Worker", archivedAt: null, pinnedAt: null, ...extra });

describe("wake target diagnostics preserve resolution", () => {
  it.each([
    { name: "live owner", rows: [thread("owner")], vizier: null, target: "owner" },
    { name: "unique successor", rows: [thread("owner", { archivedAt: 1 }), thread("next")], vizier: null, target: "next" },
    { name: "deleted owner", rows: [thread("owner", { deletedAt: 1 }), thread("next")], vizier: null, target: "next" },
    { name: "ambiguous successors", rows: [thread("owner", { archivedAt: 1 }), thread("one"), thread("two")], vizier: { ok: true, id: "vizier", via: "stored" }, target: "vizier" },
    { name: "archived candidate", rows: [thread("owner", { archivedAt: 1 }), thread("next", { archivedAt: 1 })], vizier: null, target: null },
    { name: "missing owner", rows: [], vizier: { ok: false, status: 409, error: "No candidate" }, target: null },
    { name: "vizier list unavailable", rows: [], vizier: { ok: false, status: 503, error: "List unavailable" }, target: null },
  ] satisfies { name: string; rows: ThreadRow[]; vizier: VizierResolution | null; target: string | null }[])("$name", async ({ rows, vizier, target }) => {
    const diagnostics: RerouteDiagnostic[] = [];
    let lists = 0, viziers = 0;
    const run = (report?: (d: RerouteDiagnostic) => void) => resolveWakeTarget("owner", async () => { lists++; return rows; }, async () => { viziers++; return vizier; }, report);
    expect(await run(d => diagnostics.push(d))).toBe(target);
    expect(await run()).toBe(target); // original thread-only contract
    expect(lists).toBe(2);
    expect(viziers).toBe(target === "owner" || target === "next" ? 0 : 2);
    if (target) expect(diagnostics).toEqual([]);
    else expect(diagnostics[0]).toMatchObject({ reason: vizier?.ok === false && vizier.status === 503 ? "vizier-list-unavailable" : "no-live-target",
      ownerState: rows.length ? "archived" : "missing", liveSuccessors: 0 });
  });

  it.each(["list", "vizier"])("records a %s exception without changing the null result", async stage => {
    let diagnostic: RerouteDiagnostic | undefined;
    const fail = async (): Promise<never> => { throw new Error("  lookup unavailable  \n at internal call  "); };
    expect(await resolveWakeTarget("owner", stage === "list" ? fail : async () => [], stage === "vizier" ? fail : async () => null, d => { diagnostic = d; })).toBeNull();
    expect(diagnostic).toMatchObject({ reason: stage === "list" ? "thread-list-error" : "vizier-error", error: "lookup unavailable" });
  });
});

it("bounds and redacts exception messages without storing stacks or objects", () => {
  expect(rerouteError(new Error("  failed token=placeholder Bearer placeholder https://example.test/?key=placeholder\n stack")))
    .toBe("failed token=[redacted]");
  expect(rerouteError("Authorization: Basic placeholder")).toBe("Authorization=[redacted]");
  expect(rerouteError("Bearer placeholder https://example.test/")).toBe("Bearer [redacted] [url]");
  expect(rerouteError(new Error("x".repeat(300)))).toHaveLength(240);
  expect(rerouteError({ message: " plain error " })).toBe("plain error");
  expect(rerouteError({ token: "placeholder" })).toBe("lookup failed");
});

it.each(["access_token", "refresh_token", "client_secret", "client-secret"])("redacts compound credential key %s", key => {
  expect(rerouteError(`${key}=placeholder`)).toBe(`${key}=[redacted]`);
});

it.each([
  { rows: [thread("owner", { deletedAt: 1 })], ownerState: "deleted", liveSuccessors: 0 },
  { rows: [thread("owner", { archivedAt: 1 }), thread("one"), thread("two")], ownerState: "archived", liveSuccessors: 2 },
])("records failed candidate details: $ownerState / $liveSuccessors", async ({ rows, ownerState, liveSuccessors }) => {
  let detail: RerouteDiagnostic | undefined;
  expect(await resolveWakeTarget("owner", async () => rows, async () => null, d => { detail = d; })).toBeNull();
  expect(detail).toMatchObject({ reason: "no-live-target", ownerState, liveSuccessors });
});

it.each([undefined, false, true])("server resolver keeps the fresh/cache and read-only vizier contract: %s", async fresh => {
  const list = vi.fn(async () => []);
  const vizier = vi.fn(async (_opts: { adopt?: boolean }): Promise<VizierResolution> => ({ ok: true, id: "vizier", via: "stored" }));
  const resolve = createWakeTargetResolver(list, () => ({ resolveVizier: vizier }));
  expect(await resolve("task", "owner", fresh)).toBe("vizier");
  expect(list).toHaveBeenCalledExactlyOnceWith({ cached: !(fresh ?? false) });
  expect(vizier).toHaveBeenCalledExactlyOnceWith({ adopt: false });
});

it("server resolver reads parts at call time and returns null before parts exist", async () => {
  const list = vi.fn(async () => []);
  const resolveVizier = vi.fn(async (): Promise<VizierResolution> => ({ ok: true, id: "vizier", via: "stored" }));
  let parts: { resolveVizier: typeof resolveVizier } | undefined;
  const resolve = createWakeTargetResolver(list, () => parts);
  expect(await resolve("task", "owner", true)).toBeNull();
  expect(resolveVizier).not.toHaveBeenCalled();
  parts = { resolveVizier };
  expect(await resolve("task", "owner", true)).toBe("vizier");
  expect(resolveVizier).toHaveBeenCalledExactlyOnceWith({ adopt: false });
  expect(list).toHaveBeenCalledTimes(2);
});
