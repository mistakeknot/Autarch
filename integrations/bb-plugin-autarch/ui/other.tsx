// The Other box on every card: mk's own words. "Ask / note" posts a comment and wakes the owner and never rules;
// "Answer with this" records the words as a custom pick. The text is plain text, trimmed, capped at 2000 characters.
// The server enforces the same cap; the counter here is a courtesy.
import { useState } from "react";
import { ActionButton } from "./buttons.js";

export const OTHER_MAX = 2000;

export type OtherOutcome = { ok: boolean; error?: string };

/** The trimmed text, or null when it is empty or over the cap (the buttons stay disabled). */
export function otherText(raw: string): string | null {
  const t = raw.trim();
  const n = Array.from(t).length; // code points, as the service counts them
  return n === 0 || n > OTHER_MAX ? null : t;
}

export function OtherBox({
  onNote,
  onAnswer,
  answerDisabledReason,
  label = "Other",
}: {
  onNote: (text: string) => Promise<OtherOutcome | void> | void;
  /** Absent: the card cannot be ruled here (no pick buttons), so only Ask / note is offered. */
  onAnswer?: ((text: string) => Promise<OtherOutcome | void> | void) | undefined;
  /** When set, "Answer with this" is shown disabled with this reason. */
  answerDisabledReason?: string | undefined;
  label?: string;
}) {
  const [raw, setRaw] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const t = otherText(raw);
  const count = Array.from(raw.trim()).length;
  const over = count > OTHER_MAX;
  const run = async (f: (text: string) => Promise<OtherOutcome | void> | void, okText: string, clear: boolean) => {
    if (t === null || busy) return;
    setBusy(true);
    setMsg(null);
    try {
      const r = await f(t);
      if (r && r.ok === false) setMsg({ ok: false, text: r.error ?? "not sent" });
      else {
        setMsg({ ok: true, text: okText });
        if (clear) setRaw("");
      }
    } catch (e) {
      setMsg({ ok: false, text: e instanceof Error ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="mt-3 min-w-0" data-other>
      <label className="block text-xs font-medium text-muted-foreground">
        {label}
        <textarea
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          rows={3}
          className="mt-1 block w-full min-w-0 resize-y rounded-md border border-input bg-transparent p-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          placeholder="Your own words: a question, a note, or a different answer. Plain text."
          data-other-text
        />
      </label>
      <div className="mt-1 flex flex-wrap items-center gap-2">
        <span className={`text-xs ${over ? "font-medium text-destructive" : "text-muted-foreground"}`} data-other-counter aria-live="polite">{`${count} / ${OTHER_MAX}`}</span>
        <ActionButton disabled={t === null || busy} onClick={() => void run(onNote, "Sent. The owner is woken; nothing was ruled or closed.", true)} data-other-note>Ask / note</ActionButton>
        {onAnswer || answerDisabledReason ? (
          <ActionButton
            disabled={t === null || busy || !onAnswer || answerDisabledReason !== undefined}
            title={answerDisabledReason}
            onClick={() => onAnswer && void run(onAnswer, "Recorded as your answer.", false)}
            data-other-answer
          >
            Answer with this
          </ActionButton>
        ) : null}
      </div>
      {answerDisabledReason ? <p className="mt-1 text-xs text-muted-foreground" data-other-answer-reason>{answerDisabledReason}</p> : null}
      <p className="mt-1 text-xs text-muted-foreground" data-other-help>{`Ask / note sends a comment and wakes the owner; it never rules or closes.${onAnswer ? " The answer button records your words as your ruling." : ""}`}</p>
      {msg ? <p role={msg.ok ? "status" : "alert"} className={`mt-1 text-xs ${msg.ok ? "text-muted-foreground" : "font-medium text-destructive"}`} data-other-result={msg.ok ? "ok" : "error"}>{msg.text}</p> : null}
    </div>
  );
}
