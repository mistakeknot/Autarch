// Each destructive action hands ConfirmTap the right inputs and, once confirmed, calls the right handler. The
// lifecycle itself is in confirm-tap-lifecycle.test.tsx. Bead mk-qrp5d.
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

type Seen = { needsConfirm: boolean; resetKey?: string; onConfirmed: () => void };
const seen = vi.hoisted(() => [] as Seen[]);
vi.mock("../ui/confirmtap.js", async (orig) => {
  const actual = await orig<typeof import("../ui/confirmtap.js")>();
  return {
    ...actual,
    ConfirmTap: (p: Seen & { children: (c: { armed: boolean; onClick: () => void; onBlur: () => void }) => unknown }) => {
      seen.push({ needsConfirm: p.needsConfirm, ...(p.resetKey !== undefined ? { resetKey: p.resetKey } : {}), onConfirmed: p.onConfirmed });
      return p.children({ armed: false, onClick: () => {}, onBlur: () => {} }) as never;
    },
  };
});

import { ApprovalsList, AskCard } from "../ui/asks.js";
import { BindingsPanel } from "../ui/settings.js";

beforeEach(() => { seen.length = 0; });

describe("the call sites of ConfirmTap", () => {
  it("Revoke always confirms, resets per approval, and revokes that approval", () => {
    const revoke = vi.fn();
    renderToStaticMarkup(<ApprovalsList approvals={[{ approval_id: "a1", decision_id: "d1", kind: "merge", target: "repo#9", identity: "abc", minted_at: "x", expires_at: "2026-10-08T10:00:00.000Z" }]} onRevoke={revoke} nowMs={Date.parse("2026-10-01T00:00:00.000Z")} />);
    expect(seen).toHaveLength(1);
    expect(seen[0]).toMatchObject({ needsConfirm: true, resetKey: "a1" });
    expect(revoke).not.toHaveBeenCalled();
    seen[0]!.onConfirmed();
    expect(revoke).toHaveBeenCalledWith("a1");
  });

  it("binding Reject always confirms and rejects that binding; Confirm is not wrapped", () => {
    const bind = vi.fn();
    renderToStaticMarkup(<BindingsPanel bindings={[{ tasks_project_id: "tp1", home_project: "Autarch", state: "suggested", suggested_at: null, confirmed_at: null }]} inactive={[]} legacyCount={0} onBind={bind} />);
    expect(seen).toHaveLength(1);
    expect(seen[0]).toMatchObject({ needsConfirm: true, resetKey: "tp1|suggested" });
    seen[0]!.onConfirmed();
    expect(bind).toHaveBeenCalledWith({ tasks_project_id: "tp1", state: "rejected" });
  });

  it("an AskCard confirms only its destructive options and resets per card revision", () => {
    renderToStaticMarkup(
      <AskCard ask={{ id: "dec1", project: "Autarch", thread: "t", subject: "s", asker: "thread", filed_at: "2026-09-29T10:00:00Z", revision: "rev1", mentions: 0, ask: { question: "?", options: [{ id: "no", label: "Reject and close the PR", kind: "ruling-only", reversible: false }, { id: "yes", label: "Merge it", kind: "ruling-only", reversible: true }] } }} onPick={(() => {}) as never} onOpen={() => {}} />,
    );
    expect(seen.map((s) => s.needsConfirm)).toEqual([true, false]);
    expect(seen.every((s) => s.resetKey === "dec1|rev1")).toBe(true);
  });
});
