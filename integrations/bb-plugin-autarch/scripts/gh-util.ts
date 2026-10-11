// Shared by the GitHub sync scripts (scripts/ is not scanned by the no-exec test). Read-only `gh` with retry.
import { execFileSync } from "node:child_process";

const pause = (ms: number): void => void Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);

/** Runs `gh` with a fixed argv (no shell). A transient failure is retried with a growing pause; the last error is thrown. */
export function gh(argv: string[], tries = 4): string {
  for (let i = 1; ; i++) {
    try {
      return execFileSync("gh", argv, { encoding: "utf8", maxBuffer: 32 * 1024 * 1024, timeout: 60_000, env: { ...process.env, GH_PROMPT_DISABLED: "1", NO_COLOR: "1" } });
    } catch (e) {
      if (i >= tries) throw limited(e) ?? e;
      pause(1000 * 2 ** (i - 1));
    }
  }
}

/** A rate-limit failure becomes one plain sentence instead of a stack trace; anything else is left alone. */
function limited(e: unknown): Error | null {
  const text = String((e as { stderr?: unknown }).stderr ?? (e as Error).message);
  if (!/rate limit/i.test(text)) return null;
  try {
    const r = JSON.parse(execFileSync("gh", ["api", "rate_limit", "--jq", ".resources"], { encoding: "utf8", timeout: 30_000 })) as Record<string, { remaining: number; reset: number }>;
    const hit = Object.values(r).filter((x) => x.remaining === 0).map((x) => x.reset).sort()[0];
    if (hit) return new Error(`GitHub rate-limited until ${new Date(hit * 1000).toISOString().slice(11, 16)}Z; nothing filed, re-run after that`);
  } catch { /* fall through to the generic wording */ }
  return new Error("GitHub rate-limited; nothing filed, re-run in a while");
}
