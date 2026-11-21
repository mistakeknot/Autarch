import { readFileSync } from "node:fs";
import { expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));
import { parseAsk, identity, revision, semanticKey, normalizedJson } from "../model.js";
import { cardFingerprint, parseCard } from "../cards.js";
import { DecideQueue, type OwedAsk } from "../ui/asks.js";
import { inboxRows, MobileInbox } from "../ui/inbox.js";
import { cleanupEnvs, opened, rig } from "./card-rig.js";
import { owedAsk } from "../askbody.js";

const input = {v:1,kind:"decide",project:"Example",project_root:"/example",asker:"thread",thread:"thr_test",question:"Which?",options:[{id:"a",label:"A",kind:"ruling-only"},{id:"b",label:"B",kind:"ruling-only"}]};
const group = {key:"merge-pair",order:2};
const wire = (g?: unknown) => ({schema:"home-ask/v2",project:input.project,project_root:input.project_root,question:input.question,options:input.options,...(g===undefined?{}:{group:g})});
const card = (g?:unknown) => parseCard(`Choice\nRequest: key sha256:${["01234567","89abcdef"].join("")}\n\n\`\`\`home-ask\n${JSON.stringify(wire(g))}\n\`\`\``);
const vectors=JSON.parse(readFileSync(new URL("../../../testdata/group-vectors.json",import.meta.url),"utf8")) as {name:string;raw:string;valid:boolean}[];
it.each(vectors)("group grammar: $name", v => {
 for (const f of [()=>parseAsk({...input,group:JSON.parse(v.raw)}),()=>card(JSON.parse(v.raw))]) {
  if(v.valid) { expect(f).not.toThrow(); const value=f(); expect(value).toMatchObject("ask" in value ? {ask:{group:JSON.parse(v.raw)}} : {group:JSON.parse(v.raw)}); } else expect(f).toThrow();
 }
});
it("preserves all pinned hashes and fingerprint with and without group",()=>{
 const fixtures=JSON.parse(readFileSync(new URL("../../../schema/home-ask/v1/vectors.json",import.meta.url),"utf8"));
 for(const c of fixtures.asks) { const a=parseAsk({...c.input,group}); expect(identity(a)).toBe(c.identity); expect(revision(a)).toBe(c.revision); expect(semanticKey(a)).toBe(c.semantic_key); expect(normalizedJson(a)).toBe(c.normalized); }
 const a=parseAsk(input),b=parseAsk({...input,group}); for(const f of [identity,revision,semanticKey,normalizedJson]) expect(f(a)).toBe(f(b));
 const fp=(g?:unknown)=>cardFingerprint({title:"Choice",card:card(g),thread:"thr_test",tasksProject:"p"}); expect(fp(group)).toBe(fp());
});
it("delivers a lone card immediately through filing and stored body parsing",async()=>{
 try {const r=rig();const {t,g1}=await opened(r,{ask:{group}});expect(owedAsk(g1.body_json).ask).toMatchObject({group});
 r.edit(t,{description:t.description!.replace('"order":2','"order":3')});await r.poll();expect(r.gens(t.id)).toHaveLength(1);expect(owedAsk(r.gens(t.id)[0].body_json).ask).toMatchObject({group});
 }finally{cleanupEnvs();}
});
const row=(id:string,g?:typeof group):OwedAsk=>({id,subject:id,project:"Example",thread:"thr_test",asker:"thread",filed_at:"2026-01-01",revision:"r",ask:{question:"Which?",options:input.options,...(g?{group:g}:{})}});
it("shows groups together in order, ties in incoming order, missing members and singletons",()=>{
 const owed=[row("second",group),row("unlinked"),row("first",{...group,order:1}),row("tied",group),row("singleton",{key:"merge-pair.extra",order:7})];
 const html=renderToStaticMarkup(<DecideQueue owed={owed} onPick={()=>{}} onOpen={()=>{}} nowMs={0}/>);
 expect(html).toContain('data-merge-group="merge-pair"');expect(html).toContain('data-group-order="7"');
 const order=[...html.matchAll(/data-queue-id="([^"]+)"/g)].map(m=>m[1]);expect(order).toEqual(["first","second","tied","unlinked","singleton"]);
 const data={owed,runbook:[],lane:[],asks:[],undeliverable:[],failures:[],uncertain:[],delegation:{settings:{},suspended:false},machineOwners:{}};
 expect(inboxRows(data,0).waiting.map(r=>r.ask?.id)).toEqual(order);
 const mobile=renderToStaticMarkup(<MobileInbox asks={data} waiting={owed.length} onPick={()=>{}} onOpen={()=>{}} onNote={async()=>({ok:true})} onLater={()=>{}} sinceNode={null} sinceCount={0} onDesktop={()=>{}} now={0}/>);
 expect([...mobile.matchAll(/data-inbox-row="ask:([^"]+)"/g)].map(m=>m[1])).toEqual(order);
 expect(mobile).toContain('data-merge-group="merge-pair"');expect(mobile).toContain("step 7");
 const one=renderToStaticMarkup(<DecideQueue owed={[owed[0]]} onPick={()=>{}} onOpen={()=>{}} nowMs={0}/>);expect(one).toContain('data-merge-group="merge-pair"');expect(one).toContain("second");
});
