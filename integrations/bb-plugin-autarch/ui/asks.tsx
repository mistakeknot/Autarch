// The Asks view: what is stalled, what mk must decide, the runbook, and what is merely waiting.

import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import { ActionButton, RecommendedMark } from "./buttons.js";
import { CONFIRM_LABEL, ConfirmTap } from "./confirmtap.js";
import { CardConversation } from "./conversation.js";
import { clickStarted, paintDone } from "./clicktiming.js";
import { splitCommands, type CommandRole } from "./commandtext.js";
import { CommandBlock } from "./copy.js";
import { glance, GlanceBlock, type GlanceAnswer } from "./glance.js";
import { GH_REF_SRC, refNodes, URL_SRC } from "./links.js";
import { DocLink } from "./docreader.js";
import type { Question } from "../model.js";
import { freeOwnWordsId } from "../ownwords.js";
import { MultiPick } from "./multipick.js";
import { OtherBox, otherHelpText, type OtherImage, type OtherOutcome } from "./other.js";

/** What a pick hands back to the card: not ok means the card stays open and shows the error. */
export type PickOutcome = { ok: boolean; error?: string };
export type OnPick = (decisionId: string, optionId: string, revision: string, reason?: string, images?: OtherImage[], answers?: Record<string, string>) => void | Promise<PickOutcome | void>;

/** Turns a send into an outcome the card can show: a thrown error or a non-ok result is a failure. */
export async function pickOutcome(send: Promise<{ ok?: boolean; status?: number; error?: string }>): Promise<PickOutcome> {
  try {
    const r = await send;
    if (r?.ok === true) return { ok: true };
    return { ok: false, error: r?.error ?? (r?.status === 409 ? "this ask changed; reread and pick again" : `pick failed${r?.status ? ` (${r.status})` : ""}`) };
  } catch (e) {
    return { ok: false, error: `pick not sent: ${e instanceof Error ? e.message : String(e)}` };
  }
}

/** Turns a Later / Move back send into an outcome: a thrown error or a not-ok result is a failure. */
export async function laterOutcome(send: Promise<unknown>): Promise<PickOutcome> {
  try {
    const r = (await send) as { ok?: boolean; status?: number; error?: string } | null;
    if (r?.ok === true) return { ok: true };
    return { ok: false, error: r?.error ?? `failed${r?.status ? ` (${r.status})` : ""}` };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}

/** Runs a Later / Move back tap and reports its failure, if any, through `setError`; a handler that returns nothing is taken as done. */
export async function runLater(onLater: OnLater, taskId: string, on: boolean, setError: (error: string | null) => void): Promise<void> {
  setError(null);
  let result: unknown;
  try {
    result = await onLater(taskId, on);
  } catch (e) {
    setError(e instanceof Error ? e.message : String(e));
    return;
  }
  const r = result as { ok?: boolean; error?: string } | null | undefined;
  if (r && typeof r === "object" && r.ok === false) setError(r.error ?? "failed");
}

/** The short inline error under a Later or Move back tap that did not go through. */
export function LaterFailure({ error }: { error: string | null }) {
  if (error === null) return null;
  return <p role="alert" className="mt-2 w-full text-xs font-medium text-destructive" data-later-failure>{`The card did not move: ${error}. Try again.`}</p>;
}

export type ApprovalSpec = { kind: string; target: string; identity: string; ttl?: number };
export type Option = { id: string; label: string; kind: string; reversible?: boolean; instruction?: string; approval?: ApprovalSpec };
export type ApprovalRecord = { approval_id: string; decision_id: string; kind: string; target: string; identity: string; minted_at: string; expires_at: string };
export type OwedAsk = {
  id: string;
  project: string;
  thread: string;
  /** The asking thread's title when bb knows one; the card shows it in place of the id. */
  thread_title?: string | null;
  subject: string;
  /** The tasks key (PROJ-24); absent or null for an ask with no card. */
  key?: string | null;
  asker: string;
  filed_at: string;
  revision: string;
  mentions?: number;
  /** The tasks card behind this ask, when it is a card: the conversation reads its comments. */
  task_id?: string | null;
  /** Set while the vizier holds the card: shown greyed under "On hold", with no pick buttons. */
  held?: { reason: string; by: string; at: string } | null;
  /** Set while mk has set the card aside for later: it stays open and unruled, in the Later group, outside the Waiting count. */
  later?: { at: string; by: string } | null;
  ask: { question: string; recommendation?: string; options: Option[]; questions?: Question[] };
};
export type Lane = { id: string; subject: string; thread: string; owner: string | null; detail: string; updated_at: string; label?: string };
export type RunbookGroup = { thread: string; items: { id: string; subject: string; question: string; steps: string[]; filed_at: string }[] };
export type Obl = { id: string; decision_id?: string; kind?: string; recipient?: string; last_error?: string | null; state?: string };
export type Delegation = { settings: { vizierThreadId?: string; projects?: string[]; dailyCap?: number }; suspended: boolean };
export type AsksData = {
  owed: OwedAsk[];
  runbook: RunbookGroup[];
  lane: Lane[];
  asks: Lane[];
  undeliverable: Obl[];
  failures: Obl[];
  uncertain: Obl[];
  delegation: Delegation;
  approvals?: ApprovalRecord[];
  machineOwners: Record<string, string>;
};

export type ViewItem = { id: string; title: string; detail?: string; thread?: string; /** Set on an undeliverable wake: the Dismiss button's target. */ dismiss?: { decision_id: string; obligation_id: string } };
// operator-action items come first and stay first: Decide, Runbook, then Stalled, then Waiting.
export type Section = { key: "stalled" | "decide" | "runbook" | "waiting"; title: string; items: ViewItem[] };

export function buildAsksView(d: AsksData): Section[] {
  const stalled: ViewItem[] = [
    ...d.undeliverable.map((o) => ({
      id: `undeliverable:${o.id}`,
      title: `Undeliverable ${o.kind ?? "wake"} to ${o.recipient ?? "?"}`,
      detail: o.last_error ?? undefined,
      ...(o.decision_id ? { dismiss: { decision_id: o.decision_id, obligation_id: o.id } } : {}),
    })),
    ...d.uncertain.map((o) => ({ id: `uncertain:${o.id}`, title: `Uncertain ${o.kind ?? "wake"} to ${o.recipient ?? "?"}` })),
    ...d.failures.map((o) => ({ id: `failure:${o.id}`, title: `Ruling file failed for ${o.decision_id ?? o.id}`, detail: o.last_error ?? undefined })),
    ...d.asks.map((a) => ({ id: a.id, title: `${a.label ?? "stalled"}: ${a.subject}`, detail: a.detail, thread: a.thread })),
  ];
  const sections: Section[] = [
    { key: "decide", title: "Decide", items: d.owed.map((o) => ({ id: o.id, title: o.subject, thread: o.thread })) },
    { key: "runbook", title: "Runbook", items: d.runbook.flatMap((g) => g.items.map((i) => ({ id: i.id, title: i.subject, detail: i.question, thread: g.thread }))) },
    { key: "stalled", title: "Stalled", items: stalled },
    { key: "waiting", title: "Waiting", items: d.lane.map((l) => ({ id: l.id, title: l.subject, detail: `${l.detail} (owner ${l.owner})`, thread: l.thread })) },
  ];
  return sections.filter((s) => s.items.length > 0);
}

/** How long a recorded approval lasts, said before mk picks. ttl 0 or absent means the 24 h default. */
export function approvalExpiry(ttl: number | undefined): string {
  const s = ttl && ttl > 0 ? ttl : 86_400;
  const text = s > 86_400 && s % 86_400 === 0 ? `${s / 86_400} d` : s % 3600 === 0 ? `${s / 3600} h` : s % 60 === 0 ? `${s / 60} min` : `${s} s`;
  return `expires ${text} after you pick`;
}

/** "3 h", "4 d": how long an ask has waited. Unknown or future times read as "now". */
export function ageText(filedAt: string, nowMs: number): string {
  const ms = nowMs - Date.parse(filedAt);
  if (!(ms > 0)) return "now";
  const m = Math.floor(ms / 60_000);
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60);
  return h < 48 ? `${h} h` : `${Math.floor(h / 24)} d`;
}

/** An ask that has waited this long is probably settled elsewhere or superseded: Home marks it so mk can clear it. */
export const OLD_ASK_MS = 3 * 86_400_000;

/** An ask is old once it has waited more than three days; an unparseable or future time is not old. */
export function isOldAsk(filedAt: string, nowMs: number): boolean {
  return nowMs - Date.parse(filedAt) > OLD_ASK_MS;
}

/** The ask shown in full: the selected one while it is still owed, else the first. */
export function currentAsk<T extends { id: string }>(owed: T[], selected: string | null): T | undefined {
  return owed.find((o) => o.id === selected) ?? owed[0];
}

// Absolute paths and sha256 digests in an ask's text: shown as code, selectable in one click, and allowed to break
// anywhere so a long path never pushes the card wider than its panel. URLs and owner/repo#N are links (new tab).
const REF = new RegExp(`(${URL_SRC}|(?<![\\w/]|@[\\w.-]+:)\\/(?:[\\w.@+-]{1,255}\\/){1,64}[\\w@+-]{1,255}(?:\\.[\\w@+-]{1,255}){0,32}|\\b[0-9a-f]{64}\\b|${GH_REF_SRC})`, "g");

const ROLE_LABEL: Record<CommandRole, string> = { check: "Before it (dry run / check)", step: "Earlier step", final: "Final command", undo: "Undo (only if needed)" };

/** A card's question: prose as paragraphs, each command as a copyable block. The last one is the one to run. */
/** A question longer than this scrolls inside its own box, so eight thousand characters do not push the options off screen. */
export const LONG_QUESTION = 1500;
const LONG_BOX = "max-h-[28rem] overflow-y-auto rounded border border-border p-2";

export function QuestionText({ text }: { text: string }) {
  const parts = splitCommands(text);
  const scroll = text.length > LONG_QUESTION ? ` ${LONG_BOX}` : "";
  const mark = scroll === "" ? {} : { "data-long": "true" };
  if (!parts.some((p) => p.type === "command"))
    return <p className={`mt-2 whitespace-pre-wrap text-sm [overflow-wrap:anywhere]${scroll}`} {...mark}><RefText text={text} /></p>;
  const several = parts.filter((p) => p.type === "command").length > 1;
  return (
    <div className={`mt-2 space-y-3${scroll}`} data-question {...mark}>
      {parts.map((p, i) =>
        p.type === "prose" ? (
          <p key={i} className="m-0 whitespace-pre-wrap text-sm [overflow-wrap:anywhere]"><RefText text={p.text} /></p>
        ) : (
          <CommandBlock key={i} label={several ? ROLE_LABEL[p.role] : "Command"} command={p.text} primary={p.role === "final"} showFlags={p.role === "final"} />
        ),
      )}
    </div>
  );
}

const SHA_ONLY = /^[0-9a-f]{64}$/;
/** A digest belongs to a path only when it sits this close to it, in the text between them. */
const PAIR_GAP = 40;

/**
 * Pairs each sha256 with a path. Candidates are the path just before it and the path just after it (nothing else
 * referenced between), each within PAIR_GAP characters and not across a blank line. A candidate on the same line wins
 * over one on another line; otherwise the one before wins. So "a.md sha1; b.md sha2" and
 * "a.md\nsha256: sha1\nb.md\nsha256: sha2" give each document its own digest, "a.md\n\nsha2  b.md" gives b.md the
 * digest, and a digest with no such neighbour is used for none: no check is better than a check against the wrong
 * document.
 */
export function pairDigests(parts: readonly string[]): Map<number, string> {
  const out = new Map<number, string>();
  const isPath = (i: number) => i >= 0 && i < parts.length && i % 2 === 1 && parts[i].startsWith("/");
  const usable = (path: number, between: string) => isPath(path) && !out.has(path) && between.length <= PAIR_GAP && !/\n\s*\n/.test(between);
  parts.forEach((p, i) => {
    if (i % 2 !== 1 || !SHA_ONLY.test(p)) return;
    const before = usable(i - 2, parts[i - 1] ?? "") ? i - 2 : -1;
    const after = usable(i + 2, parts[i + 1] ?? "") ? i + 2 : -1;
    const sameLine = (between: string) => !between.includes("\n");
    const pick = before >= 0 && (after < 0 || sameLine(parts[i - 1]) || !sameLine(parts[i + 1])) ? before : after;
    if (pick >= 0) out.set(pick, p);
  });
  return out;
}

export function RefText({ text }: { text: string }) {
  const parts = text.split(REF);
  const digests = pairDigests(parts);
  return (
    <>
      {parts.map((part, i) => {
        if (i % 2 === 0) return part;
        const link = refNodes(part, String(i));
        if (link !== null) return <Fragment key={i}>{link}</Fragment>;
        return (
          <Fragment key={i}>
            <code className={`select-all rounded bg-muted px-1 font-mono text-xs ${SHA_ONLY.test(part) ? "inline-block max-w-full overflow-x-auto whitespace-nowrap align-bottom" : "[overflow-wrap:anywhere]"}`} data-ref>
              {part}
            </code>
            {part.startsWith("/") ? <DocLink path={part} sha256={digests.get(i)} /> : null}
          </Fragment>
        );
      })}
    </>
  );
}

/** Wording only: an irreversible option whose label says it throws something away or turns something down. */
const DESTRUCTIVE = /\b(abort|cancel|delete|discard|reject|drop|revert|wipe|destroy|remove|decline)\b/i;
export function isDestructiveOption(o: { label: string; reversible?: boolean }): boolean {
  return o.reversible !== true && DESTRUCTIVE.test(o.label);
}

export const UNBOUND_EXPLANATION = "This card's project is not bound to a Citadel project, so Citadel cannot record a ruling here. Bind the project, or use Ask / note to talk to the owner.";

/** Which asks can take a note from the Other box: the callback gets the decision id and mk's trimmed text. */
export type OnNote = (decisionId: string, text: string, images?: OtherImage[]) => Promise<OtherOutcome | void> | void;

/** Sets a card aside (on) or moves it back (off), by its task id. A thrown error or not-ok result is shown by the caller. */
export type OnLater = (taskId: string, on: boolean) => void | Promise<unknown>;

/** What a card calls its asking thread: the title when there is one, else the id. */
export const threadName = (a: { thread: string; thread_title?: string | null }): string => a.thread_title?.trim() || a.thread;

export function AskCard({ ask, onPick, onOpen, nowMs, onNote, onLater, unbound = false }: { ask: OwedAsk; onPick: OnPick; onOpen: (thread: string) => void; nowMs?: number; onNote?: OnNote; onLater?: OnLater; unbound?: boolean }) {
  const [failure, setFailure] = useState<string | null>(null);
  const [laterError, setLaterError] = useState<string | null>(null);
  const [pending, setPending] = useState<string | null>(null);
  const pick = async (optionId: string) => {
    setFailure(null);
    setPending(optionId);
    try {
      const r = await onPick(ask.id, optionId, ask.revision);
      if (r && r.ok === false) setFailure(r.error ?? "pick failed");
    } finally {
      setPending(null);
    }
  };
  const n = ask.mentions ?? 0;
  const hasRealOther = ask.ask.options.some((o) => o.id === "other");
  const anyInstruction = ask.ask.options.some((o) => o.kind === "instruction" && (o.instruction ?? "").trim() !== "");
  // A play or walk card gets its link, time, what is judged and what each answer does above the full text.
  const glanceOf = glance(ask);
  // Each answer shows in full what it sends; a ruling-only answer has no line.
  const glanceAnswers: GlanceAnswer[] = glanceOf
    ? ask.ask.options.map((o) => ({ id: o.id, label: o.label, recommended: ask.ask.recommendation === o.id, effect: optionEffect(o, threadName(ask), true) || null }))
    : [];
  return (
    <article className="min-w-0 rounded-lg border border-border bg-card p-4" data-decision={ask.id}>
      <h3 className="text-sm font-medium [overflow-wrap:anywhere]">
        {ask.key ? <span className="mr-2 select-all rounded bg-muted px-1.5 font-mono text-xs font-semibold" data-task-key>{ask.key}</span> : null}
        <RefText text={ask.subject} />
      </h3>
      <p className="mt-1 text-xs text-muted-foreground">
        {ask.project ? `${ask.project} · ` : ""}
        {nowMs !== undefined ? `waiting ${ageText(ask.filed_at, nowMs)} · ` : ""}
        <button type="button" className="inline-flex min-h-11 items-center underline sm:min-h-0" onClick={() => onOpen(ask.thread)}>{threadName(ask)}</button>
        {n > 0 ? ` - ${`also mentioned in ${n} ${n === 1 ? "thread" : "threads"}`}` : ""}
      </p>
      {glanceOf ? <GlanceBlock g={glanceOf} answers={glanceAnswers} /> : null}
      <QuestionText text={ask.ask.question} />
      {ask.ask.questions && ask.ask.questions.length > 0 ? (
        <MultiPick
          questions={ask.ask.questions}
          disabled={unbound}
          onAnswer={async (answers, note) => {
            const r = await onPick(ask.id, "answers", ask.revision, note, undefined, answers);
            return r && r.ok === false ? { ok: false, error: r.error ?? "pick failed" } : { ok: true };
          }}
        />
      ) : null}
      <ul className="mb-0 mt-3 list-none space-y-2 p-0">
        {ask.ask.options.map((o) => {
          const recommended = ask.ask.recommendation === o.id;
          return (
            <li key={o.id} className={`rounded border p-2 ${recommended ? "border-primary" : "border-border"}`} data-reversible={o.reversible === true ? "true" : "false"}>
              {o.approval ? (
                <p className="mb-1 text-xs" data-approval={o.approval.kind}>
                  {`Records your approval to ${o.approval.kind} ${o.approval.target} at ${o.approval.identity}; ${approvalExpiry(o.approval.ttl)}. This is a record only and does not authorize anything.`}
                </p>
              ) : null}
              <div className="flex flex-wrap items-center gap-2 text-sm">
                {unbound ? (
                  <span className="font-medium [overflow-wrap:anywhere]" data-option-label>{o.label}</span>
                ) : (
                  <ConfirmTap needsConfirm={isDestructiveOption(o)} onConfirmed={() => void pick(o.id)} resetKey={`${ask.id}|${ask.revision}`}>
                    {({ armed, onClick, onBlur }) => (
                      <ActionButton tone={isDestructiveOption(o) ? "destructive" : recommended ? "recommended" : "default"} selected={pending === o.id} disabled={pending !== null} onClick={onClick} onBlur={onBlur} data-option={o.id} {...(armed ? { "data-confirming": "true", "aria-label": `${CONFIRM_LABEL}: ${o.label}` } : {})}>{armed ? CONFIRM_LABEL : o.label}</ActionButton>
                    )}
                  </ConfirmTap>
                )}
                {recommended ? <RecommendedMark /> : null}
              </div>
              {optionEffect(o, threadName(ask)) !== "" ? <p className="mt-1 text-xs text-muted-foreground" data-effect>{optionEffect(o, threadName(ask))}</p> : null}
            </li>
          );
        })}
      </ul>
      {unbound ? <p role="note" className="mt-2 text-xs font-medium" data-unbound-note>{UNBOUND_EXPLANATION}</p> : null}
      {anyInstruction && !unbound ? (
        <p className="mt-2 text-xs text-muted-foreground">{`A "Tells ${threadName(ask)}" line is a preview of the first sentence; the full instruction is sent.`}</p>
      ) : null}
      {onLater && ask.task_id ? (
        <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <ActionButton tone="quiet" className="sm:!min-h-11" onClick={() => void runLater(onLater, ask.task_id!, true, setLaterError)} data-later-set={ask.id}>Later</ActionButton>
        </div>
      ) : null}
      <LaterFailure error={laterError} />
      {failure !== null ? <p role="alert" className="mt-2 text-xs font-medium text-destructive" data-pick-failure>{`Your pick did not go through: ${failure}. The ask is still open; try again.`}</p> : null}
      {onNote ? (
        <OtherBox
          onNote={(t, im) => onNote(ask.id, t, im)}
          onAnswer={unbound ? undefined : async (t, im) => {
            // An ask with its own option named "other" keeps it as a normal option; mk's words are recorded under their own id.
            const r = await onPick(ask.id, hasRealOther ? freeOwnWordsId(ask.ask.options) : "other", ask.revision, t, im);
            if (r && r.ok === false) return { ok: false, error: r.error ?? "pick failed" };
            return { ok: true };
          }}
          answerDisabledReason={unbound ? "Not available: this project is not bound." : undefined}
          showHelp={false}
        />
      ) : null}
      <HowThisWorks later={!!onLater && !!ask.task_id} note={onNote ? otherHelpText(!unbound) : null} />
      <CardConversation taskId={ask.task_id} />
    </article>
  );
}

export const LATER_HELP = "Later sets this card aside: it stays open, moves below the others and drops out of Waiting on you.";

/** The card's standing help (Later, Ask / note), folded into one disclosure so it does not repeat open on every card. */
function HowThisWorks({ later, note }: { later: boolean; note: string | null }) {
  if (!later && note === null) return null;
  return (
    <details className="mt-2 text-xs text-muted-foreground" data-how>
      <summary className="flex min-h-11 cursor-pointer items-center sm:min-h-0">How this works</summary>
      {later ? <p className="mt-1" data-later-help>{LATER_HELP}</p> : null}
      {note !== null ? <p className="mt-1" data-other-help>{note}</p> : null}
    </details>
  );
}

/** One plain sentence on a card under an option: what picking it does. Empty when there is nothing to add. */
export function optionEffect(o: { kind: string; instruction?: string; reversible?: boolean }, thread: string, full = false): string {
  // The schema cannot tell "unset" from "false", so only an option that acts says it cannot be undone; a ruling-only pick has no instruction to undo.
  const acts = o.kind === "instruction" || o.kind === "needs-context";
  const undo = o.reversible === true || !acts ? "" : " Cannot be undone.";
  const text = (o.instruction ?? "").replace(/\s+/g, " ").trim();
  // The default kind says nothing mk needs to know before picking, so it gets no line.
  if (o.kind === "needs-context") return "";
  if (o.kind === "instruction" && text !== "") {
    const first = text.match(/^.*?[.!?](?=\s|$)/)?.[0] ?? text;
    if (full) return `Tells ${thread}: ${text}${undo}`;
    const summary = first.length > 140 ? `${first.slice(0, 139).trimEnd()}…` : first;
    return `Tells ${thread}: ${summary}${undo}`;
  }
  // A ruling-only pick still wakes the asking thread with the ruling, so no line claims it sends nothing. An acting
  // option with no instruction text has nothing to preview, but still says it cannot be undone.
  return undo.trim();
}

/** The Decide queue: a short row per ask on the left, the selected ask in full on the right (below when narrow). */
export function DecideQueue({ owed, onPick, onOpen, nowMs, onNote, onLater, onView }: { owed: OwedAsk[]; onPick: OnPick; onOpen: (thread: string) => void; nowMs: number; onNote?: OnNote; onLater?: OnLater; onView?: (decisionId: string | null) => void }) {
  const [selected, setSelected] = useState<string | null>(null);
  const current = currentAsk(owed, selected);
  const viewingId = current?.id ?? null;
  // Tell the plugin which ask is on screen, so `bb home viewing` can answer "which card is mk looking at".
  useEffect(() => {
    onView?.(viewingId);
    paintDone();
  }, [viewingId]); // eslint-disable-line react-hooks/exhaustive-deps
  const cardRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLOListElement>(null);
  if (current === undefined) return null;
  // The project line says nothing when every ask is from the same project.
  const showProject = new Set(owed.map((o) => o.project ?? "")).size > 1;
  // When the layout wraps (narrow window or narrow panel) the card sits below the whole list; bring it into view on selection.
  const reveal = () => {
    const list = listRef.current;
    const card = cardRef.current;
    // Stacked means the card starts below the list's end; side by side it does not (any window width).
    if (list && card && card.getBoundingClientRect().top >= list.getBoundingClientRect().bottom - 1) card.scrollIntoView({ block: "start" });
  };
  return (
    <div className="flex flex-wrap items-start gap-2 sm:gap-4">
      <ol ref={listRef} className="m-0 min-w-0 shrink-0 grow basis-64 list-none space-y-1 p-0 max-sm:[font-family:ui-sans-serif,system-ui,sans-serif]" data-decide-list>
        {owed.map((o) => {
          const old = isOldAsk(o.filed_at, nowMs);
          const rec = o.ask.options.find((x) => x.id === o.ask.recommendation);
          return (
            <li key={o.id}>
              <button
                type="button"
                aria-current={o.id === current.id ? "true" : undefined}
                className={`min-h-11 w-full rounded border px-2 py-1.5 text-left ${o.id === current.id ? "border-primary bg-muted" : "border-border"}`}
                onClick={() => {
                  clickStarted();
                  setSelected(o.id);
                  reveal();
                }}
              >
                <span className="flex min-w-0 items-baseline gap-1.5 text-sm">
                  {o.key ? <span className="shrink-0 select-all font-mono text-xs font-semibold" data-task-key>{o.key}</span> : null}
                  <span className="min-w-0 [overflow-wrap:anywhere] max-sm:line-clamp-3 sm:truncate">{o.subject}</span>
                </span>
                {showProject && o.project ? <span className="block truncate text-xs text-muted-foreground" data-ask-project>{o.project}</span> : null}
                <span data-ask-meta className="block text-xs text-muted-foreground [overflow-wrap:anywhere]">
                  {ageText(o.filed_at, nowMs)}
                  {old ? <span className="font-medium" data-old-ask>{" · old: still needed?"}</span> : null}
                  {rec ? ` · rec: ${rec.label}` : ""}
                </span>
              </button>
            </li>
          );
        })}
      </ol>
      <div ref={cardRef} className="min-w-0 grow-[3] basis-[28rem] scroll-mt-2 sm:sticky sm:top-0">
        <AskCard key={current.id} ask={current} onPick={onPick} onOpen={onOpen} nowMs={nowMs} {...(onNote ? { onNote } : {})} {...(onLater ? { onLater } : {})} />
      </div>
    </div>
  );
}

/** "expires in 10 h", "expired": an approval's expiry against a clock. An unparseable time is shown as given. */
export function expiryText(expiresAt: string, nowMs: number): string {
  const ms = Date.parse(expiresAt) - nowMs;
  if (Number.isNaN(ms)) return `expires ${expiresAt}`;
  if (ms <= 0) return "expired";
  return `expires in ${ageText(new Date(nowMs - ms).toISOString(), nowMs)}`;
}

export function ApprovalsList({ approvals, onRevoke, nowMs = Date.now() }: { approvals: ApprovalRecord[]; onRevoke: (approvalId: string) => void; nowMs?: number }) {
  return (
    <section data-section="approvals">
      <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">Recorded approvals</h2>
      <p className="mb-2 text-xs text-muted-foreground">Records of what you approved. They do not authorize a merge, deploy or release.</p>
      <ul className="space-y-2">
        {approvals.map((a) => (
          <li key={a.approval_id} className="flex flex-wrap items-center gap-x-2 rounded border border-border p-2 text-sm" data-approval-id={a.approval_id}>
            <span className="min-w-0 [overflow-wrap:anywhere]" title={a.expires_at}>{`${a.kind} ${a.target} at ${a.identity}, ${expiryText(a.expires_at, nowMs)}`}</span>
            <ConfirmTap needsConfirm onConfirmed={() => onRevoke(a.approval_id)} resetKey={a.approval_id}>
              {({ armed, onClick, onBlur }) => (
                <button type="button" className={`min-h-11 text-xs underline sm:min-h-0${armed ? " font-medium text-destructive" : ""}`} onClick={onClick} onBlur={onBlur} {...(armed ? { "data-confirming": "true" } : {})}>{armed ? CONFIRM_LABEL : "Revoke"}</button>
              )}
            </ConfirmTap>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** One card in the Later section. It owns the error from a Move back that did not go through. */
function LaterEntry({ o, onPick, onOpen, nowMs, onNote, onLater, renderMoves }: { o: OwedAsk; onPick: OnPick; onOpen: (thread: string) => void; nowMs: number; onNote?: OnNote; onLater?: OnLater; renderMoves?: (taskId: string) => ReactNode }) {
  const [error, setError] = useState<string | null>(null);
  return (
    <li className="flex min-w-0 flex-wrap items-center gap-x-2 rounded border border-border px-2 py-0.5 text-sm" data-later={o.id}>
      <span className="flex min-w-0 grow items-baseline gap-1.5">
        {o.key ? <span className="shrink-0 select-all font-mono text-xs font-semibold" data-task-key>{o.key}</span> : null}
        <span className="min-w-0 [overflow-wrap:anywhere]">{o.subject}</span>
      </span>
      {onLater && o.task_id ? <ActionButton className="sm:!min-h-11" onClick={() => void runLater(onLater, o.task_id!, false, setError)} data-later-clear={o.id}>Move back</ActionButton> : null}
      <details className="shrink-0 open:basis-full">
        <summary className="flex min-h-11 cursor-pointer items-center text-xs text-muted-foreground">Open card</summary>
        <div className="mt-1">
          <AskCard ask={o} onPick={onPick} onOpen={onOpen} nowMs={nowMs} {...(onNote ? { onNote } : {})} />
        </div>
        {renderMoves && o.task_id ? <div className="mt-2 space-y-3">{renderMoves(o.task_id)}</div> : null}
      </details>
      <LaterFailure error={error} />
    </li>
  );
}

/** The tasks that have an owed decision card, wherever it shows (Decide, Later or on hold). */
export function owedTasks(data: AsksData): Set<string> {
  return new Set(data.owed.map((o) => o.task_id).filter((t): t is string => typeof t === "string"));
}

export function AsksPanel({ part, data, onPick, onOpen, onRevoke, onDismiss, onNote, onLater, onView, renderMoves, hideHeld, nowMs = Date.now() }: { /** Which part of the panel shows: the decide list alone, or Later alone. Unset shows both, as a card-key link does. */ part?: "decide" | "later"; hideHeld?: boolean; renderMoves?: (taskId: string) => ReactNode; data: AsksData; onPick: OnPick; onOpen: (thread: string) => void; onRevoke?: (approvalId: string) => void; onDismiss?: (decisionId: string, obligationId: string) => void; onNote?: OnNote; onLater?: OnLater; onView?: (decisionId: string | null) => void; nowMs?: number }) {
  // In the Queue the held cards show under "Blocked on others"; shown here only on a card-key link.
  const held = hideHeld ? [] : data.owed.filter((o) => o.held);
  // Later cards leave the Decide list for their own group below it; a held card stays held even if it was also set aside.
  const later = data.owed.filter((o) => o.later && !o.held);
  const live = data.owed.some((o) => o.held || o.later) ? { ...data, owed: data.owed.filter((o) => !o.held && !o.later) } : data;
  const onlyLater = part === "later";
  const view = onlyLater ? [] : buildAsksView(live);
  const approvals = onlyLater ? [] : data.approvals ?? [];
  if (onlyLater && later.length === 0) return <p className="p-4 text-sm text-muted-foreground">Nothing is set aside.</p>;
  if (view.length === 0 && approvals.length === 0 && held.length === 0 && (part === "decide" || later.length === 0)) return <p className="p-4 text-sm text-muted-foreground">Nothing needs you.</p>;
  return (
    <div className="space-y-3 p-2 sm:space-y-4 sm:p-3">
      {view.map((s) => (
        <section key={s.key} data-section={s.key}>
          <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">{`${s.title} (${s.items.length})`}</h2>
          {s.key === "decide" ? (
            <DecideQueue owed={live.owed} onPick={onPick} onOpen={onOpen} nowMs={nowMs} {...(onNote ? { onNote } : {})} {...(onLater ? { onLater } : {})} {...(onView ? { onView } : {})} />
          ) : (
            <div className="space-y-3">
              {s.items.map((i) => (
                <div key={i.id} className="rounded border border-border p-3 text-sm [overflow-wrap:anywhere]">
                  <div><RefText text={i.title} /></div>
                  {i.detail ? <div className="text-xs text-muted-foreground"><RefText text={i.detail} /></div> : null}
                  {s.key === "stalled" && i.dismiss && onDismiss ? (
                    <div className="mt-2">
                      <ActionButton onClick={() => onDismiss(i.dismiss!.decision_id, i.dismiss!.obligation_id)} data-dismiss={i.dismiss.obligation_id}>Dismiss</ActionButton>
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          )}
        </section>
      ))}
      {later.length > 0 && part !== "decide" ? (
        <section data-section="later">
          <h2 className="mb-1 text-xs font-semibold uppercase text-muted-foreground">{`Later (${later.length})`}</h2>
          <p className="mb-1 text-xs text-muted-foreground">
            {`Set aside by you. Still open and not ruled; not in Waiting on you.${buildAsksView(live).length === 0 ? " Nothing else needs you now." : ""}`}
          </p>
          <ul className="m-0 list-none space-y-1 p-0">
            {later.map((o) => (
              <LaterEntry key={o.id} o={o} onPick={onPick} onOpen={onOpen} nowMs={nowMs} {...(onNote ? { onNote } : {})} {...(onLater ? { onLater } : {})} {...(renderMoves ? { renderMoves } : {})} />
            ))}
          </ul>
        </section>
      ) : null}
      {held.length > 0 ? (
        <section data-section="on-hold">
          <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">{`On hold (${held.length})`}</h2>
          <ul className="m-0 list-none space-y-2 p-0">
            {held.map((o) => (
              <li key={o.id} className="rounded border border-border p-2 text-sm text-muted-foreground opacity-70" data-held={o.id}>
                <span className="flex min-w-0 items-baseline gap-1.5">
                  {o.key ? <span className="shrink-0 select-all font-mono text-xs font-semibold" data-task-key>{o.key}</span> : null}
                  <span className="min-w-0 truncate">{o.subject}</span>
                </span>
                <span className="block text-xs [overflow-wrap:anywhere]" data-hold-reason>{`On hold: ${o.held!.reason}`}</span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {approvals.length > 0 ? <ApprovalsList approvals={approvals} onRevoke={onRevoke ?? (() => {})} nowMs={nowMs} /> : null}
    </div>
  );
}

type PickReq = { decision_id: string; option_id: string; revision: string; reason?: string; images?: OtherImage[] };
/** A short identity for a set of images, so a retry with the same images keeps its pick id and a changed set does not. */
export const imagesSig = (im?: readonly OtherImage[]): string => (im ?? []).map((i) => `${i.mime}:${i.data_base64.length}:${fnv1a(i.data_base64)}`).join(",");
/** FNV-1a over every character: a changed image changes the key. Not a security hash; the server holds the real identity. */
function fnv1a(s: string): string {
  let a = 0x811c9dc5, b = 0x01000193 ^ 0x5bd1e995;
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    a = Math.imul(a ^ c, 0x01000193) >>> 0;
    b = Math.imul(b ^ c, 0x85ebca6b) >>> 0;
  }
  return `${a.toString(16)}${b.toString(16)}`;
}
type Result = { ok?: boolean; status?: number; error?: string };

/** One pick_id per (decision, option, revision) until the pick resolves, so a retry cannot double-file. */
export class PickController {
  private ids = new Map<string, string>();
  constructor(private mint: () => string) {}

  async send(rpc: (req: PickReq & { pick_id: string }) => Promise<Result>, req: PickReq, reread: () => void): Promise<Result> {
    const key = `${req.decision_id}|${req.option_id}|${req.revision}|${req.reason ?? ""}|${imagesSig(req.images)}`;
    let id = this.ids.get(key);
    if (id === undefined) {
      id = this.mint();
      this.ids.set(key, id);
    }
    const r = await rpc({ ...req, pick_id: id }); // a thrown error keeps the id for the retry
    if (r.ok === true) {
      this.ids.delete(key);
    } else if (r.status === 409) {
      this.ids.delete(key);
      reread();
    }
    return r;
  }
}
