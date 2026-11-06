import { existsSync } from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { touchTrigger } from "../trigger.js";
import { TRIGGER_FILE } from "../updates.js";
import { tmpDir } from "./helpers.js";

const calls = vi.hoisted(() => ({ openSync: 0, closeSync: 0 }));
vi.mock("node:fs", async (importOriginal) => {
  const real = await importOriginal<typeof import("node:fs")>();
  return {
    ...real,
    openSync: (...a: Parameters<typeof real.openSync>) => {
      calls.openSync++;
      return real.openSync(...a);
    },
    closeSync: (...a: Parameters<typeof real.closeSync>) => {
      calls.closeSync++;
      return real.closeSync(...a);
    },
  };
});

let t: ReturnType<typeof tmpDir>;
beforeEach(() => {
  calls.openSync = 0;
  calls.closeSync = 0;
});
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
  it("opens and closes the file on every call, even when it already exists -- a skip-if-present optimization would stop the .path unit's PathChanged= from firing on repeat requests", () => {
    const d = dir();
    touchTrigger(d);
    touchTrigger(d);
    touchTrigger(d);
    expect(calls.openSync).toBe(3);
    expect(calls.closeSync).toBe(3);
  });
});
