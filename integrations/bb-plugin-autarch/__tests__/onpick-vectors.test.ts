import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import { hashes } from "./hash-golden-helper";
import { parseCard, toV1 } from "../cards";
import { canonicalJson, canonicalOnPick } from "../model";
import { createHash } from "node:crypto";
it("preserves opaque decoded ruling references in canonical declarations", () => {
  const value = { on_pick: { ruling: { bead: "bead-e\u0301", under: ["basis-e\u0301"] }, reversible: false } };
  expect(JSON.parse(canonicalJson(value))).toEqual(value);
  expect(canonicalJson(value)).not.toBe(canonicalJson(JSON.parse(JSON.stringify(value).normalize("NFC"))));
});
const rows = JSON.parse(readFileSync(new URL("../../../testdata/on-pick-vectors.json", import.meta.url), "utf8"));
for (const row of rows) it(`on-pick shared vectors ${row.name}`, () => {
  if (row.error) expect(() => parseCard(row.description)).toThrow();
  else if (row.text_field) {
    const option = toV1(parseCard(row.description), row.thread).options![0];
    const d = option.on_pick!;
    const actual = row.text_field === "instruction" ? option.instruction : row.text_field === "notification" ? d.notification : row.text_field === "bead" ? d.ruling.bead : d.ruling.under[0];
    expect(actual).toBe(row.text_value);
  }
  else {
    expect(hashes(row)).toEqual(row.expected);
    const declaration = JSON.parse(row.description.split("```home-ask\n")[1].split("\n```")[0]).options[0].on_pick;
    expect(canonicalOnPick(declaration)).toBe(row.declaration_canonical);
    expect(createHash("sha256").update(canonicalOnPick(declaration)).digest("hex")).toBe(row.declaration_sha256);
  }
});
it("all behavioral hashes distinguish opaque Unicode references", () => {
  const a = hashes(rows.find((r: any) => r.name === "opaque decomposed"));
  const b = hashes(rows.find((r: any) => r.name === "opaque composed"));
  for (const key of ["h1", "identity", "revision", "semantic_digest", "h5"] as const) expect(a[key]).not.toBe(b[key]);
});
