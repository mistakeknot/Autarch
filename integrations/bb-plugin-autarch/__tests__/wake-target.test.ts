import { describe, expect, it } from "vitest";
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
    else expect(diagnostics[0]).toMatchObject({ reason: vizier?.ok === false && vizier.status === 503 ? "thread-list-unavailable" : "no-live-target",
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
