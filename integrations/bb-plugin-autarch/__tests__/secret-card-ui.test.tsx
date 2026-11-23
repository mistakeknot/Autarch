// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { homeMethods } from "../contract.js";
const mocks = vi.hoisted(() => ({ list: vi.fn(), respond: vi.fn(), rpc: vi.fn(), comment: vi.fn(), sent: [] as unknown[], methods: new Map<string, ReturnType<typeof vi.fn>>() }));
vi.mock("@get-bb/plugin-sdk/app", () => {
 const sdk = { threads: { interactions: { list: mocks.list, respond: mocks.respond } }, tasks: { comment: mocks.comment } };
 return { ThreadChat: () => null, useSdk: () => sdk, useRpc: () => ({ call: (method: string, ...args: unknown[]) => { mocks.rpc(method,...args); return mocks.methods.get(method)?.(...args); } }) };
});
import { AskCard, type OwedAsk } from "../ui/asks.js";
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const meta = { thread: "thr_23456789ab", names: ["FAKE_TEST_TOKEN"], path: "/fake-test/credentials.env", project: "fake-test-project" };
const interaction = () => ({ id: "pint_23456789ab", threadId: meta.thread, createdAt: Date.now(), expiresAt: Date.now()+60000, origin: { kind: "plugin", pluginId: "secrets", rendererId: "secret-request" }, status: "pending", resolution: null, payload: { kind: "plugin", title: "Add secrets", data: { threadId: meta.thread, project: meta.project, destination: { kind: "dotenv", path: meta.path, consumer: "fake-test-consumer" }, expiresAtMs: Date.now()+60000, fields: [{ name: "FAKE_TEST_TOKEN", description: null, existing: false }] } } });
let host: HTMLDivElement; let root: Root;
const pick = vi.fn(); const note = vi.fn();
beforeEach(() => { vi.clearAllMocks(); mocks.methods.clear(); for (const method of Object.keys(homeMethods)) mocks.methods.set(method,vi.fn()); mocks.sent.length=0; host=document.createElement("div"); document.body.append(host); root=createRoot(host); mocks.list.mockResolvedValue([interaction()]); mocks.respond.mockImplementation(async (args: unknown) => { mocks.sent.push(structuredClone(args)); return { ...interaction(), status: "resolved", resolution: { kind: "plugin_submitted" } }; }); });
afterEach(() => { act(() => root.unmount()); host.remove(); vi.restoreAllMocks(); });
async function mount() {
 const ask = { id:"fake-decision", thread:meta.thread, subject:"Provide credential", project:meta.project, asker:"thread", filed_at:"2026-01-01", revision:"fake-revision", secret:meta, ask:{question:"Provide credential",options:[]} } as OwedAsk;
 await act(async () => root.render(<AskCard ask={ask} onPick={pick} onNote={note} onOpen={() => {}} />));
}
it("shows a waiting masked form with the destination, consumer and expiry", async () => {
 await mount(); expect(host.querySelector('input[type="password"]')).not.toBeNull();
 expect(host.textContent).toContain(meta.path); expect(host.textContent).toContain("fake-test-consumer"); expect(host.textContent).toContain("Expires");
});
it("shows expired with no input when no request remains", async () => {
 mocks.list.mockResolvedValue([]); await mount(); expect(host.textContent).toContain("re-request"); expect(host.querySelector("input")).toBeNull();
});
it("refuses expired matching interactions", async () => {
 const i=interaction(); i.payload.data.expiresAtMs=Date.now()-1; mocks.list.mockResolvedValue([i]); await mount(); expect(host.textContent).toContain("re-request"); expect(host.querySelector("input")).toBeNull();
});
it("submits only to interactions.respond, clears inputs, and confirms without echo", async () => {
 const consoles = Object.getOwnPropertyNames(console).filter(k => typeof console[k as keyof Console] === "function") as (keyof Console)[];
 const spies = consoles.map(k => vi.spyOn(console as unknown as Record<string, (...args: unknown[]) => void>,k).mockImplementation(() => {}));
 const allRpcs = [...mocks.methods.values()];
 await mount(); const input=host.querySelector('input[name="FAKE_TEST_TOKEN"]'); expect(input).not.toBeNull();
 const value="fake-test-value-not-a-secret";
 await act(async () => { Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,"value")!.set!.call(input,value); input!.dispatchEvent(new Event("input",{bubbles:true})); });
 await act(async () => host.querySelector("form")!.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true})));
 expect(mocks.respond).toHaveBeenCalledTimes(1); expect(mocks.sent).toEqual([{threadId:meta.thread,interactionId:"pint_23456789ab",value:{values:{FAKE_TEST_TOKEN:value}}}]);
 expect(host.textContent).toContain("Submitted"); expect(host.innerHTML).not.toContain(value); expect(host.querySelector("input")).toBeNull();
 for(const spy of [pick,note,mocks.rpc,mocks.comment,...allRpcs,...spies]) expect(JSON.stringify(spy.mock.calls)).not.toContain(value);
 expect(localStorage.length).toBe(0); expect(location.href).not.toContain(value);
});

it("rejects mismatched and ambiguous waiters and requests exceeding one hour", async () => {
 for (const change of ["project", "path", "name", "renderer", "plugin", "ambiguous", "ttl"]) {
  const i=interaction();
  if(change==="project") i.payload.data.project="other-fake-project";
  if(change==="path") i.payload.data.destination.path="/other-fake/credentials.env";
  if(change==="name") i.payload.data.fields[0]!.name="OTHER_FAKE_TOKEN";
  if(change==="renderer") i.origin.rendererId="other-renderer";
  if(change==="plugin") i.origin.pluginId="other-plugin";
  if(change==="ttl") i.payload.data.expiresAtMs=Date.now()+3_600_001;
  mocks.list.mockResolvedValue(change==="ambiguous"?[i,{...i,id:"pint_abcdefghij"}]:[i]);
  await act(async()=> root.render(null)); await mount();
  expect(host.querySelector("input"),change).toBeNull(); expect(host.textContent).toContain("re-request");
 }
});
it("clears the value and shows a constant error when the SDK rejects", async () => {
 const value="fake-test-value-not-a-secret";
 mocks.respond.mockRejectedValue(new Error(value));
 await mount();const input=host.querySelector("input")!;
 await act(async()=> {input.value=value;host.querySelector("form")!.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true}));});
 expect(input.value).toBe("");expect(host.innerHTML).not.toContain(value);expect(host.querySelector('[role="alert"]')).not.toBeNull();
 expect(pick).not.toHaveBeenCalled();expect(note).not.toHaveBeenCalled();expect(mocks.rpc).not.toHaveBeenCalled();
});

it("shows submitted from a resolved interaction without inputs", async () => {
 mocks.list.mockResolvedValue([{...interaction(),status:"resolved",resolution:{kind:"plugin_submitted"}}]);
 await mount(); expect(host.textContent).toContain("Submitted"); expect(host.querySelector("input")).toBeNull();
});

it("known non-secret cards show text entry without a metadata RPC",async()=>{
 const ask={id:"fake-decision",task_id:"fake-task",secret:null,thread:meta.thread,subject:"Choose",project:meta.project,asker:"thread",filed_at:"2026-01-01",revision:"fake-revision",ask:{question:"Choose",options:[]}} as OwedAsk;
 await act(async()=>root.render(<AskCard ask={ask} onPick={pick} onNote={note} onOpen={()=>{}} />));
 expect(host.querySelector("[data-other-text]")).not.toBeNull();expect(mocks.rpc).not.toHaveBeenCalled();expect(mocks.list).not.toHaveBeenCalled();
});

it("an expired request has no masked form and keeps its normal card options", async () => {
 const i=interaction();i.expiresAt=Date.now()-1;i.payload.data.expiresAtMs=Date.now()-1;mocks.list.mockResolvedValue([i]);
 const ask={id:"fake-decision",thread:meta.thread,subject:"Provide credential",project:meta.project,asker:"thread",filed_at:"2026-01-01",revision:"fake-revision",secret:meta,ask:{question:"Provide credential",options:[{id:"retry",label:"Request again",kind:"ruling-only"},{id:"later",label:"Later",kind:"ruling-only"}]}} as OwedAsk;
 await act(async()=>root.render(<AskCard ask={ask} onPick={pick} onNote={note} onOpen={()=>{}} />));
 expect(host.textContent).toContain("expired");expect(host.querySelector("form")).toBeNull();expect(host.querySelector("input")).toBeNull();
 const retry=host.querySelector<HTMLButtonElement>('[data-option="retry"]')!;expect(retry).not.toBeNull();expect(retry.disabled).toBe(false);expect(host.querySelector('[data-option="later"]')).not.toBeNull();
 await act(async()=>retry.click());expect(pick).toHaveBeenCalledWith("fake-decision","retry","fake-revision");expect(mocks.respond).not.toHaveBeenCalled();
});

it("prefers a live pending match over resolved and expired history", async () => {
 const old={...interaction(),status:"resolved",resolution:{kind:"plugin_submitted"}};
 const expired=interaction();expired.expiresAt=Date.now()-1;expired.payload.data.expiresAtMs=Date.now()-1;
 const pending={...interaction(),id:"pint_abcdefghij"};mocks.list.mockResolvedValue([old,expired,pending]);
 await mount();const input=host.querySelector<HTMLInputElement>('input[name="FAKE_TEST_TOKEN"]');expect(input).not.toBeNull();
 input!.value="fake-test-value-not-a-secret";
 await act(async()=>host.querySelector("form")!.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true})));
 expect(mocks.sent).toMatchObject([{interactionId:pending.id}]);
});
it("confirms submission even when respond returns no payload", async()=>{
 mocks.respond.mockResolvedValue(undefined);await mount();host.querySelector<HTMLInputElement>("input")!.value="fake-test-value-not-a-secret";
 await act(async()=>host.querySelector("form")!.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true})));
 expect(host.textContent).toContain("Submitted");expect(host.querySelector("input")).toBeNull();
});
it("does not match a destination with an extra trailing slash",async()=>{
 const i=interaction();i.payload.data.destination.path+="/";mocks.list.mockResolvedValue([i]);await mount();expect(host.querySelector("input")).toBeNull();
});
it("explains rejected line breaks and uses card button and mobile input styling",async()=>{
 await mount();const input=host.querySelector<HTMLInputElement>("input")!;
 expect(input.className).toContain("text-base");expect(host.querySelector('[aria-label="Show FAKE_TEST_TOKEN"]')!.className).toContain("border-input");
 Object.defineProperty(input,"value",{get:()=>"fake-test\nvalue",configurable:true});
 await act(async()=>host.querySelector("form")!.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true})));
 expect(host.textContent).toContain("Values cannot contain line breaks");expect(mocks.respond).not.toHaveBeenCalled();
});
it("explains unavailable read-model metadata instead of silently removing text entry",async()=>{
 const ask={id:"fake-decision",task_id:"fake-task",secret_unavailable:true,thread:meta.thread,subject:"Choose",project:meta.project,asker:"thread",filed_at:"2026-01-01",revision:"fake-revision",ask:{question:"Choose",options:[]}} as OwedAsk;
 await act(async()=>root.render(<AskCard ask={ask} onPick={pick} onNote={note} onOpen={()=>{}} />));
 expect(host.textContent).toContain("Text entry unavailable: could not check this card for a secret request");expect(host.querySelector("[data-other-text]")).toBeNull();
 expect(Array.from(host.querySelectorAll('[role="status"]')).some(p=>p.textContent?.includes("Text entry unavailable"))).toBe(false);
});

for (const mode of ["missing", "unrelated", "same-pending", "new-pending", "list-error"] as const) {
 it(`polling after submit: ${mode}`,async()=>{
  vi.useFakeTimers();
  try {
   await mount();host.querySelector<HTMLInputElement>("input")!.value="fake-test-value-not-a-secret";
   await act(async()=>host.querySelector("form")!.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true})));
   expect(host.textContent).toContain("Submitted");
   if(mode==="list-error")mocks.list.mockRejectedValue(new Error("fake-test-error"));
   else if(mode==="missing")mocks.list.mockResolvedValue([]);
   else if(mode==="unrelated"){const unrelated=interaction();unrelated.payload.data.project="unrelated-fake-project";mocks.list.mockResolvedValue([unrelated]);}
   else mocks.list.mockResolvedValue([{...interaction(),id:mode==="new-pending"?"pint_abcdefghij":"pint_23456789ab"}]);
   await act(async()=>vi.advanceTimersByTimeAsync(5000));
   if(mode==="new-pending"){
    expect(host.querySelector('input[type="password"]')).not.toBeNull();
    host.querySelector<HTMLInputElement>("input")!.value="fake-test-value-not-a-secret";
    await act(async()=>host.querySelector("form")!.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true})));
    expect(mocks.sent).toMatchObject([{interactionId:"pint_23456789ab"},{interactionId:"pint_abcdefghij"}]);
   }else{expect(host.textContent).toContain("Submitted");expect(host.querySelector("form")).toBeNull();}
  } finally {vi.useRealTimers();}
 });
}
