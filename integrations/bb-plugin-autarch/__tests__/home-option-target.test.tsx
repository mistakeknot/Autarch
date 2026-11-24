// @vitest-environment jsdom
import { act, type ComponentType } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { AsksData, OwedAsk } from "../ui/asks.js";

const sdk=vi.hoisted(()=>({
  rpc:{call:vi.fn()},
  nav:{toPluginPanel:vi.fn(),toThread:vi.fn(),openThreadPanel:vi.fn()},
}));
vi.mock("@get-bb/plugin-sdk/app",()=>({
  definePluginApp:(setup:(app:unknown)=>void)=>{
    const registered:{home?:unknown}={};
    setup({slots:{navPanel:(slot:{component:unknown})=>{registered.home=slot.component;},threadPanelAction:()=>{}}});
    return registered;
  },
  useRpc:()=>sdk.rpc,
  useBbNavigate:()=>sdk.nav,
  useRealtime:()=>{},
  ThreadChat:()=>null,
  experimental_Icon:()=>null,
}));
import plugin from "../app.js";
import { clickStarted } from "../ui/clicktiming.js";

const HomePage=(plugin as unknown as {home:ComponentType<{subPath?:string}>}).home;
const first:OwedAsk={id:"fixture-first",key:"FAKE-1",project:"fixture",thread:"thr_fixture",subject:"First comparison",asker:"thread",filed_at:"2030-01-01T00:00:00Z",revision:"fixture-revision",ask:{question:"Which variant?",options:[{id:"first",label:"First variant",kind:"ruling-only"},{id:"second",label:"Second variant",kind:"ruling-only"}]}};
const second:OwedAsk={...first,id:"fixture-second",key:"FAKE-2",subject:"Second comparison"};
let root:Root,host:HTMLDivElement;
const originalScroll=Object.getOwnPropertyDescriptor(HTMLElement.prototype,"scrollIntoView");
beforeEach(()=>{
  vi.clearAllMocks();localStorage.clear();
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
  vi.stubGlobal("requestAnimationFrame",(cb:FrameRequestCallback)=>{cb(0);return 1;});
  Object.defineProperty(HTMLElement.prototype,"scrollIntoView",{value:vi.fn(),configurable:true});
  const asks:AsksData={owed:[first,second],runbook:[],lane:[],asks:[],undeliverable:[],failures:[],uncertain:[],delegation:{settings:{},suspended:false},machineOwners:{}};
  sdk.rpc.call.mockImplementation(async(name:string)=>{
    switch(name){
      case "listAsks":return asks;
      case "queue":return {rows:[],bindings:[],inactive_projects:[],legacy:{count:0,owed:[],runbook:[],machine:{lane:[],asks:[]}}};
      case "catchup":return {items:[]};
      case "health":return {ready:true};
      case "waiting":return {decide:2,later:0,held:0,updates:0,definition:0,suspended:false};
      case "conversationUnread":return {unread:{}};
      case "learn":return {ok:true,enabled:false,cards:[]};
      case "ideas":return {ok:true,ideas:[],show:false};
      case "updateInfo":return {status:null,pending:false};
      case "cardState":return {found:true,state:"picked",title:"Closed comparison"};
      case "setViewing":case "clickTiming":return {ok:true};
      default:throw new Error(`Unexpected fixture RPC: ${name}`);
    }
  });
  host=document.createElement("div");document.body.append(host);root=createRoot(host);
});
afterEach(async()=>{
  await act(async()=>root.unmount());host.remove();vi.restoreAllMocks();vi.unstubAllGlobals();
  if(originalScroll)Object.defineProperty(HTMLElement.prototype,"scrollIntoView",originalScroll);
  else delete (HTMLElement.prototype as any).scrollIntoView;
});
const calls=(method:string)=>sdk.rpc.call.mock.calls.filter(([name])=>name===method);

it("renders an open option subPath as a suggestion without answering or writing viewing markers",async()=>{
  await act(async()=>root.render(<HomePage subPath="FAKE-1/option/first"/>));
  const card=host.querySelector('[data-decision="fixture-first"]')!;
  expect(card).not.toBeNull();
  const option=card.querySelector('[data-option="first"]')!;
  expect(option.getAttribute("data-suggested")).toBe("true");
  expect(option.getAttribute("aria-pressed")).not.toBe("true");
  const status=card.querySelector('[data-cujgel-suggestion]')!;
  expect(status.textContent).toBe("Suggested from Cujgel: tap it to answer.");
  expect(option.getAttribute("aria-describedby")).toBe(status.id);
  expect(host.querySelector('[data-card-option-notice]')).toBeNull();
  for(const method of ["pick","note","setViewing","clickTiming"])expect(calls(method)).toHaveLength(0);
});

it("renders a stale option subPath as a notice and leaves fallback viewing and paint timings working",async()=>{
  clickStarted();
  await act(async()=>root.render(<HomePage subPath="FAKE-3/option/first"/>));
  expect(host.querySelector('[data-card-option-notice]')?.textContent).toContain("not open");
  expect(host.querySelector('[data-suggested]')).toBeNull();
  expect(host.querySelector('[data-cujgel-suggestion]')).toBeNull();
  expect(calls("setViewing").map(([,args])=>args)).toContainEqual({decision_id:"fixture-first"});
  expect(calls("clickTiming").some(([,args])=>args.kind==="paint")).toBe(true);
  sdk.rpc.call.mockClear();
  await act(async()=>host.querySelector<HTMLButtonElement>('[data-queue-id="fixture-second"]')!.click());
  expect(host.querySelector('[data-decision="fixture-second"]')).not.toBeNull();
  expect(calls("setViewing").map(([,args])=>args)).toContainEqual({decision_id:"fixture-second"});
  expect(calls("clickTiming").some(([,args])=>args.kind==="paint")).toBe(true);
  expect(calls("pick")).toHaveLength(0);
});
