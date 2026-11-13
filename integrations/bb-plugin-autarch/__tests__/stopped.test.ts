import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import { parseCard, toV1, cardFingerprint } from "../cards.js";
import { parseAsk, normalizedJson, revision, semanticKey } from "../model.js";
const vectors = JSON.parse(readFileSync(new URL("../../../testdata/blocker-stopped.json", import.meta.url), "utf8")) as {name:string;fields:Record<string,unknown>;valid:boolean}[];
const base = {schema:"home-ask/v2",project:"p",project_root:"/r",question:"Q?", options:[{id:"a",label:"A",kind:"ruling-only"},{id:"b",label:"B",kind:"ruling-only"}]};
const desc = (fields:Record<string,unknown>) => 'Blocks: bead:example\nRequest: example sha256:0123456789abcdef\n\n```home-ask\n'+JSON.stringify({...base,...fields})+'\n```\n';
for (const v of vectors) it(v.name, () => {
 const parse = () => parseCard(desc(v.fields));
 const {schema:_,...body} = base;
 const legacy = () => parseAsk({...body,v:1,kind:"decide",asker:"thread",thread:"t",...v.fields});
 if(v.valid) { expect(parse()).toBeTruthy(); expect(legacy()).toBeTruthy(); }
 else { expect(parse).toThrow(); expect(legacy).toThrow(); }
});
it("stopped changes no identity or fingerprint", () => {
 const a=parseCard(desc({urgency:"blocker",stopped:"Release waits"}));
 const b=parseCard(desc({urgency:"blocker",stopped:"Tests wait"}));
 const x=toV1(a,"t"), y=toV1(b,"t");
 expect(normalizedJson(x)).toBe(normalizedJson(y)); expect(revision(x)).toBe(revision(y)); expect(semanticKey(x)).toBe(semanticKey(y));
 const fp=(card:ReturnType<typeof parseCard>)=>cardFingerprint({title:"Q",card,thread:"t",tasksProject:"p"}); expect(fp(a)).toBe(fp(b));
});
