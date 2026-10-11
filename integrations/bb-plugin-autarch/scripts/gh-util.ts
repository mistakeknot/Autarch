// Shared by the GitHub sync scripts (scripts/ is not scanned by the no-exec test). Read-only `gh` with retry.
import { execFileSync } from "node:child_process";

const pause = (ms: number): void => void Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);

/** Runs `gh` with a fixed argv (no shell). A transient failure is retried with a growing pause; the last error is thrown. */
export function gh(argv: string[], tries = 4): string {
  for (let i = 1; ; i++) {
    try {
      return execFileSync("gh", argv, { encoding: "utf8", maxBuffer: 32 * 1024 * 1024, timeout: 60_000, env: { ...process.env, GH_PROMPT_DISABLED: "1", NO_COLOR: "1" } });
    } catch (e) {
      if (i >= tries) throw e;
      pause(1000 * 2 ** (i - 1));
    }
  }
}
