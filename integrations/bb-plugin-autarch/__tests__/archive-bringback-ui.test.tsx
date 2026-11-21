// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ArchiveRow } from "../archive.js";
import { ArchiveRowView } from "../ui/archive.js";

vi.mock("../components/ui/icon.js", () => ({ Icon: () => null }));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const row: ArchiveRow = {
  task_id: "fixture", card_key: "fixture-card", title: "Saved question", project: "Example", state: "closed",
  decided_at: "2026-10-01T00:00:00Z", pick_label: null, option_id: null, by: null, reason: null,
  generation: 1, no_pick: true, decision_id: "decision-fixture",
};
let host: HTMLDivElement;
let root: Root;
beforeEach(() => {
  host = document.createElement("div"); document.body.appendChild(host); root = createRoot(host);
});
afterEach(() => { act(() => root.unmount()); host.remove(); vi.restoreAllMocks(); });

function mount(over: Partial<ArchiveRow> = {}, busy = false) {
  const current = { ...row, ...over }; const onBringBack = vi.fn();
  act(() => root.render(<ArchiveRowView row={current} onBringBack={onBringBack} busy={busy} />));
  return { current, onBringBack, button: host.querySelector<HTMLButtonElement>("button[data-archive-bringback]") };
}

describe("Archive Bring back eligibility (mk-bzi8s.14)", () => {
  it("shows a disabled button and explains a missing saved question", () => {
    const { button, onBringBack } = mount({ decision_id: null, generation: null });
    expect(button).not.toBeNull();
    expect(button!.textContent).toBe("Bring back");
    expect(button!.disabled).toBe(false);
    expect(button!.tabIndex).toBe(0);
    expect(button!.getAttribute("aria-disabled")).toBe("true");
    act(() => button!.focus()); expect(document.activeElement).toBe(button);
    expect(host.querySelector("[data-archive-bringback-reason]")?.textContent).toBe("No saved question to reopen.");
    act(() => button!.click()); expect(onBringBack).not.toHaveBeenCalled();
  });
  it("links the disabled button to its reason and removes the link when the action becomes available", () => {
    const unavailable = mount({ decision_id: null, generation: null });
    const reasonId = unavailable.button!.getAttribute("aria-describedby");
    expect(reasonId).toBeTruthy();
    expect(document.getElementById(reasonId!)?.textContent).toBe("No saved question to reopen.");
    expect(host.querySelector("[data-archive-bringback-reason]")?.id).toBe(reasonId);
    act(() => unavailable.button!.focus()); expect(document.activeElement).toBe(unavailable.button);
    const available = mount();
    expect(available.button!.disabled).toBe(false);
    expect(available.button!.hasAttribute("aria-describedby")).toBe(false);
    expect(available.button!.hasAttribute("aria-disabled")).toBe(false);
    expect(document.getElementById(reasonId!)).toBeNull();
  });
  it.each(["ruled", "open", "observed", "display"])("does not submit a no-pick row in state %s", state => {
    const { button, onBringBack } = mount({ state });
    expect(button).not.toBeNull(); expect(button!.disabled).toBe(false);
    expect(button!.getAttribute("aria-disabled")).toBe("true");
    act(() => button!.focus()); expect(document.activeElement).toBe(button);
    act(() => button?.click()); expect(onBringBack).not.toHaveBeenCalled();
  });
  it("submits the closed no-pick row from the actual button", () => {
    const { button, current, onBringBack } = mount();
    expect(button).not.toBeNull(); expect(button!.disabled).toBe(false); expect(button!.textContent).toBe("Bring back");
    act(() => button!.click());
    expect(onBringBack).not.toHaveBeenCalled();
    act(() => document.querySelector<HTMLButtonElement>("[data-archive-confirm]")!.click());
    expect(onBringBack).toHaveBeenCalledExactlyOnceWith(current);
  });
  it("keeps picked rows available and disables them while busy", () => {
    const picked = { state: "ruled", no_pick: false, option_id: "a", by: "mk" };
    const { button, current, onBringBack } = mount(picked);
    expect(button!.disabled).toBe(false);
    act(() => button!.click());
    expect(onBringBack).not.toHaveBeenCalled();
    act(() => document.querySelector<HTMLButtonElement>("[data-archive-confirm]")!.click());
    expect(onBringBack).toHaveBeenCalledExactlyOnceWith(current);
    const busy = mount(picked, true); expect(busy.button!.disabled).toBe(false);
    expect(busy.button!.getAttribute("aria-disabled")).toBe("true");
    act(() => busy.button!.focus()); expect(document.activeElement).toBe(busy.button);
    act(() => busy.button!.click()); expect(busy.onBringBack).not.toHaveBeenCalled();
  });
});

it("opens an in-UI dialog, cancels without sending, and confirms only once while busy", () => {
  const { button, onBringBack, current } = mount({ no_pick: false, state: "ruled", option_id: "a", by: "mk" });
  act(() => { button!.click(); button!.click(); });
  expect(onBringBack).not.toHaveBeenCalled();
  expect(document.querySelector('[role="dialog"]')).not.toBeNull();
  act(() => document.querySelector<HTMLButtonElement>("[data-archive-cancel]")!.click());
  expect(onBringBack).not.toHaveBeenCalled();
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  act(() => button!.click());
  const confirm = document.querySelector<HTMLButtonElement>("[data-archive-confirm]")!;
  expect(confirm.textContent).toBe(`Bring back ${row.card_key}: ${row.title}`);
  act(() => { confirm.click(); confirm.click(); });
  expect(onBringBack).toHaveBeenCalledExactlyOnceWith(current);
  // Reopen a fresh dialog after a retry, then make that live dialog busy.
  act(() => root.render(<ArchiveRowView row={current} onBringBack={onBringBack} error="retry" />));
  act(() => button!.click());
  act(() => root.render(<ArchiveRowView row={current} onBringBack={onBringBack} busy />));
  const liveConfirm = document.querySelector<HTMLButtonElement>("[data-archive-confirm]")!;
  expect(liveConfirm).not.toBeNull(); expect(liveConfirm.isConnected).toBe(true);
  expect(liveConfirm.disabled).toBe(true);
  act(() => liveConfirm.click());
  expect(onBringBack).toHaveBeenCalledTimes(1);
});

it.each(["", "x".repeat(200)])("uses a bounded nonempty title in the confirmation: %s", title => {
  const { button } = mount({ title });
  act(() => button!.click());
  expect(document.querySelector("[data-archive-confirm]")!.textContent).toBe(`Bring back ${row.card_key}: ${title ? title.slice(0, 120) : "Untitled card"}`);
});

it("falls back to the saved decision subject when the card title is empty", () => {
  const { button } = mount({ title: "", subject: "Saved subject" });
  act(() => button!.click());
  expect(document.querySelector("[data-archive-confirm]")!.textContent).toBe(`Bring back ${row.card_key}: Saved subject`);
});
