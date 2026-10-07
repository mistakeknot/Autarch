// The PR poller (plan D2, Revision 2): the only thing that closes a move by itself. It reads GitHub state
// (read-only, `gh pr view`) for open or claimed `pr` moves and closes one only on MERGED or CLOSED.
// A failed poll, an unknown state or malformed output leaves the move open and is counted, never a close.
import { execFile } from "node:child_process";
import type { MoveRow } from "./store.js";
import type { Service } from "./service.js";

export const PR_POLL_MS = 5 * 60_000;
const GH_TIMEOUT_MS = 20_000;
const CONCURRENCY = 2;
const MAX_BACKOFF_MS = 60 * 60_000;
export const PR_URL = /^https:\/\/github\.com\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\/pull\/[1-9][0-9]{0,8}$/;

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

export interface PollStats {
  polls: number;
  errors: number;
  closed: number;
}

export interface PrPollerOptions {
  svc: Service;
  ghView: GhView;
  /** Epoch ms; injectable. */
  clock?: () => number;
  intervalMs?: number;
  log?: (msg: string) => void;
}

export class PrPoller {
  readonly stats: PollStats = { polls: 0, errors: 0, closed: 0 };
  private readonly failures = new Map<string, { n: number; until: number }>();
  private readonly clock: () => number;

  constructor(private readonly o: PrPollerOptions) {
    this.clock = o.clock ?? Date.now;
  }

  private live(): (MoveRow & { url: string })[] {
    const out: (MoveRow & { url: string })[] = [];
    for (const m of this.o.svc.store.moves()) {
      if (m.kind !== "pr" || m.state === "closed") continue;
      let url: unknown;
      try {
        url = (JSON.parse(m.payload_json) as { url?: unknown }).url;
      } catch {
        continue;
      }
      if (typeof url === "string" && PR_URL.test(url)) out.push({ ...m, url });
    }
    return out;
  }

  private async one(m: MoveRow & { url: string }): Promise<void> {
    const key = `${m.task_id}#${m.generation}`;
    this.stats.polls++;
    try {
      const v = await this.o.ghView(m.url);
      if (v.state === "MERGED" || v.state === "CLOSED") {
        const evidence = JSON.stringify({ url: m.url, state: v.state, mergedAt: v.mergedAt });
        if (this.o.svc.closeMoveOn(m.task_id, m.generation, "github", evidence)) this.stats.closed++;
      } else if (v.state === "OPEN") {
        this.o.svc.store.markMoveChecked(m.task_id, m.generation, null);
      } else {
        throw new Error(`unknown PR state ${JSON.stringify(String(v.state).slice(0, 40))}`);
      }
      this.failures.delete(key);
    } catch (e) {
      this.stats.errors++;
      const msg = e instanceof Error ? e.message : String(e);
      const n = (this.failures.get(key)?.n ?? 0) + 1;
      this.failures.set(key, { n, until: this.clock() + Math.min((this.o.intervalMs ?? PR_POLL_MS) * 2 ** (n - 1), MAX_BACKOFF_MS) });
      this.o.svc.store.markMoveChecked(m.task_id, m.generation, msg.slice(0, 300));
      this.o.log?.(`pr poll ${m.url}: ${msg}`);
    }
  }

  /** One pass over every due open or claimed pr move, two at a time. Never throws. */
  async pollOnce(): Promise<void> {
    const now = this.clock();
    const due = this.live().filter((m) => (this.failures.get(`${m.task_id}#${m.generation}`)?.until ?? 0) <= now);
    let i = 0;
    const worker = async () => {
      while (i < due.length) await this.one(due[i++]!);
    };
    await Promise.all(Array.from({ length: Math.min(CONCURRENCY, due.length) }, worker));
  }

  /** Polls immediately, then every interval until the signal aborts. `sleep` is injectable. */
  async run(signal: AbortSignal, sleep: (ms: number, signal: AbortSignal) => Promise<void>): Promise<void> {
    while (!signal.aborted) {
      try {
        await this.pollOnce();
      } catch (e) {
        this.o.log?.(`pr poll pass: ${e instanceof Error ? e.message : String(e)}`);
      }
      await sleep(this.o.intervalMs ?? PR_POLL_MS, signal);
    }
  }
}
