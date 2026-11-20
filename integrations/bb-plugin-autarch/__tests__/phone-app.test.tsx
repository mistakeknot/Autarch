// @vitest-environment jsdom
import { act, createElement, type ComponentType } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { AsksData, OwedAsk } from "../ui/asks.js";
const sdk = vi.hoisted(()=>({page:null as ComponentType | null, rpc:{call:vi.fn()},nav:{toThread:vi.fn(),openThreadPanel:vi.fn(),toPluginPanel:vi.fn()}}));
vi.mock("@get-bb/plugin-sdk/app",()=>({
 definePluginApp:(setup:Function)=>{setup({slots:{navPanel:(p:{component:ComponentType})=>{sdk.page=p.component},threadPanelAction:()=>{}}});return {};},
 useRpc:()=>sdk.rpc,useBbNavigate:()=>sdk.nav,useRealtime:()=>{},ThreadChat:()=>null,experimental_Icon:()=>null,
}));
import "../app.js";
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT=true;
const sample: OwedAsk={id:"sample",project:"Example",thread:"sample",subject:"Sample choice",asker:"thread",filed_at:"2026-11-09T07:00:00Z",revision:"sample",task_id:"sample",ask:{question:"Choose a sample",recommendation:"a",options:[{id:"a",label:"Continue",kind:"ruling-only",reversible:true}]}};
let host:HTMLDivElement,root:Root;
let data:AsksData;
let phoneWidth:boolean;
let widthListeners:Set<() => void>;
beforeEach(()=>{
 vi.useFakeTimers();localStorage.clear();
 HTMLElement.prototype.scrollIntoView=vi.fn();
 phoneWidth=true;widthListeners=new Set();
 vi.stubGlobal("matchMedia",()=>({
  get matches(){return phoneWidth},
  addEventListener:(_event:string,listener:()=>void)=>widthListeners.add(listener),
  removeEventListener:(_event:string,listener:()=>void)=>widthListeners.delete(listener),
 }));
 data={owed:[sample],runbook:[],lane:[],asks:[],undeliverable:[],failures:[],uncertain:[],delegation:{settings:{},suspended:false},machineOwners:{}};
 sdk.rpc.call.mockReset().mockImplementation(async(method:string)=>{
  if(method==="listAsks")return data;
  if(method==="catchup")return [];
  if(method==="health")return {ready:true};
  if(method==="waiting")return {decide:1,later:0,held:0,updates:0,suspended:false};
  if(method==="queue")return null;
  if(method==="learn")return {ok:true,enabled:false,cards:[]};
  if(method==="ideas")return {ok:true,ideas:[{task_id:"idea",project:"Example",key:"EX-1",words:"Sample idea"}],show:true};
  if(method==="updateInfo")return null;
  return {ok:true};
 });
 host=document.createElement("div");document.body.append(host);root=createRoot(host);
});
afterEach(()=>{act(()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();localStorage.clear();});
const mount=()=>act(async()=>{root.render(createElement(sdk.page!));});
const click=async(selector:string)=>{const el=host.querySelector<HTMLElement>(selector);expect(el,selector).not.toBeNull();await act(async()=>el!.click());};
const sent=()=>sdk.rpc.call.mock.calls.filter(([method])=>method==="pick");
it("phone app routes Now to the inbox, other sections to panels, and More back to Queue",async()=>{
 await mount();expect(host.querySelector("[data-mobile-inbox]")).not.toBeNull();
 await click('[data-queue-tab="later"]');expect(host.querySelector("[data-mobile-inbox]")).toBeNull();
 await click('[data-queue-tab="decide"]');expect(host.querySelector("[data-mobile-inbox]")).not.toBeNull();
 await click('[data-home-overflow] button:last-of-type');expect(host.querySelector("[data-mobile-inbox]")).toBeNull();
 await click('[data-queue-tab="decide"]');expect(host.querySelector("[data-mobile-inbox]")).not.toBeNull();
});
it.each(["subtab","More","Full view"])("phone app preserves five seconds and Undo across %s navigation",async(destination)=>{
 await mount();await click("[data-inbox-row]");await click("[data-bar-main]");
 expect(sent()).toHaveLength(0);
 if(destination==="subtab")await click('[data-queue-tab="later"]');
 else if(destination==="More")await click('[data-home-overflow] button:last-of-type');
 else await click('[data-inbox-desktop]');
 expect(sent()).toHaveLength(0);expect(host.querySelector("[data-pick-undo]")).not.toBeNull();
 await act(async()=>{await vi.advanceTimersByTimeAsync(4000)});await click("[data-pick-undo]");
 await act(async()=>{await vi.advanceTimersByTimeAsync(2000)});expect(sent()).toHaveLength(0);
});
it("phone hidden count includes a held card and Ideas retains its open count",async()=>{
 data.owed.push({...sample,id:"held",task_id:"held",held:{reason:"Sample hold",by:"sample",at:"2026-11-09T07:00:00Z"}});
 localStorage.setItem("home.projectFilter",JSON.stringify({excluded:["Example"]}));await mount();
 expect(host.querySelector("[data-project-filter-trigger]")?.textContent).toContain("Projects · 2 hidden");
 expect(host.querySelector("[data-home-overflow]")?.textContent).toContain("Ideas (1)");
});
it("navigation keeps the original five-second deadline",async()=>{
 await mount();await click("[data-inbox-row]");await click("[data-bar-main]");
 await click('[data-queue-tab="later"]');
 await act(async()=>{await vi.advanceTimersByTimeAsync(4999)});expect(sent()).toHaveLength(0);
 await act(async()=>{await vi.advanceTimersByTimeAsync(1)});expect(sent()).toHaveLength(1);
});

// A small stateful RPC fake: a second accepted answer for one card gets the real conflict outcome.
function recordPicks() {
 const accepted = new Map<string,string>();
 const original = sdk.rpc.call.getMockImplementation()!;
 sdk.rpc.call.mockImplementation(async(method:string,input?:{decision_id:string;option_id:string})=>{
  if(method!=="pick")return original(method,input);
  const {decision_id,option_id}=input!;
  if(accepted.has(decision_id))return {ok:false,status:409,error:"already ruled"};
  accepted.set(decision_id,option_id);
  data={...data,owed:data.owed.filter(o=>o.id!==decision_id)};
  return {ok:true};
 });
 return accepted;
}
const withTwoOptions = (): OwedAsk => ({...sample,ask:{...sample.ask,options:[...sample.ask.options,{id:"b",label:"Choose another",kind:"ruling-only",reversible:true}]}});
const fullViewWhileHolding = async()=>{
 await mount();await click('[data-inbox-row="ask:sample"]');await click("[data-bar-main]");
 // DetailBar advances to the next card or list after accepting the held pick.
 if(host.querySelector("[data-inbox-back]"))await click("[data-inbox-back]");
 await click("[data-inbox-desktop]");
};
it("Full view's B supersedes the same card's held A without a false conflict",async()=>{
 data.owed=[withTwoOptions()];const accepted=recordPicks();
 await fullViewWhileHolding();expect(sent()).toHaveLength(0);
 await click('[data-queue-id="sample"]');await click('[data-option="b"]');
 expect(accepted.get("sample")).toBe("b");
 expect(host.querySelector("[data-pick-hold]")).toBeNull();
 await act(async()=>{await vi.advanceTimersByTimeAsync(6000)});
 expect(accepted.get("sample")).toBe("b");
 expect(host.querySelector("[data-pick-hold-failure]"),"the successful B must not be followed by a failed A").toBeNull();
 expect(sent().map(([,req])=>[req.decision_id,req.option_id])).toEqual([["sample","b"]]);
});
it("Full view's B on a different card leaves A held and sends both once",async()=>{
 data.owed=[withTwoOptions(),{...withTwoOptions(),id:"other",task_id:"other"}];const accepted=recordPicks();
 await fullViewWhileHolding();await click('[data-queue-id="other"]');await click('[data-option="b"]');
 expect(accepted.get("other")).toBe("b");expect(accepted.has("sample")).toBe(false);
 expect(host.querySelector("[data-pick-undo]")).not.toBeNull();
 await act(async()=>{await vi.advanceTimersByTimeAsync(5000)});
 expect(accepted.get("sample")).toBe("a");expect(sent()).toHaveLength(2);
 expect(host.querySelector("[data-pick-hold-failure]")).toBeNull();
});
it("Undo of inbox A still sends nothing after opening Full view",async()=>{
 data.owed=[withTwoOptions()];const accepted=recordPicks();
 await fullViewWhileHolding();await click("[data-pick-undo]");
 await act(async()=>{await vi.advanceTimersByTimeAsync(6000)});
 expect(accepted.size).toBe(0);expect(sent()).toHaveLength(0);
 expect(host.querySelector('[data-queue-id="sample"]')).not.toBeNull();
});

it("a width change during A's hold still lets desktop B supersede A",async()=>{
 data.owed=[withTwoOptions()];const accepted=recordPicks();
 await mount();await click('[data-inbox-row="ask:sample"]');await click("[data-bar-main]");
 await act(async()=>{phoneWidth=false;for(const listener of widthListeners)listener();});
 expect(host.querySelector("[data-mobile-inbox]")).toBeNull();
 expect(host.querySelector('[data-queue-tab="decide"]')?.textContent).toBe("Needs you now (1)");
 await click('[data-queue-id="sample"]');await click('[data-option="b"]');
 await act(async()=>{await vi.advanceTimersByTimeAsync(6000)});
 expect(accepted.get("sample")).toBe("b");
 expect(sent().map(([,req])=>[req.decision_id,req.option_id])).toEqual([["sample","b"]]);
 expect(host.querySelector("[data-pick-hold-failure]")).toBeNull();
});
it.each(["conflict","network"])("failed replacement B (%s) stays answerable without sending A",async(kind)=>{
 data.owed=[withTwoOptions()];const accepted=recordPicks();
 const original=sdk.rpc.call.getMockImplementation()!;
 let fail=true;
 sdk.rpc.call.mockImplementation(async(method:string,input?:{decision_id:string;option_id:string})=>{
  if(method==="pick" && input?.option_id==="b" && fail){
   fail=false;
   if(kind==="network")throw new Error("Sample connection lost");
   return {ok:false,status:409,error:"Sample revision changed"};
  }
  return original(method,input);
 });
 await fullViewWhileHolding();await click('[data-queue-id="sample"]');await click('[data-option="b"]');
 const failure=host.querySelector("[data-pick-failure]");
 expect(failure?.textContent).toContain(kind==="network"?"Sample connection lost":"Sample revision changed");
 expect(host.querySelector("[data-pick-hold]")).toBeNull();
 await act(async()=>{await vi.advanceTimersByTimeAsync(6000)});
 expect(accepted.size).toBe(0);
 expect(sent().map(([,req])=>req.option_id)).toEqual(["b"]);
 expect(host.querySelector('[data-queue-id="sample"]')).not.toBeNull();
 expect((host.querySelector('[data-option="b"]') as HTMLButtonElement).disabled).toBe(false);
 await click('[data-option="b"]');
 expect(accepted.get("sample")).toBe("b");
 expect(sent().map(([,req])=>req.option_id)).toEqual(["b","b"]);
 expect(host.querySelector("[data-pick-failure],[data-pick-hold-failure]")).toBeNull();
});
