import { afterEach, expect, it } from "vitest";
import { rmSync } from "node:fs";
import { renderToStaticMarkup } from "react-dom/server";
import { recoveryRig } from "./pickhandler-stage4-rig.js";
import { envs, rig } from "./card-rig.js";
import { FakeSdk } from "./wakes-helpers.js";
import { WakeLoop } from "../wakes.js";
import { AsksPanel } from "../ui/asks.js";
import { vi } from "vitest";
import { readPickHandlerStatus } from "../pickhandler.js";
import { reconcilePickReceipts } from "../pickcapture.js";
import type { AsksData } from "../ui/asks.js";
const temps: {cleanup():void}[]=[];
afterEach(()=>{while(envs.length) rmSync(envs.pop()!.dir,{recursive:true,force:true}); while(temps.length)temps.pop()!.cleanup();});
it.each(["deadline", "epoch", "note"])("recorded %s fallback releases manual ownership without recycling initial checks", async cause => {
 const {r,g,temp}=await recoveryRig(); temps.push(temp);
 await r.validatedMkPick(g.id,"manual-fallback"); await r.svc.pickWorkIdle();
 const initial=r.db.prepare("SELECT evaluation_json FROM pick_actions").get() as {evaluation_json:string};
 if(cause==="deadline") r.advance(120000);
 if(cause==="epoch") r.db.prepare("UPDATE pick_handler_control SET epoch=epoch+1").run();
 if(cause==="note") expect(r.svc.note({decision_id:g.id},"Wait for review","manual-note",{deferWake:true}).ok).toBe(true);
 r.svc.expirePickEvaluations(); r.restart();
 const view=r.svc.readPickAction({pick_id:"manual-fallback"});
 expect(view).toMatchObject({handler_status:"manual handling required",ordinary_continuation:true});
 if(view.kind!=="action")throw Error("missing");
 expect(view.facts.checks).toBeNull(); expect(view.snapshot.evaluation_json).toBe(initial.evaluation_json);
 expect(view.facts.initial_evaluation).toEqual(JSON.parse(initial.evaluation_json));
 const relay=r.svc.store.obligationsFor(g.id).find(o=>o.kind==="pick-relay")!;
 expect(r.svc.store.attempts(relay.id)).toEqual([]);
 expect(relay.voided_at).not.toBeNull();
 const wake=r.svc.store.obligationsFor(g.id).find(o=>o.kind==="wake")!;
 expect(wake.payload).toContain("Evaluation: unavailable");
 expect(wake.payload).toContain("Initial evaluation (history only)");
 if(cause==="deadline")expect(wake.payload).toContain("Fallback reason: evaluation-timeout");
 expect(Buffer.byteLength(wake.payload!.split("\nOwner words,")[0])).toBeLessThanOrEqual(4096);
 const event=r.db.prepare("SELECT seq,detail_json FROM events WHERE type='pick-fallback'").get() as {seq:number;detail_json:string};
 const original=JSON.parse(event.detail_json);
 for(const detail of [{...original,pick_id:"wrong"},{...original,generation:99},{...original,policy_version:99},{...original,reason:"unknown-reason"},{...original,reason:"before-send-evaluation"},{...original,evaluation:{}},{...original,at:"invalid"}]) {
   r.db.prepare("UPDATE events SET detail_json=? WHERE seq=?").run(JSON.stringify(detail),event.seq);
   expect(r.svc.readPickAction({pick_id:"manual-fallback"}).ordinary_continuation).toBe(false);
 }
 r.db.prepare("DELETE FROM events WHERE seq=?").run(event.seq);
 expect(r.svc.readPickAction({pick_id:"manual-fallback"}).ordinary_continuation).toBe(false);
});
it("review232: before-send owner evidence survives original-action readback", async()=>{
 const {r,g,task,temp}=await recoveryRig(); temps.push(temp);
 expect((await r.validatedMkPick(g.id,"review-pick")).ok).toBe(true); await r.svc.pickWorkIdle();
 const text="Wait for the independent review before continuing.";
 r.fake.addComment(task.id,{kind:"user",threadId:null,body:text,createdAt:r.env.now()});
 const sdk=new FakeSdk(); await new WakeLoop(r.svc,sdk).drain();
 expect(sdk.sent.filter(s=>s.threadId==="thr_target")).toHaveLength(0);
 const fallback=r.svc.store.obligationsFor(g.id).find(o=>o.kind==="wake")!;
 expect(fallback.payload).toContain(text);
 const view=r.svc.readPickAction({pick_id:"review-pick"});
 console.log("READBACK",JSON.stringify({kind:view.kind,status:view.kind==="action"?view.handler_status:null,words:view.kind==="action"?view.facts.owner_words:null,check:view.kind==="action"?view.facts.checks?.["no-extra-words"]:null}));
 expect(view.kind).toBe("action"); if(view.kind!=="action")throw Error("unavailable");
 expect(view.facts.owner_words.some(w=>w.text===text)).toBe(true);
});
it("fresh fallback survives reopen and corrupt or missing authority fails closed", async()=>{
 const {r,g,task,temp}=await recoveryRig(); temps.push(temp);
 await r.validatedMkPick(g.id,"fresh-reopen"); await r.svc.pickWorkIdle();
 const initial=r.db.prepare("SELECT evaluation_json FROM pick_actions").get() as {evaluation_json:string};
 r.fake.addComment(task.id,{kind:"user",threadId:null,body:"Wait.\u0000 完整条件",createdAt:r.env.now()});
 await new WakeLoop(r.svc,new FakeSdk()).drain(); r.restart();
 const view=r.svc.readPickAction({pick_id:"fresh-reopen"});
 expect(view.kind).toBe("action"); if(view.kind!=="action")throw Error("missing");
 expect(view.facts.checks?.["no-extra-words"].result).toBe("fail");
 expect(view.facts.owner_words.some(w=>w.text==="Wait.\u0000 完整条件")).toBe(true);
 expect(view.snapshot.evaluation_json).toBe(initial.evaluation_json);
 expect(view.facts.initial_evaluation).toEqual(JSON.parse(initial.evaluation_json));
 expect(view.ordinary_continuation).toBe(true);
 const event=r.db.prepare("SELECT seq, detail_json FROM events WHERE type='pick-fallback'").get() as {seq:number;detail_json:string};
 const original=JSON.parse(event.detail_json);
 for(const detail of [{...original,evaluation:JSON.parse(initial.evaluation_json)},{...original,evaluation:null},{...original,evaluation:{}},{...original,pick_id:"wrong"},{...original,evaluation:{...original.evaluation,pick_id:"wrong"}},{...original,evaluation:{...original.evaluation,generation:99}},{...original,evaluation:{...original.evaluation,policy_version:99}},{...original,evaluation:{...original.evaluation,evidence_times:{...original.evaluation.evidence_times,evaluated_at:"2000-01-01T00:00:00Z"}}}]) {
   r.db.prepare("UPDATE events SET detail_json=? WHERE seq=?").run(JSON.stringify(detail),event.seq);
   r.restart(); expect(r.svc.readPickAction({pick_id:"fresh-reopen"}).ordinary_continuation).toBe(false);
 }
 r.db.prepare("DELETE FROM events WHERE seq=?").run(event.seq);
 r.restart(); expect(r.svc.readPickAction({pick_id:"fresh-reopen"}).ordinary_continuation).toBe(false);
});

it.each([null,""," ","sent","queue-handle"])("persisted invalid delivered evidence %s produces one recovery wake without a second relay",async(evidence)=>{
 const {r,g,temp}=await recoveryRig(); temps.push(temp);
 await r.validatedMkPick(g.id,"persisted-invalid"); await r.svc.pickWorkIdle();
 const sdk=new FakeSdk(); sdk.send=async input=>{sdk.sent.push(input);return {delivery:"sent"};};
 await new WakeLoop(r.svc,sdk).drain();
 const relay=r.svc.store.obligationsFor(g.id).find(o=>o.kind==="pick-relay")!;
 r.db.prepare("UPDATE attempts SET state='delivered', evidence=?, handle=? WHERE obligation_id=?").run(evidence,evidence==="queue-handle"?evidence:null,relay.id);
 r.db.prepare("UPDATE obligations SET state='done' WHERE id=?").run(relay.id);
 r.restart(); r.advance(120000); expect(r.svc.expirePickEvaluations()).toBe(1); expect(r.svc.expirePickEvaluations()).toBe(0);
 const loop=new WakeLoop(r.svc,sdk); await loop.drain(); await loop.drain();
 expect(sdk.sent.filter(s=>s.threadId==="thr_target")).toHaveLength(1);
 expect(readPickHandlerStatus(r.svc.store,g.id)).toBe("delivery uncertain");
 expect(r.svc.readPickAction({pick_id:"persisted-invalid"})).toMatchObject({handler_status:"delivery uncertain",ordinary_continuation:false});
 expect(r.svc.store.obligationsFor(g.id).filter(o=>o.kind==="wake")).toHaveLength(1);
 expect(r.svc.store.obligationsFor(g.id).filter(o=>o.kind==="pick-receipt")).toHaveLength(0);
});

it.each([false,true])("durable confirmation (event equals queue handle: %s) earns one receipt and no fallback",async(queued)=>{
 const {r,g,temp}=await recoveryRig(); temps.push(temp);
 await r.validatedMkPick(g.id,"confirmed"); await r.svc.pickWorkIdle();
 const sdk=new FakeSdk(); if(queued)sdk.script.push({kind:"queue"});
 const loop=new WakeLoop(r.svc,sdk); await loop.drain();
 if(queued){ const relay=r.svc.store.obligationsFor(g.id).find(o=>o.kind==="pick-relay")!; const handle=r.svc.store.attempts(relay.id)[0].handle!; sdk.dispatch(handle); sdk.accepted[0].id=handle; await loop.reconcile(); }
 r.restart(); r.advance(120000); expect(r.svc.expirePickEvaluations()).toBe(0);
 reconcilePickReceipts(r.svc.store,r.env.now());
 await new WakeLoop(r.svc,sdk).drain();
 expect(readPickHandlerStatus(r.svc.store,g.id)).toBe("already relayed");
 expect(r.svc.readPickAction({pick_id:"confirmed"})).toMatchObject({handler_status:"already relayed",ordinary_continuation:false});
 expect(sdk.sent.filter(s=>s.threadId==="thr_target")).toHaveLength(1);
 expect(r.svc.store.obligationsFor(g.id).filter(o=>o.kind==="wake")).toHaveLength(0);
 expect(r.svc.store.obligationsFor(g.id).filter(o=>o.kind==="pick-receipt")).toHaveLength(1);
});

it("fresh head and edited owner comment outrank initial observations after reopening",async()=>{
 const filed="a".repeat(40), changed="b".repeat(40);
 const {r,g,task,temp}=await recoveryRig({subject:{kind:"github-pr",repo:"example/repo",number:7,head:filed}}); temps.push(temp);
 const evaluate=r.svc.pickEvaluation.bind(r.svc); let head=filed;
 vi.spyOn(r.svc,"pickEvaluation").mockImplementation((id,tasks,opts)=>evaluate(id,tasks,{...opts,readHead:async()=>({state:"OPEN",headRefOid:head})}));
 await r.validatedMkPick(g.id,"fresh-head"); await r.svc.pickWorkIdle();
 head=changed;
 r.fake.addComment(task.id,{kind:"user",threadId:null,body:"New condition",createdAt:r.env.now()});
 const comment=r.fake.comments.at(-1)!;
 r.svc.store.recordEvent("pick-evidence-observed",g.id,{pick_id:"fresh-head",evidence:{comments:{result:"fail",reason:"owner-comment",observed_at:r.env.now(),cutoff:null,comments:[],owner_words:[{id:comment.id,text:"Old condition"}],limitations:[]}}});
 await new WakeLoop(r.svc,new FakeSdk()).drain(); r.restart();
 const view=r.svc.readPickAction({pick_id:"fresh-head"}); expect(view.kind).toBe("action"); if(view.kind!=="action")throw Error("missing");
 expect(view.facts.head).toMatchObject({current:changed});
 expect(view.facts.owner_words.filter(w=>w.id===comment.id)).toEqual([{kind:"comment",id:comment.id,text:"New condition"}]);
 expect(view.facts.checks?.["subject-head"].result).toBe("fail");
});

it("decision queue precedes bounded history, with truthful earlier count and empty controls",()=>{
 const history=Array.from({length:30},(_,i)=>({decision_id:`past-${i}`,generation:1,status:"already relayed",observation_only:false,ordinary_continuation:false,target:"Target / Project",deadline:null,deadline_outcome:"met",first_failure:false,readback:"read original pick"}));
 const data:AsksData={owed:[{id:"active",project:"Project",thread:"asker",subject:"Active decision",asker:"agent",filed_at:"2026-01-01T00:00:00Z",revision:"one",ask:{question:"Active decision",options:[{id:"a",label:"Choose",kind:"ruling-only"},{id:"b",label:"Wait",kind:"ruling-only"}]}}],runbook:[],lane:[],asks:[],undeliverable:[],failures:[],uncertain:[],machineOwners:{},delegation:{settings:{},suspended:false},pick_actions:history};
 const render=(part?:"later"|"decide",emptyText?:string)=>renderToStaticMarkup(<AsksPanel data={data} onPick={()=>{}} onOpen={()=>{}} part={part} emptyText={emptyText}/>);
 const html=render(); expect(html.indexOf('data-section="decide"')).toBeLessThan(html.indexOf('data-pick-action=')); expect(html).toContain("10 earlier picks.");
 expect(html).not.toContain("Nothing needs you.");
 expect(render("later")).toContain("Nothing is set aside."); expect(render("later")).not.toContain("data-pick-action");
 data.owed=[]; expect(render("decide","No decisions in this project.")).toContain("No decisions in this project.");
});

it.each([undefined,""," ","sent"])("invalid response %s remains uncertain through restart and one deadline wake",async(messageId)=>{
 const {r,g,temp}=await recoveryRig(); temps.push(temp);
 await r.validatedMkPick(g.id,"invalid-response"); await r.svc.pickWorkIdle();
 const sdk=new FakeSdk(); sdk.send=async input=>{sdk.sent.push(input);return {delivery:"sent",messageId};};
 await new WakeLoop(r.svc,sdk).drain();
 const relay=r.svc.store.obligationsFor(g.id).find(o=>o.kind==="pick-relay")!;
 expect(r.svc.store.attempts(relay.id)[0].state).toBe("uncertain");
 r.restart(); r.advance(120000); r.svc.expirePickEvaluations(); r.svc.expirePickEvaluations();
 await new WakeLoop(r.svc,sdk).drain();
 expect(sdk.sent.filter(s=>s.threadId==="thr_target")).toHaveLength(1);
 expect(r.svc.store.obligationsFor(g.id).filter(o=>o.kind==="wake")).toHaveLength(1);
 expect(r.svc.store.obligationsFor(g.id).filter(o=>o.kind==="pick-receipt")).toHaveLength(0);
 expect(r.svc.readPickAction({pick_id:"invalid-response"}).ordinary_continuation).toBe(false);
});

it("multibyte fallback budgets metadata separately from full owner words",async()=>{
 const {r,g,temp}=await recoveryRig(); temps.push(temp);
 const words="条件\u0000\n".repeat(300);
 await r.validatedMkPick(g.id,"界".repeat(5000),"a",words); await r.svc.pickWorkIdle();
 const payload=r.svc.store.obligationsFor(g.id).find(o=>o.kind==="wake")!.payload!;
 expect(Buffer.byteLength(payload.split("\nOwner words,")[0])).toBeLessThanOrEqual(4096);
 expect(payload).toContain(words);
});

it("service selects at most twenty histories before expensive readback and counts earlier picks",async()=>{
 const {r,g,temp}=await recoveryRig(); temps.push(temp);
 await r.validatedMkPick(g.id,"history-first","a","Wait"); await r.svc.pickWorkIdle();
 for(let i=0;i<24;i++) {
   const task=r.card({key:`history-${i}`,ask:{recommendation:"a",options:[{id:"a",label:"Blue",kind:"ruling-only",on_pick:{schema:1,target_thread:"thr_target",reversible:true,subject:{kind:"none"},ruling:{bead:null,under:[]},notification:"Blue selected."}},{id:"b",label:"Wait",kind:"ruling-only"}]},blocks:"thread:thr_target"});
   await r.poll(); const d=r.gens(task.id)[0]; await r.validatedMkPick(d.id,`history-${i}`,"a","Wait"); await r.svc.pickWorkIdle(); r.advance(1);
 }
 const legacy=r.card({key:"legacy-history"}); await r.poll();
 const legacyDecision=r.gens(legacy.id)[0]; await r.validatedMkPick(legacyDecision.id,"legacy-history");
 const spy=vi.spyOn(r.svc,"readPickAction"); const summaries=r.svc.pickActionSummaries();
 expect(summaries).toHaveLength(20); expect(spy).toHaveBeenCalledTimes(20);
 expect(summaries[0]).toMatchObject({earlier_count:5});
 expect(r.svc.readPickAction({decision_id:g.id,generation:g.generation!}).kind).toBe("action");
});
it('one malformed historical card cannot blank pick summaries for the inbox',async()=>{
 const r=rig(); const task=r.card(); await r.poll(); const g=r.gens(task.id)[0];
 expect((await r.validatedMkPick(g.id,'old-ordinary')).ok).toBe(true);
 expect(r.svc.pickActionSummaries()).toEqual([]);
 // Match the existing unreadable-card fixtures: emulate a historical unreadable stored row.
 r.db.exec('DROP TRIGGER decisions_ask_immutable');
 r.db.prepare('UPDATE decisions SET body_json=? WHERE id=?').run(JSON.stringify({question:'Old decision',options:['unreadable option']}),g.id);
 expect(()=>r.svc.pickActionSummaries()).not.toThrow();
 expect(r.svc.pickActionSummaries()).toEqual([]);
});

it.each([
 ["string option", {options:["unreadable option"]}],
 ["JSON-looking string option", {options:['{"on_pick":{}}']}],
 ["other nonobject options", {options:[null, true, false, 42, 1.5, [{on_pick:{}}]]}],
 ["object options", {options:{a:{on_pick:{}}}}],
 ["string options", {options:'{"on_pick":{}}'}],
 ["scalar options", {options:42}],
 ["null options", {options:null}],
 ["missing options", {}],
 ["array body", [{options:[{on_pick:{}}]}]],
 ["null body", null],
 ["string body", "unreadable body"],
 ["malformed body", undefined],
])("%s cannot hide a healthy historical summary",async(_name,body)=>{
 const {r,g,temp}=await recoveryRig(); temps.push(temp);
 await r.validatedMkPick(g.id,"healthy-history","a","Wait"); await r.svc.pickWorkIdle();
 const healthy=r.svc.pickActionSummaries(); expect(healthy).toHaveLength(1);
 const task=r.card({key:"damaged-history"}); await r.poll(); const damaged=r.gens(task.id)[0];
 expect((await r.validatedMkPick(damaged.id,"damaged-history")).ok).toBe(true);
 r.db.exec("DROP TRIGGER decisions_ask_immutable");
 r.db.prepare("UPDATE decisions SET body_json=? WHERE id=?").run(body===undefined ? "{broken" : JSON.stringify(body),damaged.id);
 expect(r.svc.pickActionSummaries()).toEqual(healthy);
});

it("unreadable participating history stays unavailable without authorizing continuation or hiding healthy history",async()=>{
 const {r,g,temp}=await recoveryRig(); temps.push(temp);
 await r.validatedMkPick(g.id,"healthy-history","a","Wait"); await r.svc.pickWorkIdle();
 const healthy=r.svc.pickActionSummaries()[0];
 const task=r.card({key:"damaged-history"}); await r.poll(); const damaged=r.gens(task.id)[0];
 await r.validatedMkPick(damaged.id,"damaged-history");
 r.db.exec("DROP TRIGGER decisions_ask_immutable");
 r.db.prepare("UPDATE decisions SET body_json=? WHERE id=?").run(JSON.stringify({options:[{on_pick:{}}]}),damaged.id);
 const summaries=r.svc.pickActionSummaries();
 expect(summaries).toHaveLength(2);
 expect(summaries.find(s=>s.decision_id===g.id)).toEqual(healthy);
 expect(summaries.find(s=>s.decision_id===damaged.id)).toMatchObject({generation:damaged.generation,status:"data unavailable",ordinary_continuation:false,observation_only:false,target:null,deadline:null,deadline_outcome:"unknown",first_failure:false});
 expect(summaries.find(s=>s.decision_id===damaged.id)!.readback).toContain("damaged-history");
});

it("review232: historical actions render without a bound ahead of the empty queue",()=>{
 const actions=Array.from({length:1000},(_,i)=>({decision_id:`historical-${i}`,generation:1,status:"already relayed",observation_only:false,ordinary_continuation:false,target:"Frozen target / Project",deadline:"2026-01-01T00:02:00Z",deadline_outcome:"met",first_failure:false,readback:`bb home pick-action --pick historical-${i} --json`}));
 const t=performance.now();
 const html=renderToStaticMarkup(<AsksPanel nowMs={0} onPick={()=>{}} onOpen={()=>{}} data={{owed:[],runbook:[],lane:[],asks:[],undeliverable:[],failures:[],uncertain:[],machineOwners:{},delegation:{settings:{},suspended:false},pick_actions:actions}}/>);
 console.log("HISTORY",JSON.stringify({rows:(html.match(/data-pick-action=/g)||[]).length,bytes:Buffer.byteLength(html),ms:performance.now()-t,empty:html.includes("Nothing needs you.")}));
 expect((html.match(/data-pick-action=/g)||[]).length).toBeLessThanOrEqual(20);
 expect(html).toContain("Nothing needs you.");
});
it("review232: final fallback metadata respects the 4096-byte bound",async()=>{
 const {r,g,temp}=await recoveryRig(); temps.push(temp);
 const id="p".repeat(5000);
 expect((await r.validatedMkPick(g.id,id,"a","Wait.")).ok).toBe(true); await r.svc.pickWorkIdle();
 const wake=r.svc.store.obligationsFor(g.id).find(o=>o.kind==="wake")!;
 const metadata=wake.payload!.split("\nOwner words,")[0];
 console.log("FINAL_METADATA",JSON.stringify({bytes:Buffer.byteLength(metadata),headBytes:Buffer.byteLength(metadata.split("\n")[0])}));
 expect(Buffer.byteLength(metadata)).toBeLessThanOrEqual(4096);
});
it("review232: rejected durable evidence releases recovery ownership at deadline",async()=>{
 const {r,g,temp}=await recoveryRig(); temps.push(temp);
 expect((await r.validatedMkPick(g.id,"bad-evidence")).ok).toBe(true); await r.svc.pickWorkIdle();
 const sdk=new FakeSdk(); sdk.send=async input=>{sdk.sent.push(input); return {delivery:"sent",messageId:"sent"};};
 await new WakeLoop(r.svc,sdk).drain();
 r.advance(120000); const settled=r.svc.expirePickEvaluations();
 const view=r.svc.readPickAction({pick_id:"bad-evidence"});
 const wakes=r.svc.store.obligationsFor(g.id).filter(o=>o.kind==="wake");
 console.log("BAD_EVIDENCE",JSON.stringify({settled,status:view.kind==="action"?view.handler_status:null,ordinary:view.ordinary_continuation,wakes:wakes.length}));
 expect(wakes).toHaveLength(1);
 expect(wakes[0].payload).toContain("delivery may already have occurred; do not relay blindly");
});
