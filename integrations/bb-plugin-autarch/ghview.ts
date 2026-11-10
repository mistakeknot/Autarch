// The read-only GitHub PR reader (`gh pr view`) that the merge recheck uses. A failure throws; the caller decides what a
// failed read means.
import { execFile } from "node:child_process";

const GH_TIMEOUT_MS = 20_000;
const PR_URL = /^https:\/\/github\.com\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\/pull\/[1-9][0-9]{0,8}$/;

export interface PrView {
  state: string;
  mergedAt: string | null;
}
/** Injectable so tests never touch the network. Throws on any failure. */
export type GhView = (url: string) => Promise<PrView>;

/** The real reader: `gh pr view <url> --json state,mergedAt`, a timeout, the URL validated first. */
export const ghViewReal: GhView = (url) =>
  new Promise((resolve, reject) => {
    if (!PR_URL.test(url)) return reject(new Error("not a pull request URL"));
    execFile("gh", ["pr", "view", url, "--json", "state,mergedAt"], { timeout: GH_TIMEOUT_MS, maxBuffer: 64 * 1024, env: { ...process.env, GH_PROMPT_DISABLED: "1", NO_COLOR: "1" } }, (err, stdout) => {
      if (err) return reject(new Error(`gh pr view failed: ${String((err as Error).message).split("\n")[0]!.slice(0, 200)}`));
      try {
        const v = JSON.parse(stdout) as { state?: unknown; mergedAt?: unknown };
        if (typeof v.state !== "string") return reject(new Error("gh answered without a state"));
        resolve({ state: v.state, mergedAt: typeof v.mergedAt === "string" ? v.mergedAt : null });
      } catch {
        reject(new Error("gh answered something that is not JSON"));
      }
    });
  });
