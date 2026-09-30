// The plugin's link to `autarch serve`: a supervisor that keeps it running and a client
// that reads the project roots and build. The token file is read here, server-side only;
// nothing in this file returns the token [H-6].
import { spawn as nodeSpawn } from "node:child_process";
import { readFileSync } from "node:fs";
import type { ProjectInfo } from "./service.js";

export const MAX_RESTARTS = 3;
export const RESTART_WINDOW_MS = 10 * 60_000;

type FetchLike = (url: string | URL, init?: { headers?: Record<string, string>; signal?: AbortSignal }) => Promise<Response>;

export interface ServeClientOptions {
  addr: string;
  readToken: () => string;
  fetch?: FetchLike;
  timeoutMs?: number;
}

export class ServeClient {
  constructor(private readonly o: ServeClientOptions) {}

  private async get(path: string, auth: boolean): Promise<unknown> {
    const headers: Record<string, string> = {};
    if (auth) headers.Authorization = `Bearer ${this.o.readToken().trim()}`;
    const f = this.o.fetch ?? (globalThis.fetch as FetchLike);
    const res = await f(`http://${this.o.addr}${path}`, { headers, signal: AbortSignal.timeout(this.o.timeoutMs ?? 3000) });
    if (!res.ok) throw new Error(`serve ${path} answered ${res.status}`);
    return res.json();
  }

  async projects(): Promise<ProjectInfo[]> {
    return (await this.get("/api/projects", true)) as ProjectInfo[];
  }

  /** /health needs no token. */
  async health(): Promise<{ build?: unknown } & Record<string, unknown>> {
    return (await this.get("/health", false)) as never;
  }

  async healthy(): Promise<boolean> {
    try {
      await this.get("/health", false);
      return true;
    } catch {
      return false;
    }
  }
}

export interface SupervisorOptions {
  addr: string;
  bin: string;
  tokenFile: string;
  projectDirs: string[];
  health: () => Promise<boolean>;
  spawn?: (bin: string, args: string[]) => void;
  now?: () => number;
}

/** Start `autarch serve` when /health fails: at most 3 starts per 10 minutes, none while it answers. */
export class ServeSupervisor {
  private readonly starts: number[] = [];
  constructor(private readonly o: SupervisorOptions) {}

  args(): string[] {
    return ["serve", "--addr", this.o.addr, "--token-file", this.o.tokenFile, ...this.o.projectDirs.flatMap((d) => ["--project-dir", d])];
  }

  async check(): Promise<"healthy" | "started" | "limited"> {
    if (await this.o.health()) return "healthy";
    const now = (this.o.now ?? Date.now)();
    while (this.starts.length > 0 && now - this.starts[0]! >= RESTART_WINDOW_MS) this.starts.shift();
    if (this.starts.length >= MAX_RESTARTS) return "limited";
    this.starts.push(now);
    (this.o.spawn ?? detached)(this.o.bin, this.args());
    return "started";
  }
}

function detached(bin: string, args: string[]): void {
  const child = nodeSpawn(bin, args, { detached: true, stdio: "ignore" });
  child.on("error", () => {});
  child.unref();
}

export const tokenReader = (path: string) => () => readFileSync(path, "utf8");
