// @vitest-environment jsdom
// The phone's "More waiting" row (bead mk-j5ipa.12): one closed row at the bottom of the phone inbox that, opened, lists
// Stalled, Steps for you, Waiting on others and Recorded approvals with their counts, read-only. Before it, the phone had
// no way to reach any of these four lists; the desktop showed them.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { MobileInbox } from "../ui/inbox.js";
import type { AsksData, OwedAsk } from "../ui/asks.js";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const NOW = Date.parse("2026-10-09T12:00:00Z");
const noop = () => {};
const ask = (id: string, project = "Clavain", over: Partial<OwedAsk> = {}): OwedAsk => ({
  id,
  project,
  thread: "thr-a",
  subject: `Card ${id}`,
  asker: "thread",
  filed_at: "2026-10-09T07:00:00Z",
  revision: "r1",
  task_id: `t-${id}`,
  ask: { question: "Pick one.", recommendation: "a", options: [{ id: "a", label: "A", kind: "ruling-only" }, { id: "b", label: "B", kind: "ruling-only" }] },
  ...over,
});
const base = (over: Partial<AsksData> = {}): AsksData => ({ owed: [ask("d1")], runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {}, ...over });

/** One item in each of the four lists, five in all: two stalled (an undeliverable wake and a stalled ask). */
const full = (): AsksData =>
  base({
    undeliverable: [{ id: "u1", kind: "wake", recipient: "thr-x", last_error: "thread archived", decision_id: "d9" }],
    asks: [{ id: "s1", subject: "Deploy stuck", thread: "thr-s", owner: "thr-o", detail: "no progress for 2 d", updated_at: "2026-10-07T00:00:00Z", label: "stalled" }],
    runbook: [{ thread: "thr-r", items: [{ id: "r1", subject: "Run the install script", question: "Run it as root.", steps: ["bash /x"], filed_at: "2026-10-09T01:00:00Z" }] }],
    lane: [{ id: "l1", subject: "Review of PR 12", thread: "thr-l", owner: "thr-reviewer", detail: "review running", updated_at: "2026-10-09T10:00:00Z" }],
    approvals: [{ approval_id: "ap1", decision_id: "d8", kind: "merge", target: "org/repo#12", identity: "abc1234", minted_at: "2026-10-09T10:00:00Z", expires_at: "2026-10-10T10:00:00Z" }],
  });

const phone = (a: AsksData, initial?: { moreOpen?: boolean; holdOpen?: boolean; tab?: "waiting" | "later" | "since"; open?: string }, extra: Record<string, unknown> = {}) =>
  renderToStaticMarkup(<MobileInbox asks={a} waiting={1} onPick={noop as never} onOpen={noop} onNote={noop} onLater={noop} sinceNode={<p>since</p>} sinceCount={0} onDesktop={noop} now={NOW} {...(initial ? { initial } : {})} {...extra} />);

describe("the phone can reach the four lists", () => {
  it("(was red) a phone inbox with items in every list shows a More waiting row with the total", () => {
    expect(phone(full())).toMatch(/More waiting · 5/);
  });
});

