/** Delivery metadata only: never infer blocked work from the question. */
export function validateStopped(m: { urgency?: unknown; stopped?: unknown }, required = true): string | undefined {
  if (!required && !Object.prototype.hasOwnProperty.call(m, "stopped")) return undefined;
  if (m.urgency !== "blocker") {
    if (Object.prototype.hasOwnProperty.call(m, "stopped")) throw new Error("stopped is allowed only with blocker urgency");
    return undefined;
  }
  const s = typeof m.stopped === "string" ? m.stopped.trim() : "";
  if (!s || [...s].length > 200 || /[\r\n\u0085\u2028\u2029]/u.test(s))
    throw new Error("blocker urgency requires stopped: add stopped (or --stopped), a non-blank single line of at most 200 characters describing what is stopped until the answer, or file without blocker urgency (no phone push)");
  return s;
}
