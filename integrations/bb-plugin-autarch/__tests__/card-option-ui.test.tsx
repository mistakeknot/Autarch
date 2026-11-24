// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
vi.mock("@get-bb/plugin-sdk/app",()=>({ThreadChat:()=>null}));
import { AsksPanel, type AsksData, type OwedAsk } from "../ui/asks.js";
import * as links from "../ui/cardlink.js";

const ask:OwedAsk={id:"fixture-decision",key:"FAKE-1",project:"fixture",thread:"thr_fixture",subject:"Compare options",asker:"thread",filed_at:"2030-01-01T00:00:00Z",revision:"fixture-revision",ask:{question:"Which variant?",options:[{id:"first",label:"First variant",kind:"ruling-only"},{id:"second",label:"Second variant",kind:"ruling-only"},{id:"delete",label:"Delete variant",kind:"ruling-only"}]}};
const data=(card:OwedAsk|null):AsksData=>({owed:card?[card]:[],runbook:[],lane:[],asks:[],undeliverable:[],failures:[],uncertain:[],delegation:{settings:{},suspended:false},machineOwners:{}});
let root:Root,host:HTMLDivElement;
const rpc=vi.fn(async(..._args:unknown[])=>({ok:true}));
const pick=vi.fn((...args:unknown[])=>rpc("pick",args));
const note=vi.fn((...args:unknown[])=>rpc("note",args));
const view=vi.fn((...args:unknown[])=>{void rpc("setViewing",args);});
const submit=vi.fn();
beforeEach(()=>{vi.clearAllMocks();(globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;host=document.createElement("div");document.body.append(host);host.addEventListener("submit",submit);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.restoreAllMocks();});
async function mount(option="first",card:OwedAsk|null=ask) {
  const choose=(links as any).cardOptionSelection;
  const result=choose?choose(option,card):{option:null,notice:null};
  const Notice=(links as any).CardOptionNotice;
  await act(async()=>root.render(<>{Notice?createElement(Notice,{notice:result.notice}):null}<AsksPanel {...({preselect:{key:"FAKE-1",option}} as any)} data={data(card)} onPick={pick} onNote={note} onOpen={()=>{}} onView={view} /></>));
}
it("preselects without a pick, RPC write, submit or confirmation; mk's click answers",async()=>{
  await mount();
  expect(host.querySelector('[data-option="first"]')?.getAttribute("aria-pressed")).toBe("true");
  expect(host.querySelector('[data-option="second"]')?.getAttribute("aria-pressed")).not.toBe("true");
  expect(host.querySelector("[data-confirming]")).toBeNull();
  for(const spy of [rpc,pick,note,view,submit])expect(spy).not.toHaveBeenCalled();
  await act(async()=>host.querySelector<HTMLButtonElement>('[data-option="first"]')!.click());
  expect(pick).toHaveBeenCalledTimes(1);expect(pick).toHaveBeenCalledWith(ask.id,"first",ask.revision);
});
it("makes no RPC write merely by opening the option target",async()=>{
  await mount();for(const spy of [rpc,pick,note,view,submit])expect(spy).not.toHaveBeenCalled();
});
it("can open a later card without moving it back or answering",async()=>{
  await mount("first",{...ask,later:{at:"2030-01-01",by:"fixture"}});
  expect(host.querySelector('[data-later] details')?.hasAttribute("open")).toBe(true);
  expect(host.querySelector('[data-option="first"]')?.getAttribute("aria-pressed")).toBe("true");
  expect(rpc).not.toHaveBeenCalled();
});
it("does not arm destructive confirmation from a preselection",async()=>{
  await mount("delete");expect(host.querySelector("[data-confirming]")).toBeNull();expect(pick).not.toHaveBeenCalled();
  await act(async()=>host.querySelector<HTMLButtonElement>('[data-option="delete"]')!.click());
  expect(pick).not.toHaveBeenCalled();expect(host.querySelector('[data-option="delete"][data-confirming="true"]')).not.toBeNull();
  await act(async()=>host.querySelector<HTMLButtonElement>('[data-option="delete"]')!.click());expect(pick).toHaveBeenCalledTimes(1);
});
it.each(["unknown","First","first\n",""])("shows a notice for %j and keeps all normal controls",async option=>{
  await mount(option);expect(host.querySelector('[role="status"]')?.textContent).toBeTruthy();
  expect(host.querySelector('[data-option][aria-pressed="true"]')).toBeNull();expect(host.querySelectorAll("[data-option]")).toHaveLength(3);
  for(const spy of [rpc,pick,note,view,submit])expect(spy).not.toHaveBeenCalled();
});
it("opens a missing or non-open card target with a notice and no selected option",async()=>{
  await mount("first",null);expect(host.querySelector('[role="status"]')?.textContent).toBeTruthy();expect(host.querySelector('[aria-pressed="true"]')).toBeNull();expect(rpc).not.toHaveBeenCalled();
});
it.each(["approval","on_pick"])("never preselects an option with %s",async field=>{
  const extra=field==="approval" ? {approval:{kind:"merge",target:"fixture",identity:"a".repeat(40)}} : {on_pick:{schema:1,target_thread:"thr_fixture",reversible:true,subject:{kind:"none"},ruling:{bead:null,under:[]},notification:"fixture"}};
  const card={...ask,ask:{...ask.ask,options:[{...ask.ask.options[0],...extra},ask.ask.options[1]]}} as OwedAsk;
  await mount("first",card);expect(host.querySelector('[data-option][aria-pressed="true"]')).toBeNull();expect(host.querySelector('[role="status"]')?.textContent).toBeTruthy();expect(rpc).not.toHaveBeenCalled();
});
it("rerenders a new option target without answering or carrying the old highlight",async()=>{
  await mount("first");await mount("second");
  expect(host.querySelector('[data-option="second"]')?.getAttribute("aria-pressed")).toBe("true");
  expect(host.querySelector('[data-option="first"]')?.getAttribute("aria-pressed")).not.toBe("true");expect(rpc).not.toHaveBeenCalled();
});
it("wires the option target through both the focused and fallback app panels",()=>{
  const src=readFileSync(new URL("../app.tsx",import.meta.url),"utf8");
  expect(src).toContain("cardOpenTargetFrom(subPath)");
  const panels=src.split("<AsksPanel").slice(1);expect(panels.length).toBeGreaterThanOrEqual(2);
  for(const p of panels)expect(p.slice(0,p.indexOf("/>"))).toContain("preselect={preselect}");
});
