// @vitest-environment jsdom
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));
import { HomeOverflow } from "../ui/tabs.js";
import { QueueSurface } from "../ui/inbox.js";
import type { AsksData, OwedAsk } from "../ui/asks.js";
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const host = document.createElement("div");
let root: ReturnType<typeof createRoot>;
const mount = (node: ReactNode) => { document.body.append(host); root = createRoot(host); act(()=>root.render(node)); };
afterEach(()=>{ act(()=>root.unmount()); host.remove(); vi.useRealTimers(); });
it("More closes on Escape with focus return, selection, and outside pointer", ()=>{
 const open = vi.fn(); mount(<HomeOverflow learn onOpen={open}/>);
 const details = host.querySelector("details")!;
 const summary = host.querySelector("summary")!;
 const items = Array.from(host.querySelectorAll("button"));
 expect(items.map(b=>b.textContent)).toEqual(["Ideas","Learn","Archive","Vizier","Settings"]);
 for (const item of items) {
  details.open = true; item.focus(); expect(document.activeElement).toBe(item);
  act(()=>item.click()); expect(details.open).toBe(false);
 }
 expect(open.mock.calls.map(c=>c[0].id)).toEqual(["ideas","learn","archive","vizier","settings"]);
 details.open = true; items[0]!.focus();
 act(()=>items[0]!.dispatchEvent(new KeyboardEvent("keydown", {key:"Escape",bubbles:true})));
 expect(details.open).toBe(false); expect(document.activeElement).toBe(summary);
 details.open = true;
 act(()=>document.body.dispatchEvent(new Event("pointerdown", {bubbles:true})));
 expect(details.open).toBe(false);
});
it("the production phone surface reaches the inbox and five-second pick Undo", async()=>{
 vi.useFakeTimers();
 const pick = vi.fn(async()=>({ok:true as const}));
 const ask: OwedAsk = {id:"sample",project:"Example",thread:"sample",subject:"Sample choice",asker:"thread",filed_at:"2026-11-09T07:00:00Z",revision:"sample",task_id:"sample",ask:{question:"Choose a sample",recommendation:"a",options:[{id:"a",label:"Continue",kind:"ruling-only",reversible:true}]}};
 const asks: AsksData = {owed:[ask],runbook:[],lane:[],asks:[],undeliverable:[],failures:[],uncertain:[],delegation:{settings:{},suspended:false},machineOwners:{}};
 mount(<QueueSurface phone inbox={{asks,waiting:1,onPick:pick,onOpen:()=>{},onNote:()=>{},onLater:()=>{},sinceNode:null,sinceCount:0}}><p data-full-view>Full queue</p></QueueSurface>);
 const click = async(sel:string)=>act(async()=>{(host.querySelector(sel) as HTMLElement).click()});
 expect(host.querySelector("[data-mobile-inbox]")).not.toBeNull();
 expect(host.querySelector("[data-inbox-tabs],[data-inbox-title],[data-inbox-extras]")).toBeNull();
 await click("[data-inbox-row]"); await click("[data-bar-main]");
 expect(host.querySelector("[data-pick-hold]")?.textContent).toContain("sending in 5 s");
 expect(pick).not.toHaveBeenCalled();
 await click("[data-pick-undo]"); await act(async()=>{await vi.advanceTimersByTimeAsync(5000)});
 expect(pick).not.toHaveBeenCalled();
 if(host.querySelector("[data-inbox-back]")) await click("[data-inbox-back]"); await click("[data-inbox-desktop]");
 expect(host.querySelector("[data-full-view]")).not.toBeNull();
 await click("[data-phone-inbox-back]"); expect(host.querySelector("[data-mobile-inbox]")).not.toBeNull();

});

it("closed More lets Escape reach the page handler",()=>{
 mount(<HomeOverflow onOpen={()=>{}}/>);
 const escape=vi.fn();window.addEventListener("keydown",escape);
 try{
  const summary=host.querySelector("summary")!;
  const event=new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true});
  act(()=>summary.dispatchEvent(event));expect(event.defaultPrevented).toBe(false);expect(escape).toHaveBeenCalledOnce();
  host.querySelector("details")!.open=true;
  act(()=>summary.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true})));
  expect(escape).toHaveBeenCalledOnce();
 }finally{window.removeEventListener("keydown",escape);}
});
