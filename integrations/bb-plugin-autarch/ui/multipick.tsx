// A decision with several independent choices: each question is a row of radio chips, the recommended chip is preselected,
// and one Answer sends every pick together. A pick that differs from the recommendation is marked before it is sent.
import { useId, useState } from "react";
import type { Question } from "../model.js";
import { ActionButton, RecommendedMark } from "./buttons.js";

/** The starting picks: each question's recommended option; a question with no recommendation starts unanswered. */
export function initialPicks(questions: readonly Question[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const q of questions) if (q.recommendation && q.options.some((o) => o.id === q.recommendation)) out[q.id] = q.recommendation;
  return out;
}

/** True when every question has one of its own options picked. */
export function complete(questions: readonly Question[], picks: Readonly<Record<string, string>>): boolean {
  return questions.every((q) => q.options.some((o) => o.id === picks[q.id]));
}

export function MultiPick({ questions, onAnswer, disabled = false }: { questions: readonly Question[]; onAnswer: (answers: Record<string, string>, note: string | undefined) => Promise<{ ok: boolean; error?: string }>; disabled?: boolean }) {
  const [picks, setPicks] = useState<Record<string, string>>(() => initialPicks(questions));
  const group = useId();
  const [note, setNote] = useState("");
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const ready = complete(questions, picks);
  const changed = questions.filter((q) => q.recommendation && picks[q.id] !== undefined && picks[q.id] !== q.recommendation).length;
  const send = async () => {
    setFailure(null);
    setPending(true);
    try {
      const r = await onAnswer(picks, note.trim() === "" ? undefined : note.trim());
      if (!r.ok) setFailure(r.error ?? "pick failed");
    } finally {
      setPending(false);
    }
  };
  return (
    <div className="mt-3 space-y-3" data-multipick>
      {questions.map((q, i) => (
        <fieldset key={q.id} className="m-0 min-w-0 rounded border border-border p-2" data-question={q.id}>
          <legend className="px-1 text-sm font-medium [overflow-wrap:anywhere]">{`${i + 1}. ${q.prompt}`}</legend>
          <div className="space-y-1" role="radiogroup" aria-label={q.prompt}>
            {q.options.map((o) => {
              const on = picks[q.id] === o.id;
              const recommended = q.recommendation === o.id;
              return (
                <label key={o.id} className={`flex min-h-11 cursor-pointer items-start gap-2 rounded border p-2 text-sm sm:min-h-0 ${on ? "border-primary bg-muted" : "border-border"}`} data-option={o.id} data-chosen={on ? "true" : undefined}>
                  <input type="radio" name={`${group}-${q.id}`} value={o.id} checked={on} disabled={disabled || pending} onChange={() => setPicks({ ...picks, [q.id]: o.id })} className="mt-1" />
                  <span className="min-w-0">
                    <span className="flex flex-wrap items-center gap-2">
                      <span className="font-medium [overflow-wrap:anywhere]">{o.label}</span>
                      {recommended ? <RecommendedMark /> : null}
                    </span>
                    {o.detail ? <span className="block text-xs text-muted-foreground [overflow-wrap:anywhere]" data-detail>{o.detail}</span> : null}
                  </span>
                </label>
              );
            })}
          </div>
          {q.recommendation && picks[q.id] !== undefined && picks[q.id] !== q.recommendation ? <p className="mt-1 text-xs font-medium" data-changed>Different from the recommendation.</p> : null}
        </fieldset>
      ))}
      <label className="block text-xs text-muted-foreground">
        Note with your answers (optional)
        <textarea value={note} onChange={(e) => setNote(e.target.value)} rows={2} maxLength={20000} disabled={disabled || pending} className="mt-1 block w-full rounded border border-border bg-background p-2 text-sm" data-multipick-note />
      </label>
      <div className="flex flex-wrap items-center gap-2">
        <ActionButton tone="recommended" disabled={!ready || disabled || pending} selected={pending} onClick={() => void send()} data-multipick-answer>Answer</ActionButton>
        <span className="text-xs text-muted-foreground" data-multipick-status>
          {ready ? (changed > 0 ? `${changed} of ${questions.length} differ from the recommendation.` : "All recommended.") : "Pick one option for every question."}
        </span>
      </div>
      {failure !== null ? <p role="alert" className="text-xs font-medium text-destructive" data-pick-failure>{`Your answers did not go through: ${failure}. The ask is still open; try again.`}</p> : null}
    </div>
  );
}
