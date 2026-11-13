import { renderToStaticMarkup } from "react-dom/server";
import { expect, it, vi } from "vitest";
vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));
import { parseAsk } from "../model.js";
import { AskCard } from "../ui/asks.js";
import { identity, revision, semanticKey, normalizedJson } from "../model.js";
import { cardFingerprint, parseCard } from "../cards.js";
import { EvidenceBlock } from "../ui/evidence.js";
import { readEvidence } from "../evidencestore.js";
import { createHash } from "node:crypto";
import { mkdtempSync, writeFileSync, rmSync, symlinkSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { cleanupEnvs, opened, rig } from "./card-rig.js";
import { owedAsk } from "../askbody.js";
import { homeMethods } from "../contract.js";
it("accepts only a hash as image RPC input", () => {
 expect(homeMethods.evidenceImage.input.safeParse({sha256:"a".repeat(64)}).success).toBe(true);
 for (const input of [{sha256:"../example"},{sha256:"a".repeat(64),path:"/example"},{}]) expect(homeMethods.evidenceImage.input.safeParse(input).success).toBe(false);
});
it("preserves fixture hashes when evidence is added", () => {
 const vectors=JSON.parse(readFileSync(fileURLToPath(new URL("../../../schema/home-ask/v1/vectors.json",import.meta.url)),"utf8"));
 for (const c of vectors.asks) { const a=parseAsk({...c.input,evidence}); expect(identity(a)).toBe(c.identity); expect(revision(a)).toBe(c.revision); expect(semanticKey(a)).toBe(c.semantic_key); expect(normalizedJson(a)).toBe(c.normalized); }
});
it("delivers initial evidence from the stored card body and ignores evidence-only edits", async () => {
 try {
  const r=rig(); const {t,g1}=await opened(r,{ask:{evidence}});
  expect(owedAsk(g1.body_json).ask).toMatchObject({evidence});
  const desc=t.description!.replace("https://example.test/run","https://example.test/new");
  r.edit(t,{description:desc}); await r.poll();
  expect(r.gens(t.id)).toHaveLength(1);
  expect(owedAsk(r.gens(t.id)[0].body_json).ask).toMatchObject({evidence});
 } finally { cleanupEnvs(); }
});

const evidence = [{ caption: "Run", url: "https://example.test/run" }];
const input = { v: 1, kind: "decide", project: "Example", project_root: "/example", asker: "thread", thread: "thr_test", question: "Which?", options: [{id:"a",label:"A",kind:"ruling-only"},{id:"b",label:"B",kind:"ruling-only"}] };
it("accepts evidence in the strict parser", () => {
 expect(() => parseAsk({ ...input, evidence })).not.toThrow();
});
it("keeps every hash, normalized JSON and card fingerprint unchanged", () => {
 const a=parseAsk(input), b=parseAsk({...input,evidence});
 for (const fn of [identity,revision,semanticKey,normalizedJson]) expect(fn(b)).toBe(fn(a));
 const request = `Request: key sha256:${["01234567", "89abcdef"].join("")}`;
 const card = (ev: boolean) => parseCard(`Choice\n${request}\n\n\`\`\`home-ask\n${JSON.stringify({schema:"home-ask/v2",project:"Example",project_root:"/example",question:input.question,options:input.options,...(ev?{evidence}:{})})}\n\`\`\``);
 expect(card(true).ask.evidence).toEqual(evidence);
 expect(cardFingerprint({title:"Choice",card:card(true),thread:"thr_test",tasksProject:"p"})).toBe(cardFingerprint({title:"Choice",card:card(false),thread:"thr_test",tasksProject:"p"}));
});
it("refuses unsafe URLs and malformed evidence", () => {
 const im={sha256:"a".repeat(64),mime:"image/png"};
 for (const url of ["http://example.test","javascript:alert(1)","data:image/png,hi","file:///test","https://user:pass@example.test","https://@example.test","https://example.test/\n","https://example.test/"+"a".repeat(2048),"https:///missing","https://example.test/\\bad"]) {
  expect(()=>parseAsk({...input,evidence:[{caption:"Run",url}]})).toThrow();
 }
 for (const evidence of [null,{},Array(7).fill({caption:"x",url:"https://example.test"}),[{caption:"",url:"https://example.test"}],[{caption:"a".repeat(201),image:im}],[{caption:"x\nx",image:im}],[{caption:"x",image_path:"/example"}],[{caption:"x",url:"",image:im}],[{caption:"x",before:null,after:{image:im}}],[{caption:"x",image:{...im,path:"/example"}}],[{caption:"x",image:{...im,mime:"image/svg+xml"}}]]) expect(()=>parseAsk({...input,evidence})).toThrow();
});
it("renders nothing for old cards and responsive before/after sources as links", () => {
 expect(renderToStaticMarkup(<EvidenceBlock />)).toBe("");
 const html=renderToStaticMarkup(<EvidenceBlock items={[{caption:"Comparison",before:{url:"https://example.test/before"},after:{url:"https://example.test/after"}}]} />);
 expect(html).toContain("Before"); expect(html).toContain("After"); expect(html).toContain("auto-fit"); expect(html).not.toContain("<img");
});
it("reads only valid, bounded hash-addressed images and hides missing paths", () => {
 const root=mkdtempSync(join(tmpdir(),"evidence-"));
 try {
  const b=Buffer.from([137,80,78,71,13,10,26,10]); const hash=createHash("sha256").update(b).digest("hex");
  writeFileSync(join(root,hash),b,{mode:0o600});
  expect(readEvidence(hash,root)).toEqual({ok:true,mime:"image/png",data_base64:b.toString("base64")});
  for (const h of ["../file","a".repeat(64),hash.toUpperCase()]) expect(readEvidence(h,root)).toEqual({ok:false});
  writeFileSync(join(root,hash),"not an image"); expect(readEvidence(hash,root)).toEqual({ok:false});
  const invalid=Buffer.from("not an image"), invalidHash=createHash("sha256").update(invalid).digest("hex");
  writeFileSync(join(root,invalidHash),invalid); expect(readEvidence(invalidHash,root)).toEqual({ok:false});
  const outside=join(root,"outside.png"); writeFileSync(outside,b);
  rmSync(join(root,hash)); symlinkSync(outside,join(root,hash)); expect(readEvidence(hash,root)).toEqual({ok:false});
  const large=Buffer.alloc(2*1024*1024+1); b.copy(large); const lh=createHash("sha256").update(large).digest("hex"); writeFileSync(join(root,lh),large); expect(readEvidence(lh,root)).toEqual({ok:false});
  const max=large.subarray(0,2*1024*1024); const mh=createHash("sha256").update(max).digest("hex"); writeFileSync(join(root,mh),max); expect(readEvidence(mh,root).ok).toBe(true);
 } finally { rmSync(root,{recursive:true,force:true}); }
});
it("renders evidence on the decision card", () => {
 const ask = { id:"d", project:"Example",thread:"thr_test",subject:"Choice",asker:"thread",filed_at:"2026-01-01",revision:"r",ask:{...input,evidence} };
 const html = renderToStaticMarkup(<AskCard ask={ask as any} onPick={() => {}} onOpen={() => {}} />);
 expect(html).toContain("Evidence");
 expect(html).toContain('rel="noopener noreferrer"');
});
