// mk-bzi8s.5: best-effort, privacy-preserving observations; never a pick gate.
import { appendFileSync, mkdirSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

export interface PilotPickMetric {
  requestKey: string; cardKey: string; project: string; pickId: string; time: string;
  optionKind: string; other: boolean; reason?: string | null;
}
/** pasted-error-v1 is a heuristic, not a diagnosis. Never persist the input text. */
export function pastedErrorV1(text: string): boolean {
  return /(?:^\s*(?:error:|[A-Za-z_$][\w.$]*(?:Error|Exception):)\s*\S|^\s*Traceback \(most recent call last\):\s*$|^\s*stack trace:\s*$|^\s*at\s+[^\r\n]*\.(?:[cm]?[jt]sx?|java|py|go|rs):\d+(?::\d+)?\)?\s*$|^\s*(?:exit(?:ed)?(?:\s+(?:with|status|code))*|Process exited with code)\s*[:=]?\s*\d+[.!]?\s*$)/im.test(text);
}
export function recordPilotPick(input: PilotPickMetric, env: NodeJS.ProcessEnv = process.env): void {
  if (env.AUTARCH_NEEDSMK_PILOTS_OFF === "1") return;
  const dir = env.AUTARCH_CARD_STATE_DIR || join(env.XDG_STATE_HOME || join(homedir(), ".local", "state"), "autarch", "needsmk");
  let pilots: Record<string, { enabled?: boolean; until?: string }>;
  try { pilots = JSON.parse(readFileSync(join(dir, "pilots.json"), "utf8")); } catch { return; }
  if (!["coldread", "daycap", "linkcheck"].some(name => pilots?.[name]?.enabled === true && Date.now() < Date.parse(pilots[name].until || ""))) return;
  let state: Record<string, { Request?: { Type?: string; Urgent?: boolean; KeepList?: string; RootRun?: unknown } }> = {};
  try { state = JSON.parse(readFileSync(join(dir, "filings.json"), "utf8")); } catch { /* metadata can be unavailable */ }
  const request = state[input.requestKey]?.Request;
  // filingEntry writes request with a lowercase JSON tag.
  const entry = state[input.requestKey] as { request?: typeof request } | undefined;
  const req = entry?.request ?? request;
  const text = input.reason || "";
  const metric = {
    time: input.time, event: "picked", event_id: `pick:${input.pickId}`,
    request_key: input.requestKey, card_key: input.cardKey, project: input.project,
    card_type: req?.Type || "other",
    exempt_flags: [req?.Urgent && "urgent", req?.KeepList && "keep-list", req?.Type === "merge" && "merge", req?.RootRun && "root-run"].filter(Boolean),
    option_kind: input.optionKind, other: input.other, free_text_length: Array.from(text).length,
    pasted_error: pastedErrorV1(text), heuristic: "pasted-error-v1",
  };
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  const path = join(dir, "pilots-metrics.jsonl");
  // Reconcile retries find the stable pick identity; append is one O_APPEND write.
  try {
    if (readFileSync(path, "utf8").split("\n").some(line => { try { return JSON.parse(line).event_id === metric.event_id; } catch { return false; } })) return;
  } catch (e) { if ((e as NodeJS.ErrnoException).code !== "ENOENT") throw e; }
  appendFileSync(path, JSON.stringify(metric) + "\n", { mode: 0o600 });
}
