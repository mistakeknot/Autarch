import { describe, expect, it } from "vitest";
import { readableCatchup, type CatchupEntry } from "../ui/catchup.js";

const owed = (decision: string): CatchupEntry => ({ item: `owed:${decision}`, kind: "owed", at: "1", text: `Waiting for you: ${decision}`, decision });

describe("Since you left lists only asks that are waiting", () => {
  it("drops a Waiting-for-you row whose ask is set aside to Later or on hold, keeps an open one", () => {
    const rows = [owed("a"), owed("b"), owed("c"), { item: "n", kind: "notice" as const, at: "1", text: "n" }];
    const out = readableCatchup(rows, [{ id: "a", later: true }, { id: "b", held: true }, { id: "c" }]);
    expect(out.map((r) => r.item)).toEqual(["owed:c"]);
  });
  it("keeps owed rows when the asks have not loaded", () => {
    expect(readableCatchup([owed("a")], null)).toHaveLength(1);
  });
});
