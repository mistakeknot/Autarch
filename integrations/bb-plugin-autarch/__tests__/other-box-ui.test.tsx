// @vitest-environment jsdom
// The Other box on a card, driven the way mk drives it (bead mk-1poq7): type, then press a button.
// Part 1: an ask with its own option id "other" still takes free text as an answer, and its own option still works.
// Part 2: Ask / note is enabled with text typed, sends, and never stays disabled behind a stuck send or image read.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { AskCard, type OwedAsk } from "../ui/asks.js";
import { IMG_READ_TIMEOUT_MS, SEND_TIMEOUT_MS } from "../ui/other.js";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const opt = (id: string, label: string) => ({ id, label, kind: "ruling-only" as const, reversible: true });
const ask = (options = [opt("a", "A"), opt("b", "B")]): OwedAsk => ({
  id: "dec1",
  project: "Autarch",
  thread: "thr-a",
  subject: "Which way?",
  asker: "thread",
  filed_at: "2026-11-01T10:00:00Z",
  revision: "rev1",
  mentions: 0,
  task_id: "task-1",
  ask: { question: "Which way?", options },
});
const OWN_OTHER = [opt("other", "Something else"), opt("a", "A")];
const TEXT = "x".repeat(157);

let host: HTMLDivElement;
let root: Root;
beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.useRealTimers();
});

type Handlers = { onPick?: (...a: unknown[]) => unknown; onNote?: (...a: unknown[]) => unknown };
const mount = (a: OwedAsk, h: Handlers = {}, unbound = false) => {
  const onPick = vi.fn(h.onPick ?? (() => ({ ok: true })));
  const onNote = vi.fn(h.onNote ?? (() => ({ ok: true })));
  act(() => root.render(<AskCard ask={a} onPick={onPick as never} onOpen={() => {}} onNote={onNote as never} unbound={unbound} />));
  return { onPick, onNote };
};
const q = <T extends Element = HTMLButtonElement>(sel: string) => host.querySelector<T>(sel);
const type = (text: string) => {
  const ta = q<HTMLTextAreaElement>("[data-other-text]")!;
  const set = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
  act(() => {
    set.call(ta, text);
    ta.dispatchEvent(new Event("input", { bubbles: true }));
  });
};
const click = async (el: Element) => { await act(async () => { el.dispatchEvent(new MouseEvent("click", { bubbles: true })); }); };
const noteBtn = () => q("[data-other-note]")!;
const answerBtn = () => q("[data-other-answer]")!;
const advance = async (ms: number) => { await act(async () => { await vi.advanceTimersByTimeAsync(ms); }); };

describe("an ask with its own option named other: the Other box", () => {
  it("offers Answer with this and Ask / note, both enabled once text is typed", () => {
    mount(ask(OWN_OTHER));
    expect(answerBtn()).not.toBeNull();
    expect(answerBtn().hasAttribute("disabled")).toBe(true); // nothing typed yet
    expect(noteBtn().hasAttribute("disabled")).toBe(true);
    type(TEXT);
    expect(answerBtn().hasAttribute("disabled")).toBe(false);
    expect(noteBtn().hasAttribute("disabled")).toBe(false);
    expect(q("[data-other-answer-reason]")).toBeNull();
  });

  it("records the words under the reserved own-words id, not under the ask's own other option", async () => {
    const { onPick } = mount(ask(OWN_OTHER));
    type(TEXT);
    await click(answerBtn());
    expect(onPick).toHaveBeenCalledTimes(1);
    expect(onPick.mock.calls[0]!.slice(0, 4)).toEqual(["dec1", "mk-own-words", "rev1", TEXT]);
    expect(q("[data-other-result='ok']")).not.toBeNull();
  });

  it("keeps the ask's own other option as a normal pick that carries no words", async () => {
    const { onPick } = mount(ask(OWN_OTHER));
    type(TEXT);
    await click(q("[data-option='other']")!);
    expect(onPick).toHaveBeenCalledTimes(1);
    expect(onPick.mock.calls[0]).toEqual(["dec1", "other", "rev1"]);
  });

  it("picks the next free reserved id when the ask also has a real mk-own-words option", async () => {
    const { onPick } = mount(ask([...OWN_OTHER, opt("mk-own-words", "Real")]));
    type(TEXT);
    await click(answerBtn());
    expect(onPick.mock.calls[0]![1]).toBe("mk-own-words-2");
  });

  it("an ask without an other option still answers under other", async () => {
    const { onPick } = mount(ask());
    type(TEXT);
    await click(answerBtn());
    expect(onPick.mock.calls[0]!.slice(0, 4)).toEqual(["dec1", "other", "rev1", TEXT]);
  });

  it("on an unbound card the answer is off with its reason, but Ask / note still sends", async () => {
    const { onNote } = mount(ask(OWN_OTHER), {}, true);
    type(TEXT);
    expect(answerBtn().hasAttribute("disabled")).toBe(true);
    expect(q("[data-other-answer-reason]")!.textContent).toMatch(/not bound/i);
    expect(noteBtn().hasAttribute("disabled")).toBe(false);
    await click(noteBtn());
    expect(onNote).toHaveBeenCalledWith("dec1", TEXT, undefined);
  });
});

describe("Ask / note", () => {
  it("is enabled with text typed on an ask with its own other option, sends once, and clears the box", async () => {
    const { onNote, onPick } = mount(ask(OWN_OTHER));
    type(TEXT);
    await click(noteBtn());
    expect(onNote).toHaveBeenCalledTimes(1);
    expect(onNote).toHaveBeenCalledWith("dec1", TEXT, undefined);
    expect(onPick).not.toHaveBeenCalled(); // a note never rules
    expect(q("[data-other-result='ok']")!.textContent).toBe("Sent. The owner is woken; nothing was ruled or closed.");
    expect(q<HTMLTextAreaElement>("[data-other-text]")!.value).toBe("");
    expect(noteBtn().hasAttribute("disabled")).toBe(true); // empty again, not stuck
  });

  it("shows a refusal, keeps the text and lets mk press it again", async () => {
    const { onNote } = mount(ask(OWN_OTHER), { onNote: () => ({ ok: false, error: "card is closed" }) });
    type(TEXT);
    await click(noteBtn());
    expect(q("[data-other-result='error']")!.textContent).toContain("card is closed");
    expect(q<HTMLTextAreaElement>("[data-other-text]")!.value).toBe(TEXT);
    expect(noteBtn().hasAttribute("disabled")).toBe(false);
    await click(noteBtn());
    expect(onNote).toHaveBeenCalledTimes(2);
  });

  it("is enabled again after a send that throws", async () => {
    mount(ask(OWN_OTHER), { onNote: () => { throw new Error("boom"); } });
    type(TEXT);
    await click(noteBtn());
    expect(q("[data-other-result='error']")!.textContent).toContain("boom");
    expect(noteBtn().hasAttribute("disabled")).toBe(false);
    expect(answerBtn().hasAttribute("disabled")).toBe(false);
  });

  it("does not stay disabled behind a send that never settles", async () => {
    vi.useFakeTimers();
    mount(ask(OWN_OTHER), { onNote: () => new Promise(() => {}) });
    type(TEXT);
    await click(noteBtn());
    expect(noteBtn().hasAttribute("disabled")).toBe(true);
    expect(q("[data-other-wait]")!.textContent).toMatch(/sending/i);
    await advance(SEND_TIMEOUT_MS + 1);
    expect(noteBtn().hasAttribute("disabled")).toBe(false);
    expect(answerBtn().hasAttribute("disabled")).toBe(false);
    expect(q("[data-other-wait]")).toBeNull();
    expect(q("[data-other-result='error']")!.textContent).toMatch(/may still go through/i);
    expect(q<HTMLTextAreaElement>("[data-other-text]")!.value).toBe(TEXT);
  });

  it("a late send success does not clear the box while an image read begun after the send is still pending", async () => {
    vi.useFakeTimers();
    let land!: (r: { ok: boolean }) => void;
    const { onNote } = mount(ask(OWN_OTHER), { onNote: () => new Promise((res) => { land = res; }) });
    type(TEXT);
    await click(noteBtn());
    await advance(SEND_TIMEOUT_MS + 1); // the send timed out, so the box is usable again
    const file = new File(["x"], "shot.png", { type: "image/png" });
    Object.defineProperty(file, "arrayBuffer", { value: () => new Promise(() => {}) });
    const paste = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(paste, "clipboardData", { value: { files: [file] } });
    await act(async () => { q("[data-other-text]")!.dispatchEvent(paste); });
    await act(async () => { land({ ok: true }); });
    expect(onNote).toHaveBeenCalledTimes(1);
    expect(q<HTMLTextAreaElement>("[data-other-text]")!.value).toBe(TEXT);
    expect(q("[data-other-result='ok']")!.textContent).toContain("Your newer draft was kept.");
  });

  // The late-success check reads the box text through a ref that follows the typed text; these two pin what it must decide.
  it("a late send success clears the box when the text is still what was sent", async () => {
    vi.useFakeTimers();
    let land!: (r: { ok: boolean }) => void;
    mount(ask(OWN_OTHER), { onNote: () => new Promise((res) => { land = res; }) });
    type(TEXT);
    await click(noteBtn());
    await advance(SEND_TIMEOUT_MS + 1);
    await act(async () => { land({ ok: true }); });
    expect(q<HTMLTextAreaElement>("[data-other-text]")!.value).toBe("");
    expect(q("[data-other-result='ok']")!.textContent).not.toContain("newer draft");
  });

  it("a late send success does not clear the box when the text was edited after the send", async () => {
    vi.useFakeTimers();
    let land!: (r: { ok: boolean }) => void;
    mount(ask(OWN_OTHER), { onNote: () => new Promise((res) => { land = res; }) });
    type(TEXT);
    await click(noteBtn());
    await advance(SEND_TIMEOUT_MS + 1);
    type(TEXT + " and more");
    await act(async () => { land({ ok: true }); });
    expect(q<HTMLTextAreaElement>("[data-other-text]")!.value).toBe(TEXT + " and more");
    expect(q("[data-other-result='ok']")!.textContent).toContain("Your newer draft was kept.");
  });

  it("does not stay disabled behind an image read that never settles", async () => {
    vi.useFakeTimers();
    const { onNote } = mount(ask(OWN_OTHER));
    type(TEXT);
    const file = new File(["x"], "shot.png", { type: "image/png" });
    Object.defineProperty(file, "arrayBuffer", { value: () => new Promise(() => {}) });
    const paste = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(paste, "clipboardData", { value: { files: [file] } });
    await act(async () => { q("[data-other-text]")!.dispatchEvent(paste); });
    expect(noteBtn().hasAttribute("disabled")).toBe(true);
    expect(q("[data-other-wait]")!.textContent).toMatch(/reading an image/i);
    await advance(IMG_READ_TIMEOUT_MS + 1);
    expect(noteBtn().hasAttribute("disabled")).toBe(false);
    expect(q("[data-other-image-error]")!.textContent).toMatch(/too long to read/i);
    await click(noteBtn());
    expect(onNote).toHaveBeenCalledWith("dec1", TEXT, undefined); // the skipped image is not attached
  });
});
