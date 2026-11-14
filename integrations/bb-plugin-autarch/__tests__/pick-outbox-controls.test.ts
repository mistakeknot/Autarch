import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { join } from "node:path";
import { decision, openStore, pickOf, tmpDir } from "./helpers.js";

const SENTINEL = "9999-12-31T23:59:59.999Z";
const NOW = "2030-01-01T00:00:00.000Z";
let temp: ReturnType<typeof tmpDir>;
let store: ReturnType<typeof openStore>;
let decisionId: string;
beforeEach(() => {
  temp = tmpDir();
  store = openStore(join(temp.dir, "data.db"), { now: () => NOW });
  const d = decision();
  decisionId = d.id;
  store.insertDecision(d);
});
afterEach(() => { store.close(); temp.cleanup(); });

function protectedRow(kind: string, state = "pending") {
  store.db.prepare(`INSERT INTO obligations
    (id, decision_id, kind, state, recipient, payload, op, next_try_at, handler_next_try_at,
     updated_at, allow_reroute, allow_batch, require_message_id)
    VALUES ('protected', ?, ?, ?, 'target', 'exact payload', 'protected-op', ?, ?, ?, 0, 0, 1)`)
    .run(decisionId, kind, state, SENTINEL, NOW, NOW);
}

describe.each(["pick-relay", "pick-receipt"])("protected outbox controls: %s", kind => {
  it("carried note 2: generic skip-duplicates insertion refuses protected kinds before SQL", () => {
    // An exact existing row would otherwise hit migration 7's ABORT trigger.
    protectedRow(kind);
    const before = store.obligation("protected");
    expect(() => store.insertObligations(decisionId, [
      { id: "ordinary", kind: "wake" },
      { id: "protected", kind, recipient: "target", payload: "exact payload", op: "protected-op", next_try_at: SENTINEL },
    ])).toThrow("protected obligations require guarded insertion");
    expect(store.obligation("ordinary")).toBeUndefined();
    expect(store.obligation("protected")).toEqual(before);
  });

  it("ordinary pick insertion cannot accidentally create protected work", () => {
    expect(() => store.recordPick(pickOf(decisionId), [
      { id: "protected", kind, next_try_at: SENTINEL },
    ])).toThrow("protected obligations require guarded insertion");
    expect(store.pick(decisionId)).toBeUndefined();
    expect(store.obligation("protected")).toBeUndefined();
  });

  it("retry and settlement preserve the legacy sentinel while moving only real due time", () => {
    protectedRow(kind, "sending");
    const later = "2030-01-01T00:00:30.000Z";
    expect(() => {
      expect(store.transition("protected", "sending", "pending", 0, { next_try_at: later })).toEqual({ ok: true });
    }).not.toThrow();
    expect(store.obligation("protected")).toMatchObject({ next_try_at: SENTINEL, handler_next_try_at: later });
    expect(store.transition("protected", "pending", "done", 0, { next_try_at: NOW })).toEqual({ ok: true });
    expect(store.obligation("protected")).toMatchObject({ next_try_at: SENTINEL, handler_next_try_at: NOW });
  });

  it("no-reroute policy refuses generic retarget without changing immutable recipient", () => {
    protectedRow(kind);
    expect(store.retarget("protected", "target", "successor")).toBe(false);
    expect(store.obligation("protected")?.recipient).toBe("target");
  });

  it("generic Resend cannot grant a blind retry of ambiguous protected acceptance", () => {
    protectedRow(kind, "uncertain");
    const before = store.obligation("protected");
    expect(store.resend("protected", 0, "click")).toEqual({ ok: false, reason: "stale" });
    expect(store.obligation("protected")).toEqual(before);
  });
});

it("legacy retries, rerouting and explicit uncertain resend keep their existing behavior", () => {
  store.insertObligations(decisionId, [{ id: "legacy", kind: "wake", recipient: "target" }]);
  expect(store.transition("legacy", "pending", "uncertain", 0, { next_try_at: NOW })).toEqual({ ok: true });
  expect(store.retarget("legacy", "target", "successor")).toBe(true);
  expect(store.resend("legacy", 0, "click")).toEqual({ ok: true, replay: false });
  expect(store.obligation("legacy")).toMatchObject({ next_try_at: NOW, handler_next_try_at: null, recipient: "successor" });
});
