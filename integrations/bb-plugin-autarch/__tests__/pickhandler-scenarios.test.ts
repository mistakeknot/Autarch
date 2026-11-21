import { expect, it } from "vitest";
import { pickHandlerScenarios } from "../e2e/scenarios/pick-handler.js";
import { SCHEMAS } from "../scripts/check-e2e.mjs";

// Socket-free scenario bodies only; exact-commit/full harness qualification belongs to the native wrapper.
it.each(Object.entries(pickHandlerScenarios))("%s produces checked runtime evidence", async (name, run) => {
  const evidence = await run();
  const schema = Reflect.get(SCHEMAS, name) as Record<string, (value: unknown) => boolean>;
  expect(schema).toBeDefined();
  for (const [field, check] of Object.entries(schema)) {
    expect(check(Reflect.get(evidence, field)), `${name}.${field}: ${JSON.stringify(Reflect.get(evidence, field))}`).toBe(true);
    expect(check(undefined), `${name}.${field} must reject missing evidence`).toBe(false);
  }
});
