import { afterEach, expect, it } from "vitest";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { appendRuling, readMatchingRuling, type LedgerPick } from "../ledger.js";
import { tmpDir } from "./helpers.js";

const dirs: ReturnType<typeof tmpDir>[] = [];
afterEach(() => { while (dirs.length) dirs.pop()!.cleanup(); });
const pick: LedgerPick = { pick_id: "pick", picked_at: "2030-01-01T00:00:00Z", by: "mk", surface: "home", card: "CARD-1", subject: "Colour", option_id: "a", option_label: "Blue" };
it("focused readback requires the complete matching selected static row", () => {
  const dir = tmpDir(); dirs.push(dir);
  const path = join(dir.dir, "ledger.jsonl");
  expect(readMatchingRuling(null, pick)).toBe("disabled");
  expect(readMatchingRuling(path, pick)).toBe("missing");
  appendRuling(path, pick);
  expect(readMatchingRuling(path, pick)).toBe("matching");
  expect(readMatchingRuling(path, { ...pick, option_label: "Changed" })).toBe("conflicting");
  writeFileSync(path, '{"id":"home-pick", broken\n');
  expect(readMatchingRuling(path, pick)).toBe("unknown");
});
it("unrelated malformed content is not repaired and does not replace selected-row evidence", () => {
  const dir = tmpDir(); dirs.push(dir);
  const path = join(dir.dir, "ledger.jsonl");
  writeFileSync(path, "unrelated broken line\n");
  appendRuling(path, pick);
  expect(readMatchingRuling(path, pick)).toBe("matching");
  expect(readMatchingRuling(dir.dir, pick)).toBe("unknown");
});
it("a malformed line naming the selected id through JSON escapes is unknown, never a false match", () => {
  const dir = tmpDir(); dirs.push(dir);
  const path = join(dir.dir, "ledger.jsonl");
  appendRuling(path, pick);
  writeFileSync(path, '{"id":"home-\\u0070ick", "words": broken\n', { flag: "a" });
  expect(readMatchingRuling(path, pick)).toBe("unknown");
  // A well-formed escaped spelling of the same complete row is the same row.
  const escaped = join(dir.dir, "escaped.jsonl");
  appendRuling(escaped, pick);
  writeFileSync(escaped, readFileSync(escaped, "utf8").replace('"home-pick"', '"home-\\u0070ick"'));
  expect(readFileSync(escaped, "utf8")).not.toContain('"home-pick"');
  expect(readMatchingRuling(escaped, pick)).toBe("matching");
});
it("a torn selected row and a duplicate that differs are not matching", () => {
  const dir = tmpDir(); dirs.push(dir);
  const torn = join(dir.dir, "torn.jsonl");
  writeFileSync(torn, '{"id":"home-pi');
  expect(readMatchingRuling(torn, pick)).toBe("unknown");
  const dup = join(dir.dir, "dup.jsonl");
  appendRuling(dup, pick);
  writeFileSync(dup, `${JSON.stringify({ id: "home-pick", ts: pick.picked_at })}\n`, { flag: "a" });
  expect(readMatchingRuling(dup, pick)).toBe("conflicting");
  const extra = join(dir.dir, "extra.jsonl");
  appendRuling(extra, pick);
  writeFileSync(extra, "");
  appendRuling(extra, { ...pick, pick_id: "other" });
  expect(readMatchingRuling(extra, pick)).toBe("missing");
});
