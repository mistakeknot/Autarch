// Card convention vectors, shared byte for byte with the Go parser
// (internal/homeask/card_test.go reads the same fixture files).
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { askingThread, parseCard, toV1, type TaskComment } from "../cards";
import { normalizedJson, parseAsk, identity, revision, semanticKey } from "../model";

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
