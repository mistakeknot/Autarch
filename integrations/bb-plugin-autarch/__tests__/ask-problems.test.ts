import { describe, expect, it } from "vitest";
import { askProblems, normalizeAsk, ValidationError, type Ask } from "../model.js";

// Each fault breaks one independent field of a valid ask and names the field askProblems must report.
const faults: { field: string; apply: (a: Ask) => void }[] = [
  { field: "v", apply: (a) => void ((a as { v: number }).v = 2) },
  { field: "project", apply: (a) => void (a.project = "") },
  { field: "project_root", apply: (a) => void (a.project_root = "relative/path") },
  { field: "request_id", apply: (a) => void (a.request_id = "not a valid id!") },
  { field: "thread", apply: (a) => void (a.thread = "bad thread!") },
  { field: "subject", apply: (a) => void (a.subject = "s".repeat(121)) },
  { field: "mention_of", apply: (a) => void (a.mention_of = "bad mention!") },
  { field: "options[0].label", apply: (a) => void (a.options![0]!.label = "l".repeat(81)) },
];

const baseAsk = (): Ask => ({
  v: 1, kind: "decide", request_id: "", ask_key: "", subject: "", project: "Autarch", project_root: "/home/dev/projects/Autarch",
  asker: "thread", thread: "thr_abc123", question: "Collapse per project or per day?", recommendation: "", supersedes: "", mention_of: "",
  options: [{ id: "", label: "Per project", kind: "" as never, instruction: "" }, { id: "", label: "Per day", kind: "" as never, instruction: "" }],
  steps: [], machine: undefined,
});

describe("askProblems", () => {
  it("reports every injected fault, never fewer, and leaves the input alone (every subset of the faults)", () => {
    for (let mask = 0; mask < 1 << faults.length; mask++) {
      const a = baseAsk();
      const want: string[] = [];
      faults.forEach((f, i) => {
        if (mask & (1 << i)) {
          f.apply(a);
          want.push(f.field);
        }
      });
      const before = JSON.stringify(a);
      const ps = askProblems(a);
      expect(JSON.stringify(a), `mask ${mask} mutated its input`).toBe(before);
      const got = new Set(ps.map((p) => p.field));
      for (const f of want) expect(got.has(f), `mask ${mask}: no problem for ${f}`).toBe(true);
      expect(ps.length, `mask ${mask}`).toBeGreaterThanOrEqual(want.length);
      if (want.length === 0) {
        expect(ps).toEqual([]);
        continue;
      }
      let thrown: unknown;
      try {
        normalizeAsk(a);
      } catch (e) {
        thrown = e;
      }
      expect(thrown).toBeInstanceOf(ValidationError);
      expect((thrown as ValidationError).problems).toEqual(ps);
    }
  });

  it("leads the error message with the first problem and lists the rest after '; also: '", () => {
    const a = baseAsk();
    (a as { v: number }).v = 2;
    a.project = "";
    expect(() => normalizeAsk(a)).toThrow(/^v must be 1; also: project: project is required/);
  });
});
