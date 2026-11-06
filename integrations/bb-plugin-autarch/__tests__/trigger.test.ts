import { existsSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { touchTrigger } from "../trigger.js";
import { TRIGGER_FILE } from "../updates.js";
import { tmpDir } from "./helpers.js";

let t: ReturnType<typeof tmpDir>;
afterEach(() => t?.cleanup());
function dir(): string {
  t = tmpDir();
  return t.dir;
}

describe("touchTrigger", () => {
  it("creates the trigger file when absent", () => {
    const d = dir();
    touchTrigger(d);
    expect(existsSync(join(d, TRIGGER_FILE))).toBe(true);
  });
  it("does not throw when the trigger file already exists", () => {
    const d = dir();
    touchTrigger(d);
    expect(() => touchTrigger(d)).not.toThrow();
    expect(existsSync(join(d, TRIGGER_FILE))).toBe(true);
  });
});
