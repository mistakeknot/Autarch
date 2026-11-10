// @vitest-environment jsdom
// The project filter and the next card after an answer (beads mk-sufr9 and mk-c0mka). The lists get their cards already
// filtered, so the card chosen after an answer is a neighbour in the list mk sees: a card of an unticked project is skipped,
// never opened, and never picked as the "card below" that is not on screen.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { DecideQueue, type AsksData, type OwedAsk } from "../ui/asks.js";
import { MobileInbox } from "../ui/inbox.js";
import { filterAsks } from "../ui/projectfilterstore.js";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const NOW = Date.parse("2026-11-09T12:00:00Z");
const ask = (id: string, project: string): OwedAsk => ({
  id, project, thread: `thr-${id}`, subject: `Subject ${id}`, asker: "thread", filed_at: "2026-11-09T07:00:00Z", revision: `rev-${id}`, task_id: `task-${id}`,
  ask: { question: `Question ${id}`, recommendation: "a", options: [{ id: "a", label: "Answer A", kind: "ruling-only", reversible: true }, { id: "b", label: "Answer B", kind: "ruling-only", reversible: true }] },
});
const data = (owed: OwedAsk[]): AsksData => ({ owed, runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {} });
const OUT = new Set(["Hidden"]);

let host: HTMLDivElement;
let root: Root;
beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView = () => {};
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
  delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView;
});

const ok = () => ({ ok: true });
const settle = () => act(async () => { await Promise.resolve(); });
const q = (sel: string) => host.querySelector(sel) as HTMLElement | null;
const click = (el: Element | null) => { expect(el).not.toBeNull(); act(() => { (el as HTMLElement).click(); }); };

describe("desktop Decide list with a project ticked out", () => {
  const props = { onPick: vi.fn(ok) as never, onOpen: () => {}, nowMs: NOW, onLater: vi.fn(ok) as never };
  const showing = () => q("[data-decide-list] [aria-current=true]")?.textContent ?? "";
  const render = (all: OwedAsk[]) => act(() => root.render(<DecideQueue owed={filterAsks(data(all), OUT).owed} {...props} />));
  const A = ask("a", "Shown"), B = ask("b", "Hidden"), C = ask("c", "Shown"), D = ask("d", "Hidden"), E = ask("e", "Shown");

  it("answering a card shows the next visible card, skipping the hidden one between them", () => {
    render([A, B, C]);
    expect(q("[data-decide-list]")?.textContent).not.toContain("Subject b");
    click(q("[data-decide-list] li:nth-child(1) button")); // select A
    expect(showing()).toContain("Subject a");
    render([B, C]); // A is answered; B is hidden by the filter
    expect(showing()).toContain("Subject c");
    expect(q("[data-decision]")?.getAttribute("data-decision")).toBe("c");
  });

  it("answering the last visible card shows the visible card above it, not a hidden one", () => {
    render([A, B, C, D]);
    click(q("[data-decide-list] li:nth-child(2) button")); // select C, the last visible
    expect(showing()).toContain("Subject c");
    render([A, B, D]); // C answered; D below is hidden, B above is hidden
    expect(showing()).toContain("Subject a");
  });

  it("answering the only visible card leaves nothing on screen, never a hidden card", () => {
    render([A, B]);
    render([B]);
    expect(host.querySelector("[data-decision]")).toBeNull();
    expect(q("[data-decide-list]")?.textContent ?? "").not.toContain("Subject b");
  });

  it("unticking the project of the card on screen moves on to the next visible card", () => {
    const rerender = (excluded: ReadonlySet<string>) => act(() => root.render(<DecideQueue owed={filterAsks(data([A, B, C, E]), excluded).owed} {...props} />));
    rerender(new Set());
    click(q("[data-decide-list] li:nth-child(3) button")); // select C
    expect(showing()).toContain("Subject c");
    rerender(new Set(["Shown"])); // the project on screen is ticked out: only B remains
    expect(host.querySelector("[data-decision]")?.getAttribute("data-decision")).toBe("b");
  });
});

describe("phone inbox with a project ticked out", () => {
  const A = ask("a", "Shown"), B = ask("b", "Hidden"), C = ask("c", "Shown"), D = ask("d", "Hidden");
  const onPick = vi.fn(ok) as never;
  const view = (all: OwedAsk[], open?: string) => {
    const shown = filterAsks(data(all), OUT);
    return <MobileInbox asks={shown} waiting={shown.owed.length} onPick={onPick} onOpen={() => {}} onNote={() => {}} onLater={vi.fn(ok) as never} sinceNode={null} sinceCount={0} onDesktop={() => {}} now={NOW} {...(open ? { initial: { open } } : {})} />;
  };
  const render = (all: OwedAsk[], open?: string) => act(() => root.render(view(all, open)));
  const detail = () => q("[data-inbox-detail]")?.getAttribute("data-inbox-detail") ?? null;

  it("the sticky bar's answer opens the next visible card, skipping the hidden one", async () => {
    render([A, B, C], "ask:a");
    click(q("[data-bar-main]"));
    await settle();
    expect(detail()).toBe("ask:c");
  });

  it("answering the last visible card opens the visible card above it, not a hidden one", async () => {
    render([A, B, C, D], "ask:c");
    click(q("[data-bar-main]"));
    await settle();
    expect(detail()).toBe("ask:a");
  });

  it("an answer pressed on the card itself, once the card leaves the list, opens the next visible card", async () => {
    render([A, B, C], "ask:a");
    click(q('[data-inbox-detail] [data-option="b"]'));
    await settle();
    render([B, C]);
    expect(detail()).toBe("ask:c");
  });

  it("answering the only visible card goes to the list, which names what the filter hides", async () => {
    render([A, B], "ask:a");
    click(q("[data-bar-main]"));
    await settle();
    render([B]);
    expect(detail()).toBeNull();
    expect(q('[data-inbox-row="ask:b"]')).toBeNull();
  });
});
