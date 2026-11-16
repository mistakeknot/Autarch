// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ArchiveRow } from "../archive.js";
import { ArchiveRowView } from "../ui/archive.js";

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
afterEach(() => { act(() => root.unmount()); host.remove(); });

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
    expect(button!.disabled).toBe(true);
    expect(host.querySelector("[data-archive-bringback-reason]")?.textContent).toBe("No saved question to reopen.");
    act(() => button!.click()); expect(onBringBack).not.toHaveBeenCalled();
  });
  it.each(["ruled", "open", "observed", "display"])("does not submit a no-pick row in state %s", state => {
    const { button, onBringBack } = mount({ state });
    expect(button === null || button.disabled).toBe(true);
    act(() => button?.click()); expect(onBringBack).not.toHaveBeenCalled();
  });
  it("submits the closed no-pick row from the actual button", () => {
    const { button, current, onBringBack } = mount();
    expect(button).not.toBeNull(); expect(button!.disabled).toBe(false); expect(button!.textContent).toBe("Bring back");
    act(() => button!.click()); expect(onBringBack).toHaveBeenCalledExactlyOnceWith(current);
  });
  it("keeps picked rows available and disables them while busy", () => {
    const picked = { state: "ruled", no_pick: false, option_id: "a", by: "mk" };
    const { button, current, onBringBack } = mount(picked);
    expect(button!.disabled).toBe(false);
    act(() => button!.click()); expect(onBringBack).toHaveBeenCalledExactlyOnceWith(current);
    const busy = mount(picked, true); expect(busy.button!.disabled).toBe(true);
    act(() => busy.button!.click()); expect(busy.onBringBack).not.toHaveBeenCalled();
  });
});
