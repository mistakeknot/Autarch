import type { ReactElement, ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({}));

import { ActionButton, buttonClass } from "../ui/buttons.js";
import { CommandBlock, copyText, legacyCopyViaDocument, type CopyEnv } from "../ui/copy.js";
import { OtherBox, otherText, OTHER_MAX } from "../ui/other.js";
import { MoveCard, moveButtons, YourMovePanel, type MoveHandlers } from "../ui/yourmove.js";
import { AskCard, AsksPanel, buildAsksView, isDestructiveOption, PickController, UNBOUND_EXPLANATION, type AsksData, type OwedAsk } from "../ui/asks.js";
import { BlocksPanel, type QueueRowView, type QueueView } from "../ui/blocks.js";
import { scriptCommands } from "../moves.js";
import type { MoveView, MoveViewGroups } from "../moveview.js";

const SHA = "a".repeat(64);
const decode = (s: string) => s.replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&quot;/g, '"').replace(/&#x27;/g, "'").replace(/&amp;/g, "&");

const move = (o: Partial<MoveView> = {}): MoveView => ({
  task_id: "t1", generation: 1, kind: "script", state: "open", title: "Run the cutover", owner: "thr-a", opened_at: "2026-10-06T10:00:00.000Z",
  claimed_at: null, skipped_at: null, checked_at: null, report_deadline_at: null, url: null, need: null,
  script: { path: "/opt/run.sh", sha256: SHA },
  commands: scriptCommands({ path: "/opt/run.sh", sha256: SHA, args: ["--set", "s1"], recover: { path: "/opt/undo.sh", sha256: "b".repeat(64) } }),
  report: null, closed_at: null, closed_by: null, evidence: null, ...o,
});
const noop = async () => ({ ok: true });
const handlers = (o: Partial<MoveHandlers> = {}): MoveHandlers => ({ onCheck: noop, onClaim: noop, onSkip: noop, onNote: noop, ...o });
const groups = (o: Partial<MoveViewGroups> = {}): MoveViewGroups => ({ yourMove: [], reported: [], later: [], hidden: [], audit: [], ...o });
const card = (m: MoveView, section: "yourMove" | "reported" | "later" | "hidden" = "yourMove") => renderToStaticMarkup(<MoveCard m={m} h={handlers()} section={section} />);
const labels = (html: string) => [...html.matchAll(/data-move-action="[a-z]+"[^>]*>([^<]*)</g)].map((x) => decode(x[1]!));

describe("Your move buttons say what mk says about himself", () => {
  it("script: I ran --check, I ran it, Later / skip, and never Done", () => {
    expect(moveButtons(move()).map((b) => b.label)).toEqual(["I ran --check", "I ran it", "Later / skip"]);
    expect(labels(card(move()))).toEqual(["I ran --check", "I ran it", "Later / skip"]);
    expect(card(move())).not.toMatch(/>\s*Done\s*</);
  });
  it("after I ran it: waiting for the script's report, no claim buttons", () => {
    const html = card(move({ state: "claimed", claimed_at: "2026-10-06T11:00:00.000Z" }), "reported");
    expect(html).toContain("Ran: waiting for the script&#x27;s report");
    expect(labels(html)).toEqual([]);
  });
  it("pr: I'll merge it, never Merged; read: I read it", () => {
    const pr = card(move({ kind: "pr", script: null, commands: [], url: "https://github.com/o/r/pull/1" }));
    expect(labels(pr)).toEqual(["I&#x27;ll merge it".replace("&#x27;", "'"), "Later / skip"]);
    expect(pr).not.toContain("Merged");
    expect(card(move({ kind: "pr", script: null, commands: [], claimed_at: "2026-10-06T11:00:00Z", state: "claimed" }), "reported")).toContain("Waiting for GitHub to show it merged");
    expect(labels(card(move({ kind: "read", script: null, commands: [], url: "https://x.test/doc" })))).toEqual(["I read it", "Later / skip"]);
  });
  it("context: shows the needed text", () => {
    const html = card(move({ kind: "context", script: null, commands: [], need: "Which account owns the domain?" }));
    expect(html).toContain("Needed from you: Which account owns the domain?");
  });
  it("a check is recorded without claiming: the card stays in Your move and says so", () => {
    const html = card(move({ checked_at: "2026-10-06T10:30:00.000Z" }));
    expect(html).toContain("does not start the report clock");
    expect(html).not.toContain("Ran: waiting");
  });
});

describe("report states", () => {
  const claimed = { state: "claimed" as const, claimed_at: "2026-10-06T11:00:00.000Z" };
  it("success shows the link and time", () => {
    const html = card(move({ ...claimed, report: { outcome: "succeeded", failing_step: null, error_line: null, report_link: "/threads/r.txt", reported_at: "2026-10-06T11:05:00.000Z", deadline_at: null } }), "reported");
    expect(html).toContain("Report: succeeded");
    expect(html).toContain("/threads/r.txt");
    expect(html).toContain("2026-10-06 11:05 UTC");
  });
  it("failure shows the step, the error and the link, and says the card stays open", () => {
    const html = card(move({ ...claimed, report: { outcome: "failed", failing_step: "migrate", error_line: "ERROR: disk full", report_link: "/threads/r.txt", reported_at: "2026-10-06T11:05:00.000Z", deadline_at: null } }), "reported");
    expect(html).toContain("Failing step: migrate");
    expect(html).toContain("Error: ERROR: disk full");
    expect(html).toContain("/threads/r.txt");
    expect(html).toContain("The card stays open");
    expect(html).toContain('role="alert"');
  });
  it("no report by the deadline says No report received", () => {
    const html = card(move({ ...claimed, report: { outcome: "no-report", failing_step: null, error_line: null, report_link: null, reported_at: null, deadline_at: "2026-10-06T12:00:00.000Z" } }), "reported");
    expect(html).toContain("No report received");
  });
});

describe("plain text only", () => {
  it("titles, needs, report text and urls are escaped, never HTML", () => {
    const evil = `<img src=x onerror=alert(1)> & "q"`;
    const html = card(
      move({ title: evil, kind: "context", script: null, commands: [], need: evil, state: "claimed", claimed_at: "2026-10-06T11:00:00Z", report: { outcome: "failed", failing_step: evil, error_line: evil, report_link: evil, reported_at: null, deadline_at: null } }),
      "reported",
    );
    expect(html).not.toContain("<img");
    expect(html).toContain("&lt;img src=x onerror=alert(1)&gt;");
    expect(html).not.toContain("dangerouslySetInnerHTML");
  });
});

describe("panel groups", () => {
  it("renders Your move, Reported, Later, Hidden and a collapsed audit list, in that order", () => {
    const html = renderToStaticMarkup(
      <YourMovePanel
        handlers={handlers()}
        data={groups({
          yourMove: [move({ task_id: "a" })],
          reported: [move({ task_id: "b", state: "claimed", claimed_at: "2026-10-06T11:00:00Z" })],
          later: [move({ task_id: "c", skipped_at: "2026-10-06T11:00:00Z" })],
          hidden: [move({ task_id: "d" })],
          audit: [{ task_id: "e", generation: 1, kind: "script", closed_at: "2026-10-06T12:00:00Z", closed_by: "report", evidence: "RESULT: OK" }],
        })}
      />,
    );
    const at = (k: string) => html.indexOf(`data-section="${k}"`);
    expect(at("move-yourMove")).toBeLessThan(at("move-reported"));
    expect(at("move-reported")).toBeLessThan(at("move-later"));
    expect(at("move-later")).toBeLessThan(at("move-hidden"));
    expect(at("move-hidden")).toBeLessThan(at("move-audit"));
    expect(html).toContain("Reported done, not verified (1)");
    expect(html).toMatch(/<details[^>]*data-section="move-audit"/);
    expect(html).not.toMatch(/<details[^>]*open/);
    expect(html).toContain("e (script) closed 2026-10-06 12:00 UTC by report");
    // hidden cards offer no buttons
    expect(html.match(/data-section-of="hidden"/g)?.length).toBe(1);
  });
  it("renders nothing when there are no moves", () => {
    expect(renderToStaticMarkup(<YourMovePanel handlers={handlers()} data={groups()} />)).toBe("");
  });
  it("shows commands in run order with the expected sha, one block each", () => {
    const html = card(move());
    const order = ["Check the file (sha256sum)", "Dry run (--check)", "The real run", "Recovery"].map((l) => html.indexOf(l));
    expect(order.every((i) => i >= 0)).toBe(true);
    expect([...order].sort((a, b) => a - b)).toEqual(order);
    expect(html.match(/data-command-box/g)?.length).toBe(4);
    expect(html).toContain(`expected sha256 ${SHA}`);
    expect(html).toContain(`expected sha256 ${"b".repeat(64)}`);
  });
});

describe("copyable command blocks", () => {
  const cmds = scriptCommands({ path: "/opt/run.sh", sha256: SHA, args: ["--set", "s1", "--note", "it is fine"], recover: null });
  const run = cmds.find((c) => c.label === "run")!.command;

  it("the block's text equals the command, and the command is one clean line", () => {
    for (const c of cmds) {
      const html = renderToStaticMarkup(<CommandBlock label={c.label} command={c.command} expectedSha={c.expectedSha} />);
      const text = decode(/<code data-command-text="true">([\s\S]*?)<\/code>/.exec(html)![1]!);
      expect(text).toBe(c.command);
      expect(c.command).not.toMatch(/[\n\r`]/);
      expect(c.command).toBe(c.command.trim());
      expect(c.command).not.toMatch(/^[$#>] /);
      expect(html).toContain("overflow-x:auto");
      expect(html).toContain("white-space:pre");
      // the label and the sha sit outside the command element
      expect(/<code[^>]*>([\s\S]*?)<\/code>/.exec(html)![1]).not.toContain("expected");
    }
  });
  it("renders a git step as runuser -u mk -- git -C", () => {
    expect(renderToStaticMarkup(<CommandBlock label="git" command={"runuser -u mk -- git -C '/r' 'status'"} />)).toContain("runuser -u mk -- git -C &#x27;/r&#x27; &#x27;status&#x27;");
  });
  it("copies the filed command byte for byte with a fake clipboard", async () => {
    let got: string | null = null;
    const env: CopyEnv = { clipboard: { writeText: async (t) => void (got = t) }, legacyCopy: () => { throw new Error("must not fall back"); } };
    expect(await copyText(run, env)).toBe(true);
    expect(got).toBe(run);
  });
  it("falls back to select-all and execCommand when the clipboard is missing or refuses", async () => {
    let legacy: string | null = null;
    const legacyCopy = (t: string) => ((legacy = t), true);
    expect(await copyText(run, { legacyCopy })).toBe(true);
    expect(legacy).toBe(run);
    legacy = null;
    expect(await copyText(run, { clipboard: { writeText: async () => { throw new Error("denied"); } }, legacyCopy })).toBe(true);
    expect(legacy).toBe(run);
    expect(await copyText(run, {})).toBe(false);
  });
  it("the real fallback selects the whole text in a textarea and copies it, then cleans up", () => {
    const calls: string[] = [];
    let value = "";
    const ta: Record<string, unknown> = {
      style: {},
      setAttribute: () => {},
      focus: () => calls.push("focus"),
      select: () => calls.push("select"),
      setSelectionRange: (a: number, b: number) => calls.push(`range ${a}-${b}`),
      set value(v: string) { value = v; },
      get value() { return value; },
    };
    const doc = {
      createElement: () => ta,
      body: { appendChild: () => calls.push("append"), removeChild: () => calls.push("remove") },
      execCommand: (c: string) => (calls.push(`exec ${c} with ${value === run}`), true),
    } as unknown as Document;
    expect(legacyCopyViaDocument(doc)(run)).toBe(true);
    expect(calls).toEqual(["append", "focus", "select", `range 0-${run.length}`, "exec copy with true", "remove"]);
  });
  it("the Copy button is wired and labelled", () => {
    const html = renderToStaticMarkup(<CommandBlock label="run" command={run} />);
    expect(html).toContain("data-copy-button");
    expect(html).toContain("Copy run command");
    expect(html).toContain('data-copy-state="idle"');
  });
});

describe("button look", () => {
  it("has an outline, hover, focus-visible, a distinct selected state and a distinct destructive tone", () => {
    const d = buttonClass("default");
    expect(d).toMatch(/\bborder\b/);
    expect(d).toContain("hover:");
    expect(d).toContain("focus-visible:");
    expect(buttonClass("default", true)).not.toBe(d);
    expect(buttonClass("destructive")).not.toBe(d);
    expect(buttonClass("recommended")).not.toBe(d);
    expect(renderToStaticMarkup(<ActionButton selected>x</ActionButton>)).toContain('aria-pressed="true"');
  });
  it("marks the recommended option and styles an irreversible turn-it-down option as destructive; reversibility wording holds", () => {
    const a: OwedAsk = {
      id: "d", project: "P", thread: "thr", subject: "S", asker: "x", filed_at: "2026-10-01T00:00:00Z", revision: "r",
      ask: { question: "Q", recommendation: "go", options: [
        { id: "go", label: "Go", kind: "instruction", reversible: true, instruction: "Do it." },
        { id: "no", label: "Cancel the rollout", kind: "ruling-only", reversible: false },
        { id: "wait", label: "Wait", kind: "ruling-only", reversible: true },
      ] },
    };
    const html = renderToStaticMarkup(<AskCard ask={a} onPick={() => {}} onOpen={() => {}} />);
    expect(html).toContain("data-recommended-mark");
    expect(html).toMatch(/data-tone="recommended"[^>]*data-option="go"|data-option="go"[^>]*data-tone="recommended"/);
    expect(html).toMatch(/data-tone="destructive"[^>]*data-option="no"|data-option="no"[^>]*data-tone="destructive"/);
    // a ruling-only option sends nothing, so none of the three says it cannot be undone
    expect(html.match(/Cannot be undone\./g)?.length).toBeUndefined();
    expect(isDestructiveOption({ label: "Cancel it", reversible: true })).toBe(false);
  });
});

describe("Other box", () => {
  it("trims, and refuses empty or over 2000", () => {
    expect(otherText("  hi  ")).toBe("hi");
    expect(otherText("   ")).toBeNull();
    expect(otherText("x".repeat(OTHER_MAX))).toBe("x".repeat(OTHER_MAX));
    expect(otherText("x".repeat(OTHER_MAX + 1))).toBeNull();
  });
  it("counts Unicode code points like the service: 1000 emoji fit, 2001 do not", () => {
    expect(otherText("\u{1F600}".repeat(OTHER_MAX))).not.toBeNull();
    expect(otherText("\u{1F600}".repeat(OTHER_MAX + 1))).toBeNull();
    expect(renderToStaticMarkup(<OtherBox onNote={() => {}} onAnswer={() => {}} />)).toContain("0 / 2000");
  });
  it("hides Answer with this on an ask that has its own option named other", () => {
    const a: OwedAsk = { id: "d", project: "P", thread: "thr", subject: "S", asker: "x", filed_at: "2026-10-01T00:00:00Z", revision: "r", ask: { question: "Q", options: [{ id: "other", label: "Else", kind: "ruling-only", reversible: true }] } };
    const html = renderToStaticMarkup(<AskCard ask={a} onPick={() => {}} onOpen={() => {}} onNote={() => {}} />);
    expect(html).toContain("data-other-answer-reason");
    expect(html).toMatch(/disabled=""[^>]*data-other-answer|data-other-answer[^>]*disabled=""/);
  });
  it("starts with both buttons disabled and a 0 / 2000 counter", () => {
    const html = renderToStaticMarkup(<OtherBox onNote={() => {}} onAnswer={() => {}} />);
    expect(html).toContain("0 / 2000");
    expect(html).toMatch(/disabled=""[^>]*data-other-note/);
    expect(html).toMatch(/disabled=""[^>]*data-other-answer/);
    expect(html).toContain("Ask / note");
    expect(html).toContain("Answer with this");
  });
  it("omits Answer with this when there is no answer handler and explains a disabled one", () => {
    expect(renderToStaticMarkup(<OtherBox onNote={() => {}} />)).not.toContain("Answer with this");
    expect(renderToStaticMarkup(<OtherBox onNote={() => {}} answerDisabledReason="not bound" />)).toContain("not bound");
  });
  it("every move card carries the box with Ask / note only (a move is not a ruling)", () => {
    const html = card(move());
    expect(html).toContain("data-other");
    expect(html).not.toContain("Answer with this");
  });
});

const base: AsksData = { owed: [], runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {} };

describe("Stalled Dismiss", () => {
  const walk = (n: ReactNode, f: (e: ReactElement<Record<string, unknown>>) => void): void => {
    if (Array.isArray(n)) return n.forEach((x) => walk(x, f));
    if (n === null || typeof n !== "object") return;
    const e = n as ReactElement<Record<string, unknown>>;
    f(e);
    if (typeof e.type === "function" && e.type !== ActionButton) walk((e.type as (p: unknown) => ReactNode)(e.props), f);
    else walk(e.props?.children as ReactNode, f);
  };
  it("an undeliverable row carries its target and a Dismiss button that calls the dismiss handler", () => {
    const d = { ...base, undeliverable: [{ id: "o1", decision_id: "dec1", kind: "ruling-wake", recipient: "thr-a" }] };
    expect(buildAsksView(d)[0]!.items[0]!.dismiss).toEqual({ decision_id: "dec1", obligation_id: "o1" });
    const onDismiss = vi.fn();
    const found: ReactElement<Record<string, unknown>>[] = [];
    walk(AsksPanel({ data: d, onPick: () => {}, onOpen: () => {}, onDismiss }), (e) => {
      if (e.props && "data-dismiss" in e.props) found.push(e);
    });
    expect(found).toHaveLength(1);
    (found[0]!.props.onClick as () => void)();
    expect(onDismiss).toHaveBeenCalledWith("dec1", "o1");
    expect(renderToStaticMarkup(<AsksPanel data={d} onPick={() => {}} onOpen={() => {}} onDismiss={onDismiss} />)).toContain("Dismiss");
  });
  it("no Dismiss on rows with no obligation to dismiss", () => {
    const d = { ...base, asks: [{ id: "m", subject: "x", thread: "t", owner: null, detail: "d", updated_at: "x" }], uncertain: [{ id: "u", kind: "wake", recipient: "t" }] };
    expect(renderToStaticMarkup(<AsksPanel data={d} onPick={() => {}} onOpen={() => {}} onDismiss={() => {}} />)).not.toContain("Dismiss");
  });
});

describe("unbound projects", () => {
  const row = (o: Partial<QueueRowView> = {}): QueueRowView => ({
    id: "d1", decision_id: "d1", task_id: "t1", card_key: "k1", binding_state: "confirmed", project: "Autarch", title: "Which order?",
    refs: [], blocks_count: 1, created_at: "2026-09-28T00:00:00.000Z", thread: "thr-a", pinned: false,
    ask: { question: "Which order?", options: [{ id: "a", label: "A", kind: "ruling-only" }] }, revision: "rv1", mentions: 0,
    display_reason: null, display_only: false, overrides_generation: null, changed_after_ruling: false, root: { state: "verified", reason: null }, ...o,
  });
  const q = (rows: QueueRowView[]): QueueView => ({ rows, legacy: { count: 0, owed: [], runbook: [], machine: { lane: [], asks: [] } }, bindings: [], inactive_projects: [] });
  const html = (rows: QueueRowView[]) => renderToStaticMarkup(<BlocksPanel data={q(rows)} nowMs={Date.parse("2026-10-01T00:00:00Z")} onPick={() => {}} onOpen={() => {}} onNote={() => {}} />);

  it("a card without a verified root goes to Unbound projects with an explanation and no pick buttons", () => {
    const h = html([row({ id: "ok" }), row({ id: "un", root: { state: "unverified", reason: "no root" } })]);
    const unbound = h.slice(h.indexOf('data-section="unbound-projects"'));
    expect(unbound).toContain('data-row="un"');
    expect(unbound).not.toContain('data-row="ok"');
    expect(unbound).not.toContain("data-option=");
    expect(unbound).toContain("cannot record a ruling");
    expect(decode(unbound)).toContain(UNBOUND_EXPLANATION);
    // Answer with this is shown disabled, Ask / note still works
    expect(unbound).toMatch(/disabled=""[^>]*data-other-answer/);
    expect(unbound).toContain("data-other-answer-reason");
    // the bound card keeps its pick buttons
    expect(h.slice(h.indexOf('data-row="ok"'), h.indexOf('data-section="unbound-projects"'))).toContain("data-option=");
  });
});

describe("pick controller with the Other text", () => {
  it("keys the pick id on the text, so different words are different picks and a retry keeps its id", async () => {
    let n = 0;
    const ids: string[] = [];
    const c = new PickController(() => `id${++n}`);
    const rpc = async (r: { pick_id: string }) => (ids.push(r.pick_id), { ok: false as const });
    await c.send(rpc, { decision_id: "d", option_id: "other", revision: "r", reason: "one" }, () => {});
    await c.send(rpc, { decision_id: "d", option_id: "other", revision: "r", reason: "one" }, () => {});
    await c.send(rpc, { decision_id: "d", option_id: "other", revision: "r", reason: "two" }, () => {});
    expect(ids).toEqual(["id1", "id1", "id2"]);
  });
});
