// @vitest-environment jsdom
import { act, StrictMode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
const { rpc }=vi.hoisted(()=>({rpc:{call:vi.fn()}}));
vi.mock("@get-bb/plugin-sdk/app",()=>({useRpc:()=>rpc}));
import { EvidenceBlock } from "../ui/evidence.js";
import type { Evidence } from "../evidence.js";
Object.assign(globalThis,{IS_REACT_ACT_ENVIRONMENT:true});
let host:HTMLDivElement,root:Root;
const image=(c:string)=>({image:{sha256:c.repeat(64),mime:"image/png" as const}});
const items:Evidence[]=[{caption:"Header",before:image("a"),after:image("b")},{caption:"Answers",before:image("c"),after:image("d")},{caption:"Overview",...image("e")},{caption:"Reference",url:"https://sample.example.test/"}];
beforeEach(()=>{
 let n=0;Object.defineProperty(URL,"createObjectURL",{configurable:true,value:vi.fn(()=>`blob:sample-${n++}`)});Object.defineProperty(URL,"revokeObjectURL",{configurable:true,value:vi.fn()});
 Object.defineProperty(HTMLDialogElement.prototype,"showModal",{configurable:true,value:vi.fn(function(this:HTMLDialogElement){this.open=true})});
 Object.defineProperty(HTMLDialogElement.prototype,"close",{configurable:true,value:vi.fn(function(this:HTMLDialogElement){this.open=false})});
 rpc.call.mockReset().mockResolvedValue({ok:true,mime:"image/png",data_base64:"iVBORw0KGgo="});
 host=document.createElement("div");document.body.append(host);root=createRoot(host);
});
afterEach(()=>{act(()=>root.unmount());host.remove();});
const mount=(e=items)=>act(async()=>root.render(<EvidenceBlock items={e}/>));
const q=(s:string)=>host.querySelector<HTMLElement>(s)!;
const click=(s:string)=>act(()=>q(s).click());
const open=()=>{const button=q('button[aria-label="Enlarge: Header: Before"]');button.focus();act(()=>button.click());return button;};
const key=(value:string)=>act(()=>q("dialog").dispatchEvent(new KeyboardEvent("keydown",{key:value,bubbles:true,cancelable:true})));
it.each(["click","Enter"])("opens on %s using the already-loaded URL",async(method)=>{
 await mount();const button=q('button[aria-label^="Enlarge:"]');expect(button.tagName).toBe("BUTTON");button.focus();
 act(()=>{if(method==="click")button.click();else { // jsdom lacks native button key activation; model the UA default action.
  const event=new KeyboardEvent("keydown",{key:"Enter",bubbles:true,cancelable:true});if(button.dispatchEvent(event))button.click();
 }});
 expect(q("dialog").hasAttribute("open")).toBe(true);expect(HTMLDialogElement.prototype.showModal).toHaveBeenCalledOnce();
 expect(document.activeElement).toBe(q('[aria-label="Close image preview"]'));
 expect(q("h2").textContent).toBe("Header: Before");expect(q("[data-evidence-full-image]").getAttribute("src")).toBe("blob:sample-0");
 expect(rpc.call).toHaveBeenCalledTimes(5);
});
it.each(["button","Escape","backdrop"])("closes via %s and returns focus",async(method)=>{
 await mount();const origin=open();if(method==="button")click('[aria-label="Close image preview"]');else if(method==="Escape")key("Escape");else click("dialog");
 expect(q("dialog")).toBeNull();expect(document.activeElement).toBe(origin);
});
it("steps through pairs and singles with keys, buttons and swipe, resetting natural size",async()=>{
 await mount();open();click('[aria-label="Next image"]');expect(q("h2").textContent).toBe("Header: After");
 key("ArrowRight");expect(q("h2").textContent).toBe("Answers: Before");key("ArrowLeft");expect(q("h2").textContent).toBe("Header: After");
 click('[aria-label="Previous image"]');expect(q("h2").textContent).toBe("Header: Before");key("ArrowLeft");expect(q("h2").textContent).toBe("Overview");
 click("[data-evidence-full-image]");expect(Array.from(host.querySelectorAll("button")).find(b=>b.textContent==="Fit to viewport")?.textContent).toBe("Fit to viewport");
 key("ArrowRight");expect(q("h2").textContent).toBe("Header: Before");expect(Array.from(host.querySelectorAll("button")).find(b=>b.textContent==="Natural size")?.textContent).toBe("Natural size");
 const swipe=(type:string,x:number)=>{const e=new Event(type,{bubbles:true});Object.defineProperty(e,type==="touchstart"?"touches":"changedTouches",{value:[{clientX:x,clientY:100}]});q("[data-evidence-viewport]").dispatchEvent(e);};
 act(()=>{swipe("touchstart",200);swipe("touchend",50)});expect(q("h2").textContent).toBe("Header: After");
 expect(rpc.call).toHaveBeenCalledTimes(5);
});
it("unavailable images cannot be activated and URL evidence stays an external link",async()=>{
 rpc.call.mockResolvedValue({ok:false});await mount();expect(host.querySelectorAll('button[aria-label^="Enlarge:"]')).toHaveLength(0);expect(host.textContent).toContain("image unavailable");
 const a=q("a");expect(a.getAttribute("href")).toBe("https://sample.example.test/");expect(a.getAttribute("target")).toBe("_blank");expect(a.getAttribute("rel")).toBe("noopener noreferrer");
});

it("a failed image decode becomes an unavailable noninteractive placeholder",async()=>{
 await mount([{caption:"Sample",...image("a")}]);act(()=>q("img").dispatchEvent(new Event("error")));
 expect(host.textContent).toContain("image unavailable");expect(host.querySelector("button,img,dialog")).toBeNull();
});
it("removing and restoring evidence does not reopen a stale viewer",async()=>{
 await mount();open();await mount([{caption:"Reference",url:"https://sample.example.test/"}]);expect(q("dialog")).toBeNull();
 await mount();expect(q("dialog")).toBeNull();open();expect(q("dialog").hasAttribute("open")).toBe(true);
});

it("contains keys from the image and body without firing host handlers",async()=>{
 await mount();open();const hostKey=vi.fn();window.addEventListener("keydown",hostKey);
 try{
  click("[data-evidence-full-image]");expect(document.activeElement).toBe(q("dialog"));
  for(const value of ["[","]","x","Tab"]){act(()=>document.body.dispatchEvent(new KeyboardEvent("keydown",{key:value,bubbles:true})));}
  act(()=>document.body.dispatchEvent(new KeyboardEvent("keydown",{key:"ArrowRight",bubbles:true,cancelable:true})));
  expect(q("h2").textContent).toBe("Header: After");
  act(()=>document.body.dispatchEvent(new KeyboardEvent("keydown",{key:"ArrowLeft",bubbles:true,cancelable:true})));
  expect(q("h2").textContent).toBe("Header: Before");
  act(()=>document.body.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true})));
  expect(q("dialog")).toBeNull();expect(hostKey).not.toHaveBeenCalled();
  document.body.dispatchEvent(new KeyboardEvent("keydown",{key:"[",bubbles:true}));expect(hostKey).toHaveBeenCalledOnce();
 }finally{window.removeEventListener("keydown",hostKey);}
});
it("returns focus to a tapped trigger even when it was never focused, including StrictMode",async()=>{
 await act(async()=>root.render(<StrictMode><EvidenceBlock items={items}/></StrictMode>));
 const origin=q('button[aria-label^="Enlarge:"]');expect(document.activeElement).not.toBe(origin);act(()=>origin.click());
 expect(document.activeElement).toBe(q('[aria-label="Close image preview"]'));
 const controls=Array.from(host.querySelectorAll("dialog button"));
 for(const control of controls){expect(control.className).toContain("sm:!min-h-11");expect(control.className).toContain("border");expect(control.className).toContain("hover:bg-state-hover");expect(control.hasAttribute("aria-pressed")).toBe(false);}
 click('[aria-label="Close image preview"]');expect(document.activeElement).toBe(origin);
 expect(q("dialog")).toBeNull();
});
it("natural-size gestures pan without navigation and fit allows pinch zoom",async()=>{
 await mount();open();expect(q("[data-evidence-viewport]").style.touchAction).toBe("pan-y pinch-zoom");
 expect(q("[data-evidence-viewport]").className).toContain("overscroll-contain");click("[data-evidence-full-image]");
 const swipe=(type:string,x:number)=>{const event=new Event(type,{bubbles:true});Object.defineProperty(event,type==="touchstart"?"touches":"changedTouches",{value:[{clientX:x,clientY:100}]});q("[data-evidence-viewport]").dispatchEvent(event);};
 act(()=>{swipe("touchstart",200);swipe("touchend",50)});expect(q("h2").textContent).toBe("Header: Before");
 expect(q("[data-evidence-viewport]").style.touchAction).toBe("auto");
 key("ArrowRight");expect(q("[data-evidence-full-image]").className).not.toContain("max-w-none");
});
