import { spawnSync } from "node:child_process";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { decision, openStore, tmpDir } from "./helpers.js";

let t: ReturnType<typeof tmpDir>;
let file: string;
beforeEach(() => {
  t = tmpDir();
  file = join(t.dir, "data.db");
});
afterEach(() => t.cleanup());

const CHILD = join(import.meta.dirname, "store-crash-child.ts");
// node --import tsx runs the child directly, so the SIGKILL reaches the test as a signal.
const run = (...args: string[]) =>
  spawnSync(process.execPath, ["--import", "tsx", CHILD, file, ...args], {
    encoding: "utf8",
    cwd: join(import.meta.dirname, ".."),
  });

describe("crash safety", () => {
  it("recordPick killed between statements or after commit leaves both pick and obligations, or neither", () => {
    const s = openStore(file);
    const steps = ["pick-inserted", "obligation-row", "obligations-inserted", "event-inserted", "committed"];
    const ids: string[] = [];
    for (let i = 0; i < 50; i++) {
      const d = decision();
      s.insertDecision(d);
      ids.push(d.id);
    }
    ids.forEach((id, i) => {
      const r = run("pick", id, steps[i % steps.length]!);
      expect(r.signal, r.stderr).toBe("SIGKILL");
    });
    const outcomes = { none: 0, both: 0 };
    ids.forEach((id, i) => {
      const pick = s.pick(id);
      const obs = s.obligationsFor(id);
      const evs = s.events().filter((e) => e.type === "picked" && e.decision_id === id);
      if (!pick) {
        expect(obs).toHaveLength(0);
        expect(evs).toHaveLength(0);
        outcomes.none++;
      } else {
        expect(obs).toHaveLength(2);
        expect(evs).toHaveLength(1);
        outcomes.both++;
      }
      // "committed" is the only step after commit
      expect(Boolean(pick)).toBe(steps[i % steps.length] === "committed");
    });
    expect(outcomes.none).toBe(40);
    expect(outcomes.both).toBe(10);
  }, 120_000);

  it("a recipient set killed mid-transaction leaves none or all three", () => {
    const s = openStore(file);
    const d = decision();
    s.insertDecision(d);
    const r = run("recipients", d.id, "obligation-row");
    expect(r.signal, r.stderr).toBe("SIGKILL");
    expect(s.obligationsFor(d.id)).toHaveLength(0);
    // a complete run after the crash inserts all three; a retry then inserts none
    expect(run("recipients", d.id, "never").status).toBe(0);
    expect(s.obligationsFor(d.id)).toHaveLength(3);
    expect(run("recipients", d.id, "never").status).toBe(0);
    expect(s.obligationsFor(d.id)).toHaveLength(3);
  });

  it("a filing killed between the registry row and the decision leaves neither", () => {
    const s = openStore(file);
    const r = run("file", "d-crash", "registry-inserted");
    expect(r.signal, r.stderr).toBe("SIGKILL");
    expect(s.registry("req-d-crash")).toBeUndefined();
    expect(s.decision("d-crash")).toBeUndefined();
    expect(run("file", "d-crash", "never").status).toBe(0);
    expect(s.registry("req-d-crash")?.decision_id).toBe("d-crash");
    expect(s.decision("d-crash")).toBeDefined();
  });
});
