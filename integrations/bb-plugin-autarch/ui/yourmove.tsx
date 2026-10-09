// "Your move": what mk owes, above the Asks. Everything here is plain text from structured fields: titles, report
// text and notes are rendered as React text (never HTML), commands come from the server's safe builder, and a button
// says what mk says about himself ("I ran it"), never what happened. Only a report or GitHub closes a move.
import { useState } from "react";
import type { MoveView, MoveViewGroups, MoveReportView } from "../moveview.js";
import { ActionButton } from "./buttons.js";
import { CommandBlock, type CopyEnv } from "./copy.js";
import { CardConversation } from "./conversation.js";
import { OtherBox, type OtherOutcome } from "./other.js";

export type MoveActionOutcome = { ok: boolean; error?: string } | void;
export interface MoveHandlers {
  onCheck: (m: MoveView) => Promise<MoveActionOutcome> | MoveActionOutcome;
  onClaim: (m: MoveView) => Promise<MoveActionOutcome> | MoveActionOutcome;
  onSkip: (m: MoveView) => Promise<MoveActionOutcome> | MoveActionOutcome;
  /** Moves a card back out of Later when it was set aside as a whole card. */
  onUnlater?: (m: MoveView) => Promise<MoveActionOutcome> | MoveActionOutcome;
  onNote: (m: MoveView, text: string) => Promise<OtherOutcome | void> | void;
}

/** "2026-10-07 00:12 UTC" from an ISO time; anything unparseable is shown as unknown. */
export function fmtTime(iso: string | null | undefined): string {
  if (!iso) return "unknown time";
  const t = Date.parse(iso);
  return Number.isNaN(t) ? "unknown time" : `${new Date(t).toISOString().slice(0, 16).replace("T", " ")} UTC`;
}

/** The button labels for a move: what mk says about himself. Exported so tests can assert the rules. */
export function moveButtons(m: Pick<MoveView, "kind" | "state" | "claimed_at" | "skipped_at" | "card_later">): { key: "check" | "claim" | "skip"; label: string }[] {
  const out: { key: "check" | "claim" | "skip"; label: string }[] = [];
  const claimed = m.claimed_at !== null;
  if (m.kind === "script") {
    if (!claimed) {
      out.push({ key: "check", label: "I ran --check" });
      out.push({ key: "claim", label: "I ran it" });
    }
  } else if (m.kind === "pr") {
    if (!claimed) out.push({ key: "claim", label: "I'll merge it" });
  } else if (m.kind === "read") {
    if (!claimed) out.push({ key: "claim", label: "I read it" });
  }
  if (m.skipped_at === null && !m.card_later && !claimed) out.push({ key: "skip", label: "Later / skip" });
  return out;
}

const WHY_LABEL: Record<string, string> = { design: "a design choice", taste: "a matter of taste", spend: "a spend decision" };

/** The compact merge card: what merging does, the review verdict (linked), and why this one is still mk's. */
export function PrFacts({ m }: { m: Pick<MoveView, "pr"> }) {
  const pr = m.pr;
  if (!pr || (!pr.summary && !pr.verdict && !pr.why)) return null;
  return (
    <div className="mt-2 space-y-1 text-sm" data-pr-facts>
      {pr.summary ? <p className="m-0 [overflow-wrap:anywhere]" data-pr-summary>{`Merging this: ${pr.summary}`}</p> : null}
      {pr.verdict ? (
        <p className="m-0 text-xs" data-pr-verdict={pr.verdict}>
          {"Review: "}
          {pr.review_url ? <a href={pr.review_url} target="_blank" rel="noopener noreferrer nofollow" className="underline">{pr.verdict}</a> : <span>{pr.verdict}</span>}
        </p>
      ) : null}
      {pr.why && WHY_LABEL[pr.why] ? <p className="m-0 text-xs text-muted-foreground" data-pr-why={pr.why}>{`Yours because it is ${WHY_LABEL[pr.why]}.`}</p> : null}
    </div>
  );
}

function Report({ r }: { r: MoveReportView }) {
  if (r.outcome === "succeeded") {
    return (
      <p className="text-sm" data-report="succeeded">
        {"Report: succeeded. "}
        {r.report_link ? <><code className="rounded bg-muted px-1 font-mono text-xs [overflow-wrap:anywhere]" data-report-link>{r.report_link}</code>{" "}</> : null}
        {`(${fmtTime(r.reported_at)})`}
      </p>
    );
  }
  if (r.outcome === "failed") {
    return (
      <div className="rounded-md border border-destructive p-2 text-sm" role="alert" data-report="failed">
        <p className="m-0 font-medium text-destructive">Report: FAILED. The card stays open.</p>
        <p className="m-0 [overflow-wrap:anywhere]" data-failing-step>{`Failing step: ${r.failing_step ?? "not named in the report"}`}</p>
        {r.error_line ? <p className="m-0 [overflow-wrap:anywhere]" data-error-line>{`Error: ${r.error_line}`}</p> : null}
        {r.report_link ? <p className="m-0 text-xs [overflow-wrap:anywhere]">{"Report: "}<code className="font-mono" data-report-link>{r.report_link}</code></p> : null}
        <p className="m-0 text-xs text-muted-foreground">{`Reported ${fmtTime(r.reported_at)}. Report text is the script's own and is not verified.`}</p>
      </div>
    );
  }
  return (
    <p className="text-sm font-medium" data-report="no-report">
      {`No report received${r.deadline_at ? ` by ${fmtTime(r.deadline_at)}` : ""}. This is not a failure and not a success: it is unknown.`}
    </p>
  );
}

export function MoveCard({ m, h, env, section }: { m: MoveView; h: MoveHandlers; env?: CopyEnv | undefined; section: "yourMove" | "reported" | "later" | "hidden" }) {
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const act = async (key: "check" | "claim" | "skip" | "back") => {
    setBusy(key);
    setError(null);
    try {
      const f = key === "check" ? h.onCheck : key === "claim" ? h.onClaim : key === "back" ? h.onUnlater : h.onSkip;
      const r = await f?.(m);
      if (r && r.ok === false) setError(r.error ?? "that did not go through");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };
  const buttons = section === "hidden" ? [] : moveButtons(m);
  const claimed = m.claimed_at !== null;
  const aside = m.card_later === true || m.skipped_at !== null;
  return (
    <article className="min-w-0 rounded-lg border border-border bg-card p-4" data-move={m.task_id} data-kind={m.kind} data-section-of={section}>
      <h3 className="m-0 text-sm font-medium [overflow-wrap:anywhere]">{m.title}</h3>
      <p className="mt-1 text-xs text-muted-foreground">{`${m.kind} move · ${m.task_id}${m.owner ? ` · owner ${m.owner}` : ""} · opened ${fmtTime(m.opened_at)}`}</p>
      {m.kind === "context" && m.need ? <p className="mt-2 whitespace-pre-wrap text-sm [overflow-wrap:anywhere]" data-need>{`Needed from you: ${m.need}`}</p> : null}
      {m.kind === "pr" && m.pr ? <PrFacts m={m} /> : null}
      {m.url ? <p className="mt-2 text-sm [overflow-wrap:anywhere]">{m.kind === "pr" ? "Pull request: " : "Open: "}<code className="font-mono text-xs" data-url>{m.url}</code></p> : null}
      {m.kind === "script" && m.commands.length > 0 ? (
        <div className="mt-3 space-y-3" data-commands>
          {m.commands.map((c) => <CommandBlock key={c.label} label={c.label === "sha256sum" ? "1. Check the file (sha256sum)" : c.label === "check" ? "2. Dry run (--check)" : c.label === "run" ? "3. The real run" : "Recovery"} command={c.command} expectedSha={c.expectedSha} primary={c.label === "run"} showFlags={c.label === "run"} {...(env ? { env } : {})} />)}
        </div>
      ) : null}
      {m.kind === "script" && m.checked_at ? <p className="mt-2 text-xs text-muted-foreground" data-checked>{`You said you ran --check at ${fmtTime(m.checked_at)}. This does not start the report clock.`}</p> : null}
      {claimed ? (
        <p className="mt-2 text-sm" data-claimed>
          {m.kind === "script" ? (m.report ? `You said you ran it at ${fmtTime(m.claimed_at)}.` : "Ran: waiting for the script's report") : m.kind === "pr" ? `You said you'd merge it (${fmtTime(m.claimed_at)}). Waiting for GitHub to show it merged.` : `You said you read it (${fmtTime(m.claimed_at)}). Not verified.`}
          {m.kind === "script" && !m.report && m.report_deadline_at ? <span className="text-xs text-muted-foreground">{` The report is due by ${fmtTime(m.report_deadline_at)}.`}</span> : null}
        </p>
      ) : null}
      {m.held ? <p className="mt-2 text-xs text-muted-foreground" data-held>{`On hold: ${m.held.reason}`}</p> : null}
      {m.skipped_at && !m.held ? <p className="mt-2 text-xs text-muted-foreground" data-skipped>{`Later: you skipped this at ${fmtTime(m.skipped_at)}. It is still open.`}</p> : null}
      {m.card_later ? <p className="mt-2 text-xs text-muted-foreground" data-card-later>Later: you set this card aside. It is still open and not in Waiting on you.</p> : null}
      {m.report ? <div className="mt-2"><Report r={m.report} /></div> : null}
      {buttons.length > 0 || (aside && h.onUnlater) ? (
        <div className="mt-3 flex flex-wrap items-center gap-2" data-move-buttons>
          {aside && h.onUnlater ? <ActionButton disabled={busy !== null} onClick={() => void act("back")} data-later-clear={m.task_id}>Move back</ActionButton> : null}
          {buttons.map((b) => (
            <ActionButton key={b.key} disabled={busy !== null} onClick={() => void act(b.key)} data-move-action={b.key}>{b.label}</ActionButton>
          ))}
        </div>
      ) : null}
      {error !== null ? <p role="alert" className="mt-2 text-xs font-medium text-destructive" data-move-failure>{`That did not go through: ${error}`}</p> : null}
      {section !== "hidden" ? <OtherBox onNote={(t) => h.onNote(m, t)} /> : null}
      <CardConversation taskId={m.task_id} />
    </article>
  );
}

const HEAD = "mb-2 text-xs font-semibold uppercase text-muted-foreground";

/** The move Later group without the tasks the Decide Later group already lists, so one card is not shown twice. */
export function omitLaterTasks(data: MoveViewGroups, tasks: ReadonlySet<string>): MoveViewGroups {
  return { ...data, later: data.later.filter((m) => !tasks.has(m.task_id)) };
}

/** `part` splits the panel so the Later group can sit below the Decide cards: "active" is everything else. */
export function YourMovePanel({ data, handlers, env, part }: { data: MoveViewGroups; handlers: MoveHandlers; env?: CopyEnv | undefined; part?: "active" | "later" }) {
  const showLater = part !== "active";
  const showRest = part !== "later";
  const n = (showRest ? data.yourMove.length + data.reported.length + data.hidden.length + data.audit.length : 0) + (showLater ? data.later.length : 0);
  if (n === 0) return null;
  const group = (key: "yourMove" | "reported" | "later" | "hidden", title: string, note?: string) =>
    data[key].length === 0 ? null : (
      <section data-section={`move-${key}`}>
        <h2 className={HEAD}>{`${title} (${data[key].length})`}</h2>
        {note ? <p className="mb-2 text-xs text-muted-foreground">{note}</p> : null}
        <div className="space-y-3">{data[key].map((m) => <MoveCard key={`${m.task_id}:${m.generation}`} m={m} h={handlers} env={env} section={key} />)}</div>
      </section>
    );
  return (
    <div className="space-y-6 p-4 pb-0" data-panel="your-move">
      {showRest ? group("yourMove", "Your move") : null}
      {showRest ? group("reported", "Reported done, not verified", "These say what you or a script reported. Only a report or GitHub closes a move.") : null}
      {showLater ? group("later", "Later") : null}
      {showRest ? group("hidden", "Hidden") : null}
      {showRest && data.audit.length > 0 ? (
        <details data-section="move-audit">
          <summary className="cursor-pointer text-xs font-semibold uppercase text-muted-foreground">{`Closed (${data.audit.length})`}</summary>
          <ul className="mt-2 list-none space-y-1 p-0 text-xs text-muted-foreground">
            {data.audit.map((a) => (
              <li key={`${a.task_id}:${a.generation}`} className="[overflow-wrap:anywhere]" data-audit={a.task_id}>
                {`${a.task_id} (${a.kind}) closed ${fmtTime(a.closed_at)} by ${a.closed_by ?? "unknown"}${a.evidence ? `: ${a.evidence}` : ""}`}
              </li>
            ))}
          </ul>
        </details>
      ) : null}
    </div>
  );
}
