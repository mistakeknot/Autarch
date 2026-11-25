// Card convention vectors, shared byte for byte with the Go parser
// (internal/homeask/card_test.go reads the same fixture files).
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { askingThread, cardFingerprint, parseCard, toV1, type TaskComment } from "../cards";
import { normalizedJson, parseAsk, identity, revision, semanticKey } from "../model";
import { Service } from "../service";
import { readStoredAsk } from "../askbody";

const dir = join(dirname(fileURLToPath(import.meta.url)), "fixtures", "cards");
const docs = readdirSync(dir)
  .filter((f) => f.endsWith(".json"))
  .sort()
  .map((f) => ({ file: f, doc: JSON.parse(readFileSync(join(dir, f), "utf8")) }));

interface CardCase {
  name: string;
  description: string;
  expect: Record<string, any>;
}

describe("card vectors (shared with Go)", () => {
  let total = 0;
  for (const { file, doc } of docs) {
    if (doc.kind !== "card") continue;
    for (const c of doc.cases as CardCase[]) {
      total++;
      it(`${file}: ${c.name}`, () => {
        if (typeof c.expect.error === "string") {
          expect(() => parseCard(c.description)).toThrow(c.expect.error);
          return;
        }
        const card = parseCard(c.description);
        expect(card.question).toBe(c.expect.question);
        expect(card.blocks).toEqual(c.expect.blocks);
        expect(card.request).toEqual(c.expect.request);
        expect(card.pull).toBe(c.expect.pull);
        expect(card.root_run).toEqual(c.expect.root_run);
        for (const want of c.expect.v1 ?? []) {
          const a = toV1(card, want.thread);
          expect(a.asker).toBe(want.asker);
          expect(a.kind).toBe("decide");
          // The output passes the unchanged rev-4 parseAsk and is a fixed point of it.
          const again = parseAsk(JSON.parse(JSON.stringify(a)));
          expect(normalizedJson(again)).toBe(normalizedJson(a));
          expect(identity(a)).toBe(want.identity);
          expect(revision(a)).toBe(want.revision);
          expect(semanticKey(a)).toBe(want.semantic_key);
        }
      });
    }
  }
  it("has the expected volume", () => expect(total).toBeGreaterThanOrEqual(60));
});

describe("toV1", () => {
  const ask = { project: "p", project_root: "/r", question: "q", options: [{ id: "a", label: "A", kind: "ruling-only" }, { id: "b", label: "B", kind: "ruling-only" }] };
  it("a pull:mycroft card ignores the thread", () => {
    const a = toV1({ pull: "mycroft", ask } as any, "thr_x");
    expect(a.asker).toBe("mycroft");
    expect(a.thread ?? "").toBe("");
  });
  it("a thread card without a thread is refused", () => {
    expect(() => toV1({ pull: "", ask } as any, "")).toThrow();
  });
});

describe("askingThread vectors (shared with Go)", () => {
  let n = 0;
  for (const { doc } of docs) {
    if (doc.kind !== "asking-thread") continue;
    for (const c of doc.cases as { name: string; comments: TaskComment[]; expect: string }[]) {
      n++;
      it(c.name, () => {
        const before = JSON.stringify(c.comments);
        expect(askingThread(c.comments)).toBe(c.expect);
        expect(JSON.stringify(c.comments)).toBe(before);
      });
    }
  }
  it("has the expected volume", () => expect(n).toBeGreaterThanOrEqual(6));
});

const outcomes: { name: string; wire?: Record<string, unknown>; stored?: Record<string, unknown>; canonical?: string; error?: boolean }[] =
  JSON.parse(readFileSync(new URL("../../../testdata/expected-outcome-vectors.json", import.meta.url), "utf8"));
const outcomeCard = (wire: Record<string, unknown>) =>
  parseCard("Proceed?\nBlocks: thread:thr_fixture\nRequest: fixture sha256:0123456789abcdef\n```home-ask\n" + JSON.stringify(wire) + "\n```");

describe("expected outcomes (shared with Go)", () => {
  for (const row of outcomes) it(row.name, () => {
    if (row.stored) {
      expect(readStoredAsk(JSON.stringify(row.stored), "card")).toBeNull();
      return;
    }
    if (row.error) {
      expect(() => outcomeCard(row.wire!)).toThrow();
      return;
    }
    const body = normalizedJson(toV1(outcomeCard(row.wire!), "thr_fixture"));
    expect(body).toBe(row.canonical);
    expect(normalizedJson(readStoredAsk(body, "card")!)).toBe(body);
    if (row.name !== "absent") expect(readStoredAsk(body, "home")).toBeNull();
  });

  it("preserves the original generation's selected check after a card edit", () => {
    const original = outcomes.find(row => row.name === "merge")!;
    const amended = structuredClone(original.wire) as { options: { expected_outcome?: { by_when: { after_seconds: number } } }[] };
    amended.options[0]!.expected_outcome!.by_when.after_seconds = 172800;
    const project = "fixture-project";
    const thread = "thr_fixture";
    const snapshot = (wire: Record<string, unknown>, generation: number, supersedes: string | null) => {
      const card = outcomeCard(wire);
      const fingerprint = cardFingerprint({ title: "Fixture", card, thread, tasksProject: project });
      // Use the actual generation projection without a live service or database.
      const genInput = Service.prototype["genInput"];
      return genInput.call({}, { id: "fixture", projectId: project } as Parameters<typeof genInput>[0], card, thread, fingerprint, generation, supersedes,
        { project_root: "/fixture", root_dev: "1", root_ino: "2", root_fsid: null });
    };
    const gen1 = snapshot(original.wire!, 1, null);
    const gen2 = snapshot(amended, 2, gen1.id);
    expect(gen2.supersedes).toBe(gen1.id);
    expect(gen2.revision).not.toBe(gen1.revision);
    expect(gen2.card_fp).not.toBe(gen1.card_fp);
    const originalBody = readStoredAsk(gen1.body_json, "card")!;
    const amendedBody = readStoredAsk(gen2.body_json, "card")!;
    const selected = (body: typeof originalBody) => JSON.parse(normalizedJson(body)).options.find((o: { id: string }) => o.id === "yes").expected_outcome;
    expect(selected(originalBody).by_when).toEqual({ after_seconds: 86400 });
    expect(selected(amendedBody).by_when).toEqual({ after_seconds: 172800 });
  });
});
