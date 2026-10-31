// Fallback polling for the panel (bead mk-0zih8). Every tick is several reads in the one bb server process, so a tick
// is skipped while the page is hidden, and one read runs when it becomes visible again. Realtime signals still refetch at once.
export const POLL_MS = 30_000;

const hidden = (): boolean => typeof document !== "undefined" && document.visibilityState === "hidden";

/** Run `fn` every `ms` while the page is visible, and once when it turns visible. Returns the stop function. */
export function startPoll(fn: () => void, ms: number): () => void {
  const t = setInterval(() => {
    if (!hidden()) fn();
  }, ms);
  const onVisible = (): void => {
    if (!hidden()) fn();
  };
  if (typeof document !== "undefined") document.addEventListener("visibilitychange", onVisible);
  return () => {
    clearInterval(t);
    if (typeof document !== "undefined") document.removeEventListener("visibilitychange", onVisible);
  };
}
