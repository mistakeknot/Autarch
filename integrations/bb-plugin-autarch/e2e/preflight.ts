// The loaded-identity preflight [F-7] [G-7] [H-6] [I-4]: before any scenario the
// harness reads what is actually running and compares it to the build file.
// Any mismatch aborts the run with no scenario lines.
import { rigFetch, rigSpawn } from "./rigexec.js";
import type { ChildProcess } from "node:child_process";
import { mkdirSync, readFileSync, realpathSync, statSync } from "node:fs";
import { createServer } from "node:net";
import { join } from "node:path";
import type { BuildFile } from "../scripts/build-identity.mjs";

export type { BuildFile };

export interface ProjectRoot {
  name: string;
  root: string;
  dev: number | string | bigint;
  ino: number | string | bigint;
}

/** What the plugin's `health` RPC returns (the fields the preflight needs). */
export interface PluginHealth {
  source_sha256: string | null;
  build: { exe_sha256?: string } | null;
  projects: ProjectRoot[] | null;
  projects_error: string | null;
}

export interface Readers {
  /** The loaded plugin's `health` RPC. */
  health(): Promise<PluginHealth>;
  /** `data-home-source` on the Home tab root, from the app bundle's identity.json. */
  homeSource(): Promise<string | null>;
  /** `autarch version --json` of the build file's autarch_path. */
  autarchVersion(): Promise<{ sha256: string }>;
}

export interface Loaded {
  plugin_source: string;
  app_source: string;
  serve_sha256: string;
  autarch_sha256: string;
}

export class PreflightError extends Error {
  constructor(message: string) {
    super(`preflight: ${message}`);
  }
}

export function statProject(name: string, path: string): ProjectRoot {
  const root = realpathSync(path);
  const st = statSync(root);
  return { name, root, dev: st.dev, ino: st.ino };
}

export async function preflight(build: BuildFile, scratch: ProjectRoot[], r: Readers): Promise<Loaded> {
  const h = await r.health();
  if (h.source_sha256 !== build.source_sha256) throw new PreflightError(`plugin source ${h.source_sha256} is not the build's ${build.source_sha256}`);
  const serve = h.build?.exe_sha256;
  if (serve !== build.autarch_sha256) throw new PreflightError(`connected serve ${serve ?? "unknown"} is not the build's autarch ${build.autarch_sha256}`);
  if (h.projects_error) throw new PreflightError(`serve did not resolve projects: ${h.projects_error}`);
  if (!h.projects) throw new PreflightError("serve returned no project roots");
  for (const want of scratch) {
    const got = h.projects.find((p) => p.root === want.root);
    if (!got) throw new PreflightError(`serve does not list scratch project ${want.root}`);
    if (String(got.dev) !== String(want.dev) || String(got.ino) !== String(want.ino)) throw new PreflightError(`scratch project ${want.root} resolves to (${got.dev}, ${got.ino}), not (${want.dev}, ${want.ino})`);
  }
  const app = await r.homeSource();
  if (app !== build.source_sha256) throw new PreflightError(`Home data-home-source ${app} is not the build's ${build.source_sha256}`);
  const v = await r.autarchVersion();
  if (v.sha256 !== build.autarch_sha256) throw new PreflightError(`autarch version ${v.sha256} is not the build's ${build.autarch_sha256}`);
  return { plugin_source: h.source_sha256, app_source: app, serve_sha256: serve, autarch_sha256: v.sha256 };
}

export function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const s = createServer();
    s.once("error", reject);
    s.listen(0, "127.0.0.1", () => {
      const port = (s.address() as { port: number }).port;
      s.close(() => resolve(port));
    });
  });
}

export interface OwnServe {
  addr: string;
  tokenFile: string;
  projectDirs: string[];
  child: ChildProcess;
  stop(): Promise<void>;
}

/**
 * The harness's own `autarch serve`: a free loopback port, a scratch token file and one
 * scratch discovery parent as --project-dir. No default daemon or default token is used [H-6].
 */
export async function startOwnServe(o: { autarchPath: string; dir: string; parent: string; home?: string }): Promise<OwnServe> {
  const port = await freePort();
  const addr = `127.0.0.1:${port}`;
  mkdirSync(o.dir, { recursive: true });
  const tokenFile = join(o.dir, "serve.token");
  const home = o.home ?? join(o.dir, "home");
  mkdirSync(home, { recursive: true });
  const child = rigSpawn(o.autarchPath, ["serve", "--addr", addr, "--token-file", tokenFile, "--project-dir", o.parent], { home, stdout: "ignore" });
  const exited = new Promise<never>((_, reject) => child.once("exit", (c) => reject(new Error(`autarch serve exited (${c}) before it answered`))));
  const up = (async () => {
    for (let i = 0; i < 100; i++) {
      try {
        const res = await rigFetch(`http://${addr}/health`, { signal: AbortSignal.timeout(500) });
        if (res.ok) return;
      } catch {
        /* not up yet */
      }
      await new Promise((r) => setTimeout(r, 100));
    }
    throw new Error("autarch serve did not answer /health");
  })();
  try {
    await Promise.race([up, exited]);
  } catch (e) {
    child.kill("SIGKILL");
    throw e;
  }
  return {
    addr,
    tokenFile,
    projectDirs: [o.parent],
    child,
    stop: () =>
      new Promise<void>((resolve) => {
        if (child.exitCode !== null) return resolve();
        child.once("exit", () => resolve());
        child.kill("SIGTERM");
        setTimeout(() => child.kill("SIGKILL"), 3000).unref();
      }),
  };
}

/** The plugin settings the harness applies so no other daemon or token serves the run. */
export function pluginSettings(s: OwnServe, autarchPath: string): Record<string, unknown> {
  return { serveAddr: s.addr, serveTokenFile: s.tokenFile, serveProjectDirs: s.projectDirs, autarchBin: autarchPath };
}

export const readToken = (file: string) => readFileSync(file, "utf8").trim();
