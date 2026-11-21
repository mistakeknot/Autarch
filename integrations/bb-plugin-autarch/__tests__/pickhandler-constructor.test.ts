import { afterEach, expect, it } from "vitest";
import Database from "better-sqlite3";
import { Store } from "../store.js";
import { Service, type ServiceDeps } from "../service.js";
import { cleanupEnvs } from "./card-rig.js";
import { recoveryRig } from "./pickhandler-stage4-rig.js";

const stores: Store[] = [];
const temps: { cleanup(): void }[] = [];
const now = () => "2026-10-01T00:00:00.000Z";
afterEach(() => {
  while (stores.length) stores.pop()!.close();
  cleanupEnvs();
  while (temps.length) temps.pop()!.cleanup();
});
function control(store: Store) {
  return store.db.prepare("SELECT * FROM pick_handler_control WHERE id = 1").get() as { mode: string; epoch: number };
}
function seeded(mode: string) {
  const store = new Store(new Database(":memory:"), { now }); stores.push(store);
  store.db.prepare(`UPDATE pick_handler_control SET mode = ?, epoch = epoch + 1,
    owner_enablement_json = '{"fixture":"reviewed"}', reviewed_build = 'test-build',
    reviewed_scope_json = '{}', enabled_cells_json = '["instruction:none"]' WHERE id = 1`).run(mode);
  return store;
}
function construct(store: Store, deps: Partial<ServiceDeps> = {}) {
  return new Service({ store, now, projects: async () => [], env: {}, ...deps });
}

it.each([undefined, "off", "shadow"])("production constructor with config %s invalidates existing isolated live control", pickHandlerMode => {
  const store = seeded("live");
  const before = control(store);
  construct(store, { pickHandlerMode });
  expect(control(store)).toEqual({ ...before, mode: pickHandlerMode === "shadow" ? "shadow" : "off", epoch: before.epoch + 1, updated_at: now() });
});

it("isolated live constructor preserves exact owner control, epoch and captured snapshot until the original deadline", async () => {
  const { r, g, temp } = await recoveryRig(); temps.push(temp);
  expect((await r.validatedMkPick(g.id, "constructor-restart")).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const beforeControl = control(r.svc.store);
  const snapshot = r.db.prepare("SELECT * FROM pick_actions WHERE decision_id = ?").get(g.id);
  const obligations = r.svc.store.obligationsFor(g.id);
  expect(beforeControl.mode).toBe("live");
  expect(snapshot).toMatchObject({ capture_mode: "live", capture_epoch: beforeControl.epoch, disposition: "automatic",
    evaluation_deadline_at: new Date(r.env.clock.t + 120_000).toISOString() });
  const reopened = new Store(new Database(r.env.file), { now: r.env.now }); stores.push(reopened);
  const svc = construct(reopened, { now: r.env.now, pickCaptureRuntime: { mode: () => "live" } });
  expect(control(reopened)).toEqual(beforeControl);
  expect(reopened.db.prepare("SELECT * FROM pick_actions WHERE decision_id = ?").get(g.id)).toEqual(snapshot);
  expect(svc.expirePickEvaluations()).toBe(0);
  expect(reopened.obligationsFor(g.id)).toEqual(obligations);
  r.advance(120_000);
  expect(svc.expirePickEvaluations()).toBe(1);
  expect(reopened.db.prepare("SELECT * FROM pick_actions WHERE decision_id = ?").get(g.id)).toMatchObject({ disposition: "fallback",
    evaluation_deadline_at: new Date(r.env.clock.t).toISOString() });
  expect(reopened.obligationsFor(g.id).find(o => o.op?.endsWith(":fallback"))?.payload).toContain("evaluation-timeout");
});

it.each([undefined, "off", "shadow", "live"].flatMap(runtime => ["off", "shadow", "live"].map(persisted => ({ runtime, persisted }))))(
  "live config is rejected without mutation for $runtime runtime and $persisted control", ({ runtime, persisted }) => {
    const store = seeded(persisted);
    const before = control(store);
    expect(() => construct(store, { pickHandlerMode: "live", ...(runtime === undefined ? {} : { pickCaptureRuntime: { mode: () => runtime } }) }))
      .toThrow("pickHandlerMode must be off or shadow in PR3");
    expect(control(store)).toEqual(before);
  },
);

const policies = [undefined, "off", "shadow"];
it.each(["off", "shadow"].flatMap(persisted => policies.map(config => ({ persisted, config }))))(
  "isolated live constructor cannot enable $persisted control with config $config", ({ persisted, config }) => {
    const store = seeded(persisted);
    const before = control(store);
    construct(store, { pickHandlerMode: config, pickCaptureRuntime: { mode: () => "live" } });
    const mode = config === "shadow" ? "shadow" : "off";
    expect(control(store)).toEqual({ ...before, mode, epoch: before.epoch + Number(persisted !== mode),
      ...(persisted !== mode ? { updated_at: now() } : {}) });
  },
);

it.each(["off", "shadow"].flatMap(runtime => ["off", "shadow", "live"].flatMap(persisted => policies.map(config => ({ runtime, persisted, config })))))(
  "$runtime runtime follows production policy for $persisted control and config $config", ({ runtime, persisted, config }) => {
    const store = seeded(persisted);
    const before = control(store);
    construct(store, { pickHandlerMode: config, pickCaptureRuntime: { mode: () => runtime } });
    const mode = config === "shadow" ? "shadow" : "off";
    expect(control(store)).toEqual({ ...before, mode, epoch: before.epoch + Number(persisted !== mode),
      ...(persisted !== mode ? { updated_at: now() } : {}) });
  },
);
