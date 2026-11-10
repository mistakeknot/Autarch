// The phone's 5-second Undo (bead mk-j5ipa.13). After a one-tap pick on the phone the pick is held here for five seconds under a
// "sending in 5 s · Undo" bar. It goes through the normal answer path (the page's onPick) only when the bar closes; Undo drops it
// and the card is back, unanswered. Nothing is held on the server and the desktop does not use this.
//
// Held picks are the ones that took one tap: no typed words, no images, no multi-question answers, and no option that asks for a
// second tap to confirm. Those keep their own flow and go at once.
//
// A held pick is sent exactly once: when the bar closes, when the page is hidden or closed, when the phone inbox goes away (Full
// view), or when another card is picked meanwhile (the first can no longer be undone). The send is the page's own onPick, the one
// answer path there is (a plugin RPC with a retry-safe pick id), not a fetch or a beacon: the answer route is not an HTTP route.
import { useCallback, useEffect, useRef, useState } from "react";
import { ActionButton } from "./buttons.js";
import type { OnPick } from "./asks.js";

export const UNDO_MS = 5000;

interface Held {
  decisionId: string;
  optionId: string;
  revision: string;
  endsAt: number;
}

export interface PickHold {
  /** Wraps the page's onPick: a one-tap pick is held, any other goes straight through. */
  pick: OnPick;
  /** The decision being held, for the list to mark it. */
  heldId: string | null;
  /** Whole seconds left on the bar, or null when nothing is held. */
  seconds: number | null;
  /** What the last send said when it did not go through; shown in the bar until dismissed. */
  failure: string | null;
  undo: () => void;
  dismissFailure: () => void;
}

/** `eligible` says whether a (decision, option) pick is a one-tap pick that may be held. */
export function usePickHold(onPick: OnPick, eligible: (decisionId: string, optionId: string) => boolean): PickHold {
  const [held, setHeld] = useState<Held | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const [failure, setFailure] = useState<string | null>(null);
  const heldRef = useRef<Held | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  // The latest callbacks, for the timer and the page events, which outlive a render.
  const onPickRef = useRef(onPick);
  const eligibleRef = useRef(eligible);
  useEffect(() => {
    onPickRef.current = onPick;
    eligibleRef.current = eligible;
  });

  const clearTimer = () => {
    if (timer.current !== null) clearTimeout(timer.current);
    timer.current = null;
  };

  /** Sends the held pick now, once; a no-op when nothing is held. */
  const flush = useCallback(() => {
    const h = heldRef.current;
    if (h === null) return;
    heldRef.current = null;
    clearTimer();
    setHeld(null);
    const failed = (error: string) => setFailure(error);
    try {
      void Promise.resolve(onPickRef.current(h.decisionId, h.optionId, h.revision)).then(
        (r) => { if (r && r.ok === false) failed(r.error ?? "that did not go through"); },
        (e) => failed(e instanceof Error ? e.message : String(e)),
      );
    } catch (e) {
      failed(e instanceof Error ? e.message : String(e));
    }
  }, []);

  const pick: OnPick = useCallback((...args) => {
    const [decisionId, optionId, revision, reason, images, answers] = args;
    const oneTap = reason === undefined && (images === undefined || images.length === 0) && answers === undefined && eligibleRef.current(decisionId, optionId);
    const cur = heldRef.current;
    if (!oneTap) {
      // A later explicit answer (confirmed destructive pick, typed words, images) on the card that is held supersedes the held
      // pick: drop it so the two never both reach the agent, and the earlier one cannot land after a failed later one.
      if (cur !== null && cur.decisionId === decisionId) {
        heldRef.current = null;
        clearTimer();
        setHeld(null);
      }
      return onPickRef.current(...args);
    }
    // A second tap on the card already held does not replace it; it resolves first.
    if (cur !== null && cur.decisionId === decisionId) return;
    flush();
    setFailure(null);
    const h: Held = { decisionId, optionId, revision, endsAt: Date.now() + UNDO_MS };
    heldRef.current = h;
    setNow(Date.now());
    setHeld(h);
    timer.current = setTimeout(flush, UNDO_MS);
  }, [flush]);

  const undo = useCallback(() => {
    heldRef.current = null;
    clearTimer();
    setHeld(null);
  }, []);

  // The bar counts down once a second.
  useEffect(() => {
    if (held === null) return;
    const t = setInterval(() => setNow(Date.now()), 250);
    return () => clearInterval(t);
  }, [held]);

  // Hidden or closed before the bar closes: send it now rather than lose it.
  useEffect(() => {
    if (held === null) return;
    const hide = () => flush();
    const vis = () => { if (document.visibilityState === "hidden") flush(); };
    window.addEventListener("pagehide", hide);
    document.addEventListener("visibilitychange", vis);
    return () => {
      window.removeEventListener("pagehide", hide);
      document.removeEventListener("visibilitychange", vis);
    };
  }, [held, flush]);

  // The inbox goes away (Full view, or the page unmounts): send it.
  useEffect(() => flush, [flush]);

  const seconds = held === null ? null : Math.max(0, Math.ceil((held.endsAt - now) / 1000));
  return { pick, heldId: held?.decisionId ?? null, seconds, failure, undo, dismissFailure: () => setFailure(null) };
}

/**
 * The bar. It appears at once (no slide) and its only change is the number. Undo is a 44 px target. A failed send replaces the
 * countdown with what went wrong; the card is still open, so mk can pick again.
 */
export function PickHoldBar({ hold }: { hold: PickHold }) {
  if (hold.seconds !== null) {
    return (
      <div className="flex min-h-12 items-center justify-between gap-2 border-t border-border bg-muted px-4 py-1 text-sm" data-pick-hold>
        <span aria-hidden="true">{`sending in ${hold.seconds} s ·`}</span>
        <span className="sr-only" role="status">Your answer is sent in 5 seconds. Undo is available.</span>
        <ActionButton tone="quiet" className="min-h-11" onClick={hold.undo} data-pick-undo>Undo</ActionButton>
      </div>
    );
  }
  if (hold.failure !== null) {
    return (
      <div className="flex min-h-12 items-center justify-between gap-2 border-t border-border bg-muted px-4 py-1 text-sm" data-pick-hold-failure>
        <span role="alert" className="font-medium text-destructive">{`Your answer did not go through: ${hold.failure}. The card is still open; try again.`}</span>
        <ActionButton tone="quiet" className="min-h-11" onClick={hold.dismissFailure}>OK</ActionButton>
      </div>
    );
  }
  return null;
}
