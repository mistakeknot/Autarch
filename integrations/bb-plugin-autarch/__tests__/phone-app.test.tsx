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
beforeEach(()=>{
 vi.useFakeTimers();localStorage.clear();
 vi.stubGlobal("matchMedia",()=>({matches:true,addEventListener:()=>{},removeEventListener:()=>{}}));
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
