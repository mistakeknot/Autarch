import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { expect, it } from "vitest";
import { validateEvidence } from "../evidence.js";
import { defaultEvidenceRoot } from "../evidencestore.js";
import { parseAsk } from "../model.js";

const vectors=JSON.parse(readFileSync(fileURLToPath(new URL("../../../testdata/evidence-vectors.json",import.meta.url)),"utf8")) as {name:string;evidence:unknown;valid:boolean}[];
it.each(vectors)("shared evidence vector: $name", v => {
 const validate=()=>validateEvidence(v.evidence);
 const parse=()=>parseAsk({v:1,kind:"decide",project:"Example",project_root:"/example",asker:"thread",thread:"thr_test",question:"Which?",options:[{id:"a",label:"A",kind:"ruling-only"},{id:"b",label:"B",kind:"ruling-only"}],evidence:v.evidence});
 for (const fn of [validate,parse]) { if (v.valid) expect(fn).not.toThrow(); else expect(fn).toThrow(); }
 if (v.valid) {
  for (const item of validateEvidence(v.evidence)) {
   for (const source of [item,item.before,item.after]) {
    if (source?.url) expect(() => new URL(source.url!)).not.toThrow();
   }
  }
 }
});
it("uses the default evidence root for empty and relative overrides", () => {
 const expected=defaultEvidenceRoot({});
 expect(defaultEvidenceRoot({AUTARCH_HOME_EVIDENCE:""})).toBe(expected);
 expect(defaultEvidenceRoot({AUTARCH_HOME_EVIDENCE:"relative/evidence"})).toBe(expected);
 const absolute=join(tmpdir(),"absolute-evidence");
 expect(defaultEvidenceRoot({AUTARCH_HOME_EVIDENCE:absolute})).toBe(absolute);
});
