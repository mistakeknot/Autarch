// Citadel UX audit, Batch A (bead mk-utw91.1): the buttons that did nothing and the failures nobody saw.
//  - Copy command / Copy status command on a Blocked row were drawn only when a handler was passed, and none was.
//  - Revoke in the card-link view had no handler (the list swallowed it with an empty function).
//  - Nine RPC calls ended in `() => {}`, so a refused or failed click looked like one that worked.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { ActionError, actionErrorText, runAction } from "../ui/actionerror.js";
import { focusedAsks } from "../ui/cardlink.js";
import { ApprovalsList, AsksPanel, type ApprovalRecord, type AsksData } from "../ui/asks.js";
import { BlocksRow, type QueueRowView } from "../ui/blocks.js";
import type { RootRunPanelView } from "../ui/rootrun.js";
import { rpcOutcome } from "../movehandlers.js";
import { RootRunSection } from "../ui/rootrun.js";
import type { ReactElement, ReactNode } from "react";

const NOW = Date.parse("2026-10-01T00:00:00.000Z");

const blockedRow = (): QueueRowView => ({
  id: "d1", decision_id: "d1", task_id: "t1", card_key: "k1", binding_state: "confirmed", project: "Autarch", title: "Run the script?",
  refs: [], blocks_count: 1, created_at: "2026-09-28T00:00:00.000Z", thread: "thr-a", pinned: false,
  ask: { question: "Run it?", options: [{ id: "a", label: "A" }, { id: "b", label: "B" }] } as never, revision: "rv1", mentions: 0,
  display_reason: null, display_only: false, overrides_generation: null, changed_after_ruling: false, root: { state: "verified", reason: null },
});
const rootView: RootRunPanelView = {
  present: true, badge: "run by paste, not authenticated", state: "match", problems: [], tuple: { script: "/opt/run.sh", sha256: "a".repeat(64), timeout: 60, set: "s1" },
  actual_sha256: null, owner_thread: "thr_a", item_json: null, command: "todo-add --set 's1' --from-card 'card-t1.json'", card_file: "card-t1.json", status: { kind: "none" },
  status_command: "root-run-status t1",
};

// RootRunSection has no hooks, so calling it returns its element tree and a button's onClick can be run directly.
function buttonsOf(node: ReactNode, out: { text: string; onClick?: () => void }[] = []): { text: string; onClick?: () => void }[] {
  if (Array.isArray(node)) { for (const n of node) buttonsOf(n, out); return out; }
  if (node === null || typeof node !== "object") return out;
  const el = node as ReactElement<{ children?: ReactNode; onClick?: () => void }>;
  if (el.type === "button") out.push({ text: String(([] as ReactNode[]).concat(el.props.children ?? "").join("")), ...(el.props.onClick ? { onClick: el.props.onClick } : {}) });
  return buttonsOf(el.props?.children, out);
}

describe("root-run Copy buttons on a Blocked row", () => {
  it("clicking each button hands the exact command to the copy handler, and the row's handler copies it", () => {
    const copied: string[] = [];
    const tree = RootRunSection({ view: rootView, onCopy: (t) => copied.push(t) });
    const buttons = buttonsOf(tree);
    buttons.find((b) => b.text === "Copy command")!.onClick!();
    buttons.find((b) => b.text === "Copy status command")!.onClick!();
    expect(copied).toEqual([rootView.command, rootView.status_command]);
    const src = readFileSync(join(__dirname, "..", "ui", "blocks.tsx"), "utf8");
    expect(src).toMatch(/onCopy=\{\(text\) => void copyText\(text\)\.then\(/);
  });
  it("draws Copy command and Copy status command once the row's root-run view has loaded", () => {
    const html = renderToStaticMarkup(<BlocksRow row={blockedRow()} nowMs={NOW} onPick={() => {}} onOpen={() => {}} rootRun={{ views: { t1: rootView }, load: () => {} }} />);
    expect(html).toContain(">Copy command</button>");
    expect(html).toContain(">Copy status command</button>");
  });
});

const approval: ApprovalRecord = { approval_id: "a1", decision_id: "d1", kind: "merge", target: "repo#9", identity: "abc", minted_at: "x", expires_at: "2026-10-08T10:00:00.000Z" };
const asksWithApproval = (): AsksData => ({ owed: [], runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], approvals: [approval], delegation: { settings: {}, suspended: false }, machineOwners: {} }) as AsksData;

describe("Revoke is never a button that does nothing", () => {
  it("is not drawn when the list has no handler", () => {
    const html = renderToStaticMarkup(<ApprovalsList approvals={[approval]} nowMs={NOW} />);
    expect(html).toContain('data-approval-id="a1"');
    expect(html).not.toContain(">Revoke</button>");
  });
  it("is drawn when a handler is given, in the list and through the asks panel", () => {
    expect(renderToStaticMarkup(<ApprovalsList approvals={[approval]} onRevoke={() => {}} nowMs={NOW} />)).toContain(">Revoke</button>");
    expect(renderToStaticMarkup(<AsksPanel data={asksWithApproval()} onPick={async () => ({ ok: true })} onOpen={() => {}} onRevoke={() => {}} nowMs={NOW} />)).toContain(">Revoke</button>");
    expect(renderToStaticMarkup(<AsksPanel data={asksWithApproval()} onPick={async () => ({ ok: true })} onOpen={() => {}} nowMs={NOW} />)).not.toContain(">Revoke</button>");
  });
  it("is reachable in the card-link view: the linked card's own approval survives focusedAsks", () => {
    const card = { id: "d1", project: "p", thread: "t", subject: "s", key: "DEMO-1", asker: "card", filed_at: "2026-10-21T00:00:00Z", revision: "r", ask: { question: "q", options: [] } } as never;
    const data = focusedAsks({ ...asksWithApproval(), approvals: [approval, { ...approval, approval_id: "a9", decision_id: "other" }] }, card);
    const html = renderToStaticMarkup(<AsksPanel data={data} onPick={async () => ({ ok: true })} onOpen={() => {}} onRevoke={() => {}} nowMs={NOW} />);
    expect(html).toContain('data-approval-id="a1"');
    expect(html).toContain(">Revoke</button>");
    expect(html).not.toContain('data-approval-id="a9"');
  });
  it("is wired in every AsksPanel the page renders, the card-link view included", () => {
    const src = readFileSync(join(__dirname, "..", "app.tsx"), "utf8");
    const panels = src.split("<AsksPanel").slice(1);
    expect(panels.length).toBeGreaterThanOrEqual(2);
    for (const p of panels) expect(p.slice(0, p.indexOf("/>"))).toContain("onRevoke={revoke}");
    expect(src).toContain('rpc.call("revokeApproval", { approval_id })');
  });
});

describe("a failed click now shows", () => {
  it("runAction sets the line for a refusal, a missing reason and a thrown error, and always refetches", async () => {
    for (const [send, want] of [
      [Promise.resolve({ ok: false, reason: "unknown" }), "Revoking the approval did not go through: unknown"],
      [Promise.resolve({ ok: false, error: "only the vizier thread may bind a project" }), "Revoking the approval did not go through: only the vizier thread may bind a project"],
      [Promise.reject(new Error("network down")), "Revoking the approval did not go through: network down"],
    ] as const) {
      const set = vi.fn();
      const after = vi.fn();
      await runAction("Revoking the approval", send, set, after);
      expect(set).toHaveBeenLastCalledWith(want);
      expect(after).toHaveBeenCalledTimes(1);
    }
  });
  it("runAction clears the line after a success", async () => {
    const set = vi.fn();
    await runAction("Dismissing it", Promise.resolve({ ok: true }), set);
    expect(set).toHaveBeenLastCalledWith(null);
  });
  it("rpcOutcome reads the service's `reason` when there is no `error`", async () => {
    expect(await rpcOutcome(Promise.resolve({ ok: false, reason: "unknown" }))).toEqual({ ok: false, error: "unknown" });
    expect(actionErrorText("X", undefined)).toBe("X did not go through: no reason was given");
  });
  it("the line is a role=alert paragraph, and nothing when there is no failure", () => {
    const html = renderToStaticMarkup(<ActionError message="Dismissing it did not go through: unknown" />);
    expect(html).toContain('role="alert"');
    expect(html).toContain("data-action-error");
    expect(html).toContain("Dismissing it did not go through: unknown");
    expect(renderToStaticMarkup(<ActionError message={null} />)).toBe("");
  });
  it("none of the nine calls ends in an empty error handler any more", () => {
    const src = readFileSync(join(__dirname, "..", "app.tsx"), "utf8");
    for (const name of ["dismiss", "revokeApproval", "setBinding", "override", "markSeen", "learnSet", "updateInfo"]) {
      const lines = src.split("\n").filter((l) => l.includes(`"${name}"`));
      expect(lines.length).toBeGreaterThan(0);
      for (const l of lines) expect(l, name).not.toMatch(/\(\) => \{\}\)/);
    }
  });
});
