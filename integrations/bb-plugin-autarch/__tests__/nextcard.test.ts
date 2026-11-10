import { describe, expect, it } from "vitest";
import { neighborAfterRemoval as next } from "../ui/nextcard.js";

describe("neighborAfterRemoval", () => {
  it("takes the card below", () => expect(next(["a", "b", "c"], "b", ["a", "c"])).toBe("c"));
  it("takes the card above when it was last", () => expect(next(["a", "b", "c"], "c", ["a", "b"])).toBe("b"));
  it("takes the second card when the first goes", () => expect(next(["a", "b", "c"], "a", ["b", "c"])).toBe("b"));
  it("is null when it was the only card", () => expect(next(["a"], "a", [])).toBeNull());
  it("skips cards below that also left, then looks above", () => {
    expect(next(["a", "b", "c", "d"], "b", ["a", "d"])).toBe("d");
    expect(next(["a", "b", "c"], "b", ["a"])).toBe("a");
  });
  it("is null when the card was not in the list as it was", () => expect(next(["a", "b"], "x", ["a", "b"])).toBeNull());
  it("ignores cards that arrived after the list was seen", () => expect(next(["a", "b", "c"], "b", ["a", "n", "c"])).toBe("c"));
});
