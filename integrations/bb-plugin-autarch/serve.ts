// The plugin's link to `autarch serve`: a client that reads the project roots and build. The plugin
// never starts or restarts serve (plan Task 2.7: no process spawning in plugin server code). The token file is read here, server-side only;
// nothing in this file returns the token [H-6].
import { readFileSync } from "node:fs";
import type { ProjectInfo } from "./service.js";

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

export const tokenReader = (path: string) => () => readFileSync(path, "utf8");
