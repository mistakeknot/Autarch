// Tap again to confirm: a destructive action asks for a second tap within a short window, so a mis-tap on a phone
// cannot record a ruling, reject a binding or revoke an approval. UI only: nothing is sent until the second tap.
import { useEffect, useState, type ReactNode } from "react";

/** How long the button waits for the second tap before it goes back to its normal label. */
export const CONFIRM_MS = 4000;

export const CONFIRM_LABEL = "Tap again to confirm";

/** What the button wires up: its click and blur handlers, and whether it is waiting for the second tap. */
export type ConfirmProps = { armed: boolean; onClick: () => void; onBlur: () => void };

/**
 * One tap on an action that needs confirming. Not armed: arm it (the caller shows CONFIRM_LABEL). Armed: run it and
 * disarm. An action that does not need confirming runs on the first tap and never arms.
 */
export function tapStep(armed: boolean, needsConfirm: boolean): { armed: boolean; run: boolean } {
  if (!needsConfirm) return { armed: false, run: true };
  return armed ? { armed: false, run: true } : { armed: true, run: false };
}

/**
 * Wraps one button. Armed state resets after CONFIRM_MS, on blur, and when `resetKey` changes (another card or
 * revision), and goes away with the button. `initialArmed` is for tests that render the armed look.
 */
export function ConfirmTap({ needsConfirm, onConfirmed, resetKey, initialArmed = false, children }: { needsConfirm: boolean; onConfirmed: () => void; resetKey?: string; initialArmed?: boolean; children: (p: ConfirmProps) => ReactNode }) {
  const [armed, setArmed] = useState(initialArmed);
  useEffect(() => {
    if (!armed) return;
    const t = setTimeout(() => setArmed(false), CONFIRM_MS);
    return () => clearTimeout(t);
  }, [armed]);
  // A different card or revision starts disarmed; the first run keeps `initialArmed`.
  const [seenKey, setSeenKey] = useState(resetKey);
  if (seenKey !== resetKey) {
    setSeenKey(resetKey);
    if (armed) setArmed(false);
  }
  return (
    <>
      {children({
        armed: needsConfirm && armed,
        onClick: () => {
          const s = tapStep(armed, needsConfirm);
          setArmed(s.armed);
          if (s.run) onConfirmed();
        },
        onBlur: () => setArmed(false),
      })}
    </>
  );
}
