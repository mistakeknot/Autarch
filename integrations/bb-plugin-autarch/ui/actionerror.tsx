// A button that sends an RPC used to drop a refusal or a thrown error on the floor (`.then(refetch, () => {})`), so a
// click that did nothing looked like one that worked. This keeps the last failure as one line the page shows, in the
// same `role="alert"` form the other panels use for a failed action. Display only: it changes no request.
import { useCallback, useState } from "react";
import { rpcOutcome } from "../movehandlers.js";

/** The line shown for a failed action: what mk clicked, then the reason. */
export function actionErrorText(label: string, error: string | undefined): string {
  return `${label} did not go through: ${error ?? "no reason was given"}`;
}

/**
 * Sends one action and reports the result. A success clears the line; a refusal or a thrown error sets it. `after`
 * (the refetch) runs either way, so the page shows what the server holds now.
 */
export async function runAction(label: string, send: Promise<unknown>, setError: (message: string | null) => void, after?: () => void): Promise<void> {
  const out = await rpcOutcome(send);
  setError(out.ok ? null : actionErrorText(label, out.error));
  after?.();
}

/** The last failed action's line (or null) and the function that sends an action. */
export function useActionError(): { error: string | null; act: (label: string, send: Promise<unknown>, after?: () => void) => void } {
  const [error, setError] = useState<string | null>(null);
  const act = useCallback((label: string, send: Promise<unknown>, after?: () => void) => void runAction(label, send, setError, after), []);
  return { error, act };
}

export function ActionError({ message }: { message: string | null }) {
  if (message === null) return null;
  return <p role="alert" className="m-0 px-4 py-2 text-sm font-medium text-destructive" data-action-error>{message}</p>;
}
