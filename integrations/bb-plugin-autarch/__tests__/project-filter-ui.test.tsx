// @vitest-environment jsdom
// The project filter combobox, driven the way mk drives it (bead mk-sufr9): open it, tick projects out and in, search,
// use the keyboard, reset. Also the hook that keeps the choice on the device, and the places that show the result.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { ProjectFilterBar, useProjectFilter } from "../ui/projectfilter.js";
import { PROJECT_FILTER_KEY } from "../ui/projectfilterstore.js";
import { MobileInbox } from "../ui/inbox.js";
import { AsksPanel, type AsksData, type OwedAsk } from "../ui/asks.js";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const CHOICES = [{ name: "Autarch", count: 4 }, { name: "Clavain", count: 2 }, { name: "Yaoivers", count: 1 }];

let host: HTMLDivElement;
let root: Root;
beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  localStorage.clear();
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

const q = <T extends Element = HTMLElement>(sel: string) => host.querySelector<T>(sel);
const qa = (sel: string) => Array.from(host.querySelectorAll<HTMLElement>(sel));
const click = (el: Element | null) => act(() => { (el as HTMLElement).click(); });
const key = (el: Element | null, k: string) => act(() => { el!.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true })); });
const type = (el: HTMLInputElement, v: string) => act(() => {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(el, v);
  el.dispatchEvent(new Event("input", { bubbles: true }));
});

function mount(excluded: string[] = [], hidden = 0, choices = CHOICES) {
  const onChange = vi.fn();
  const render = (ex: string[], h = hidden) => act(() => root.render(<ProjectFilterBar choices={choices} excluded={new Set(ex)} hidden={h} onChange={onChange} />));
  render(excluded);
  return { onChange, render };
}
const trigger = () => q("[data-project-filter-trigger]");
const options = () => qa("[role=option]");
const opened = () => click(trigger());

describe("the closed combobox", () => {
  it("reads All projects when nothing is excluded, and as a collapsed popup button", () => {
    mount();
    expect(trigger()!.textContent).toContain("All projects");
    expect(trigger()!.getAttribute("aria-expanded")).toBe("false");
    expect(trigger()!.getAttribute("aria-haspopup")).toBe("listbox");
    expect(options()).toHaveLength(0);
  });
  it("says how many projects are ticked in when some are out", () => {
    mount(["Clavain"]);
    expect(trigger()!.textContent).toContain("2 of 3 projects");
  });
  it("has a button at least 44px tall on touch", () => {
    mount();
    expect(trigger()!.className).toContain("min-h-11");
  });
  it("is not drawn when no card has a project", () => {
    mount([], 0, []);
    expect(q("[data-project-filter]")).toBeNull();
  });
});

describe("the open list", () => {
  it("lists every project with its card count, all ticked, each row 44px tall", () => {
    mount();
    opened();
    expect(trigger()!.getAttribute("aria-expanded")).toBe("true");
    expect(options().map((o) => o.textContent)).toEqual(expect.arrayContaining([expect.stringContaining("Autarch"), expect.stringContaining("Clavain"), expect.stringContaining("Yaoivers")]));
    expect(options().map((o) => o.getAttribute("aria-selected"))).toEqual(["true", "true", "true"]);
    expect(options()[0]!.textContent).toContain("4");
    for (const o of options()) expect(o.className).toContain("min-h-11");
    expect(q("[role=listbox]")!.getAttribute("aria-multiselectable")).toBe("true");
  });
  it("shows an excluded project unticked", () => {
    mount(["Clavain"]);
    opened();
    expect(options().map((o) => o.getAttribute("aria-selected"))).toEqual(["true", "false", "true"]);
  });
  it("ticks a project out and in with a click, handing the whole new set to onChange", () => {
    const { onChange, render } = mount();
    opened();
    click(options()[1]!);
    expect([...onChange.mock.calls[0]![0]]).toEqual(["Clavain"]);
    render(["Clavain"]);
    click(options()[1]!);
    expect(onChange.mock.calls[1]![0].size).toBe(0);
  });
  it("stays open while mk ticks several", () => {
    mount();
    opened();
    click(options()[0]!);
    expect(options()).toHaveLength(3);
  });
  it("narrows the list as mk types, without regard to case, and says so when nothing matches", () => {
    mount();
    opened();
    const input = q<HTMLInputElement>("input[role=combobox]")!;
    type(input, "CLA");
    expect(options().map((o) => o.textContent)).toEqual([expect.stringContaining("Clavain")]);
    type(input, "zzz");
    expect(options()).toHaveLength(0);
    expect(q("[data-project-filter-nomatch]")!.textContent).toContain("No project matches");
  });
  it("closes on Escape and gives focus back to the button", () => {
    mount();
    opened();
    key(q("input[role=combobox]"), "Escape");
    expect(options()).toHaveLength(0);
    expect(document.activeElement).toBe(trigger());
  });
  it("closes when mk clicks outside it", () => {
    mount();
    opened();
    act(() => { document.body.dispatchEvent(new MouseEvent("mousedown", { bubbles: true })); });
    expect(options()).toHaveLength(0);
  });
});

describe("the keyboard", () => {
  it("moves through the list with the arrow keys and ticks with Enter", () => {
    const { onChange } = mount();
    opened();
    const input = q<HTMLInputElement>("input[role=combobox]")!;
    expect(document.activeElement).toBe(input);
    key(input, "ArrowDown");
    key(input, "ArrowDown");
    expect(input.getAttribute("aria-activedescendant")).toBe(options()[1]!.id);
    key(input, "Enter");
    expect([...onChange.mock.calls[0]![0]]).toEqual(["Clavain"]);
  });
  it("wraps, and Home and End jump to the ends", () => {
    mount();
    opened();
    const input = q<HTMLInputElement>("input[role=combobox]")!;
    key(input, "End");
    expect(input.getAttribute("aria-activedescendant")).toBe(options()[2]!.id);
    key(input, "ArrowDown");
    expect(input.getAttribute("aria-activedescendant")).toBe(options()[0]!.id);
    key(input, "ArrowUp");
    expect(input.getAttribute("aria-activedescendant")).toBe(options()[2]!.id);
    key(input, "Home");
    expect(input.getAttribute("aria-activedescendant")).toBe(options()[0]!.id);
  });
  it("opens from the button with the arrow key", () => {
    mount();
    key(trigger(), "ArrowDown");
    expect(options()).toHaveLength(3);
  });
  it("keeps the active row inside the narrowed list", () => {
    const { onChange } = mount();
    opened();
    const input = q<HTMLInputElement>("input[role=combobox]")!;
    key(input, "End");
    type(input, "aut");
    key(input, "Enter");
    expect([...onChange.mock.calls[0]![0]]).toEqual(["Autarch"]);
  });
});

describe("the reset and the hidden note", () => {
  it("offers All projects in the list only while something is excluded, and it clears the set", () => {
    const { onChange } = mount();
    opened();
    expect(q("[data-project-filter-all]")).toBeNull();
    act(() => root.unmount());
    root = createRoot(host);
    const again = mount(["Clavain", "Yaoivers"]);
    opened();
    click(q("[data-project-filter-all]"));
    expect(again.onChange.mock.calls[0]![0].size).toBe(0);
    expect(onChange).not.toHaveBeenCalled();
  });
  it("says plainly how many cards the filter hides, with the reset beside it", () => {
    const { onChange } = mount(["Autarch"], 3);
    const note = q("[data-project-filter-hidden]")!;
    expect(note.textContent).toContain("3 hidden by the project filter");
    const reset = note.querySelector<HTMLButtonElement>("button")!;
    expect(reset.textContent).toContain("All projects");
    expect(reset.className).toContain("min-h-11");
    click(reset);
    expect(onChange.mock.calls[0]![0].size).toBe(0);
  });
  it("says nothing when nothing is hidden", () => {
    mount(["Autarch"], 0);
    expect(q("[data-project-filter-hidden]")).toBeNull();
  });
  it("announces the hidden count to a screen reader", () => {
    mount(["Autarch"], 2);
    expect(q("[data-project-filter-hidden]")!.getAttribute("role")).toBe("status");
  });
});

describe("a long project name", () => {
  it("wraps in the list instead of widening the page", () => {
    mount([], 0, [{ name: "P".repeat(120), count: 1 }]);
    opened();
    expect(options()[0]!.innerHTML).toContain("[overflow-wrap:anywhere]");
  });
});

function Probe({ onState }: { onState: (s: ReturnType<typeof useProjectFilter>) => void }) {
  onState(useProjectFilter());
  return null;
}
describe("useProjectFilter", () => {
  it("starts with nothing excluded", () => {
    let s!: ReturnType<typeof useProjectFilter>;
    act(() => root.render(<Probe onState={(x) => { s = x; }} />));
    expect(s.excluded.size).toBe(0);
  });
  it("keeps the choice across a reload on this device", () => {
    let s!: ReturnType<typeof useProjectFilter>;
    act(() => root.render(<Probe onState={(x) => { s = x; }} />));
    act(() => s.setExcluded(new Set(["Clavain"])));
    expect([...s.excluded]).toEqual(["Clavain"]);
    expect(localStorage.getItem(PROJECT_FILTER_KEY)).toContain("Clavain");
    act(() => root.unmount());
    root = createRoot(host);
    act(() => root.render(<Probe onState={(x) => { s = x; }} />));
    expect([...s.excluded]).toEqual(["Clavain"]);
  });
  it("forgets the choice on reset", () => {
    let s!: ReturnType<typeof useProjectFilter>;
    act(() => root.render(<Probe onState={(x) => { s = x; }} />));
    act(() => s.setExcluded(new Set(["Clavain"])));
    act(() => s.setExcluded(new Set()));
    expect(localStorage.getItem(PROJECT_FILTER_KEY)).toBeNull();
  });
  it("still works when storage is blocked", () => {
    const spy = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("blocked"); });
    let s!: ReturnType<typeof useProjectFilter>;
    act(() => root.render(<Probe onState={(x) => { s = x; }} />));
    act(() => s.setExcluded(new Set(["A"])));
    expect([...s.excluded]).toEqual(["A"]);
    spy.mockRestore();
  });
});

const owed = (id: string, project: string, over: Partial<OwedAsk> = {}): OwedAsk => ({
  id, project, thread: "t", subject: `Subject ${id}`, asker: "thread", filed_at: "2026-11-01T00:00:00Z", revision: "r", task_id: `task-${id}`,
  ask: { question: "q", options: [{ id: "a", label: "A", kind: "ruling-only" }] }, ...over,
});
const data = (o: OwedAsk[]): AsksData => ({ owed: o, runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {} });
const noop = () => {};

describe("where the result shows", () => {
  it("the phone inbox draws the filter under the header, on the list only, with counts it was given", () => {
    const html = renderToStaticMarkup(
      <MobileInbox asks={data([owed("1", "A")])} waiting={5} onPick={noop as never} onOpen={noop} onNote={noop} onLater={noop} sinceNode={null} sinceCount={0} onDesktop={noop}
        filter={{ node: (hidden) => <p data-test-filter>{`hidden ${hidden}`}</p>, hidden: { waiting: 2, later: 7 }, laterCount: 9 }} />,
    );
    expect(html).toContain("data-test-filter");
    expect(html).toContain("hidden 2");
    expect(html).toContain("Later 9");
  });
  it("the phone inbox names a list the filter emptied, instead of saying nothing is waiting", () => {
    const html = renderToStaticMarkup(
      <MobileInbox asks={data([])} waiting={5} onPick={noop as never} onOpen={noop} onNote={noop} onLater={noop} sinceNode={null} sinceCount={0} onDesktop={noop}
        filter={{ node: () => null, hidden: { waiting: 2, later: 0 }, laterCount: 0 }} />,
    );
    expect(html).toContain("Everything here is hidden by the project filter.");
    expect(html).not.toContain("Nothing is waiting on you.");
  });
  it("the phone inbox keeps its own words when the filter hides nothing", () => {
    const html = renderToStaticMarkup(
      <MobileInbox asks={data([])} waiting={0} onPick={noop as never} onOpen={noop} onNote={noop} onLater={noop} sinceNode={null} sinceCount={0} onDesktop={noop}
        filter={{ node: () => null, hidden: { waiting: 0, later: 0 }, laterCount: 0 }} />,
    );
    expect(html).toContain("Nothing is waiting on you.");
  });
  it("the asks panel uses the empty text it is given", () => {
    const html = renderToStaticMarkup(<AsksPanel data={data([])} part="decide" emptyText="Everything here is hidden by the project filter." onPick={noop as never} onOpen={noop} />);
    expect(html).toContain("Everything here is hidden by the project filter.");
    expect(html).not.toContain("Nothing needs you.");
  });
  it("the asks panel keeps its own words without one", () => {
    expect(renderToStaticMarkup(<AsksPanel data={data([])} part="decide" onPick={noop as never} onOpen={noop} />)).toContain("Nothing needs you.");
    expect(renderToStaticMarkup(<AsksPanel data={data([])} part="later" onPick={noop as never} onOpen={noop} />)).toContain("Nothing is set aside.");
  });
});
