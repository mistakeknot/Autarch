import { createHash } from "node:crypto";
import { chmodSync, mkdtempSync, mkdirSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { expect, it } from "vitest";
import { validateEvidence } from "../evidence.js";
import { defaultEvidenceRoot, readEvidence } from "../evidencestore.js";
import { parseAsk } from "../model.js";

const vectors=JSON.parse(readFileSync(fileURLToPath(new URL("../../../testdata/evidence-vectors.json",import.meta.url)),"utf8")) as {name:string;evidence?:unknown;raw_evidence?:string;valid:boolean}[];
it("loads at least the Go floor of 50 shared vectors", () => { expect(vectors.length).toBeGreaterThanOrEqual(50); });
it.each(vectors)("shared evidence vector: $name", v => {
 const evidence=v.raw_evidence===undefined ? v.evidence : JSON.parse(v.raw_evidence);
 const validate=()=>validateEvidence(evidence);
 const parse=()=>parseAsk({v:1,kind:"decide",project:"Example",project_root:"/example",asker:"thread",thread:"thr_test",question:"Which?",options:[{id:"a",label:"A",kind:"ruling-only"},{id:"b",label:"B",kind:"ruling-only"}],evidence});
 for (const fn of [validate,parse]) { if (v.valid) expect(fn).not.toThrow(); else expect(fn).toThrow(); }
 if (v.valid) {
  for (const item of validateEvidence(evidence)) {
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

it.each([
 ["/store/", "/store"],
 ["/link/../store//", "/store"],
 ["//store/./", "/store"],
 ["/../../", "/"],
])("cleans evidence override %s like Go filepath.Clean", (input, expected) => {
 expect(defaultEvidenceRoot({AUTARCH_HOME_EVIDENCE:input})).toBe(expected);
});
it("reads the lexically cleaned root and rejects symlinked and nonprivate roots", () => {
 const base=mkdtempSync(join(tmpdir(),"evidence-parity-"));
 const store=join(base,"store"), link=join(base,"link");
 const image=Buffer.from("iVBORw0KGgo=","base64");
 const hash=createHash("sha256").update(image).digest("hex");
 try {
  mkdirSync(store,{mode:0o700});
  mkdirSync(join(base,"elsewhere"),{mode:0o700});
  symlinkSync(join(base,"elsewhere"),link);
  writeFileSync(join(store,hash),image);
  const root=defaultEvidenceRoot({AUTARCH_HOME_EVIDENCE:link+"/../store/"});
  expect(readEvidence(hash,root).ok).toBe(true);
  symlinkSync(store,join(base,"store-link"));
  for (const suffix of ["", "/"]) {
   expect(readEvidence(hash,defaultEvidenceRoot({AUTARCH_HOME_EVIDENCE:join(base,"store-link")+suffix}))).toEqual({ok:false});
  }
  chmodSync(store,0o755);
  expect(readEvidence(hash,root)).toEqual({ok:false});
 } finally { rmSync(base,{recursive:true,force:true}); }
});

it.each([
 ["linux", false],
 ["darwin", false],
 ["win32", true],
] as const)("applies root permissions for %s", (platform, accepted) => {
 const root=mkdtempSync(join(tmpdir(),"evidence-mode-"));
 const image=Buffer.from("iVBORw0KGgo=","base64");
 const hash=createHash("sha256").update(image).digest("hex");
 try {
  writeFileSync(join(root,hash),image);
  chmodSync(root,0o755);
  expect(readEvidence(hash,root,platform).ok).toBe(accepted);
  const link=root+"-link";
  try {
   symlinkSync(root,link);
   expect(readEvidence(hash,link,platform)).toEqual({ok:false});
  } finally { rmSync(link,{force:true}); }
 } finally { rmSync(root,{recursive:true,force:true}); }
});
