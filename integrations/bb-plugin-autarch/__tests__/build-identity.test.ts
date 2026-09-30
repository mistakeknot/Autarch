import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import { join } from "node:path";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { pluginSettings, preflight, PreflightError, readToken, startOwnServe, statProject, type BuildFile, type OwnServe, type PluginHealth, type Readers } from "../e2e/preflight.js";
import { sourceSha256 } from "../scripts/source-hash.mjs";
import { ServeClient, tokenReader } from "../serve.js";
import { tmpDir } from "./helpers.js";

const PLUGIN_ROOT = join(import.meta.dirname, "..");
const t = tmpDir();
let build: BuildFile;
let own: OwnServe;
let parent = "";
let decoy: Server;
let decoyAddr = "";
const OLD = "b".repeat(64);

/** A stand-in for the loaded plugin's `health` RPC, with the same fields and the same client. */
function fakePlugin(addr: string, tokenFile: string, source: string): () => Promise<PluginHealth> {
  return async () => {
    const serve = new ServeClient({ addr, readToken: tokenReader(tokenFile) });
    let build: unknown = null;
    let projects = null;
    let projects_error: string | null = null;
    try {
      build = (await serve.health()).build ?? null;
    } catch {
      /* serve down */
    }
    try {
      projects = await serve.projects();
    } catch (e) {
      projects_error = e instanceof Error ? e.message : String(e);
    }
    return { source_sha256: source, build: build as PluginHealth["build"], projects, projects_error };
  };
}

function readers(over: Partial<Readers> & { health: Readers["health"] }): Readers {
  return {
    homeSource: async () => build.source_sha256,
    autarchVersion: async () => JSON.parse(execFileSync(build.autarch_path, ["version", "--json"], { encoding: "utf8" })),
    ...over,
  };
}

beforeAll(async () => {
  const out = join(t.dir, "build.json");
  execFileSync("node", [join(PLUGIN_ROOT, "scripts", "build-identity.mjs"), "--out", out, "--repo", PLUGIN_ROOT, "--scratch", join(t.dir, "build"), "--skip-npm-ci"], { stdio: ["ignore", "ignore", "inherit"] });
  build = JSON.parse(readFileSync(out, "utf8"));
  parent = join(t.dir, "projects");
  for (const p of ["alpha", "beta"]) {
    mkdirSync(join(parent, p), { recursive: true });
    execFileSync("git", ["-C", join(parent, p), "init", "-q"]);
  }
  own = await startOwnServe({ autarchPath: build.autarch_path, dir: join(t.dir, "serve"), parent });
  // An older daemon already answering, with a different token file.
  decoy = createServer((req, res) => {
    res.setHeader("content-type", "application/json");
    if (req.url === "/health") return void res.end(JSON.stringify({ status: "ok", build: { exe_sha256: OLD } }));
    res.statusCode = 401;
    res.end("{}");
  });
  await new Promise<void>((r) => decoy.listen(0, "127.0.0.1", r));
  decoyAddr = `127.0.0.1:${(decoy.address() as { port: number }).port}`;
}, 240_000);

afterAll(async () => {
  await own?.stop();
  decoy?.close();
  t.cleanup();
});

describe("build-identity", () => {
  it("records the commit, the plugin source hash, and the built autarch", () => {
    expect(build.commit).toBe(execFileSync("git", ["-C", PLUGIN_ROOT, "rev-parse", "HEAD"], { encoding: "utf8" }).trim());
    expect(build.plugin_dir).toContain(join(t.dir, "build"));
    expect(sourceSha256(build.plugin_dir)).toBe(build.source_sha256);
    expect(JSON.parse(readFileSync(join(build.plugin_dir, "identity.json"), "utf8")).source_sha256).toBe(build.source_sha256);
    expect(build.autarch_sha256).toMatch(/^[0-9a-f]{64}$/);
  });

  it("hashes the tree without identity.json, dist and node_modules", () => {
    const d = join(t.dir, "hash");
    mkdirSync(join(d, "dist"), { recursive: true });
    mkdirSync(join(d, "node_modules"));
    writeFileSync(join(d, "a.ts"), "x");
    const before = sourceSha256(d);
    writeFileSync(join(d, "identity.json"), "{}");
    writeFileSync(join(d, "dist", "app.js"), "y");
    writeFileSync(join(d, "node_modules", "m.js"), "z");
    expect(sourceSha256(d)).toBe(before);
    writeFileSync(join(d, "a.ts"), "xx");
    expect(sourceSha256(d)).not.toBe(before);
  });
});

describe("preflight against the real autarch serve", () => {
  const scratch = () => ["alpha", "beta"].map((n) => statProject(n, join(parent, n)));

  it("passes when plugin, app, serve and autarch all match the build file", async () => {
    const loaded = await preflight(build, scratch(), readers({ health: fakePlugin(own.addr, own.tokenFile, build.source_sha256) }));
    expect(loaded).toEqual({ plugin_source: build.source_sha256, app_source: build.source_sha256, serve_sha256: build.autarch_sha256, autarch_sha256: build.autarch_sha256 });
  });

  it("aborts when the plugin's source is older than the build file", async () => {
    await expect(preflight(build, scratch(), readers({ health: fakePlugin(own.addr, own.tokenFile, OLD) }))).rejects.toThrow(/plugin source/);
  });

  it("aborts when the app's data-home-source is older than the build file", async () => {
    await expect(preflight(build, scratch(), readers({ health: fakePlugin(own.addr, own.tokenFile, build.source_sha256), homeSource: async () => OLD }))).rejects.toThrow(/data-home-source/);
    await expect(preflight(build, scratch(), readers({ health: fakePlugin(own.addr, own.tokenFile, build.source_sha256), homeSource: async () => null }))).rejects.toThrow(PreflightError);
  });

  it("aborts when the connected serve is not the built autarch", async () => {
    await expect(preflight(build, scratch(), readers({ health: fakePlugin(own.addr, own.tokenFile, build.source_sha256) }), )).resolves.toBeTruthy();
    const stale = { ...build, autarch_sha256: OLD };
    await expect(preflight(stale, scratch(), readers({ health: fakePlugin(own.addr, own.tokenFile, build.source_sha256), autarchVersion: async () => ({ sha256: OLD }) }))).rejects.toThrow(/connected serve/);
  });

  it("aborts when autarch version differs from the build file", async () => {
    await expect(preflight(build, scratch(), readers({ health: fakePlugin(own.addr, own.tokenFile, build.source_sha256), autarchVersion: async () => ({ sha256: OLD }) }))).rejects.toThrow(/autarch version/);
  });

  it("points the plugin at the harness's own serve and token, not a decoy on another port or the default token", async () => {
    const s = pluginSettings(own, build.autarch_path);
    expect(s.serveAddr).toBe(own.addr);
    expect(s.serveAddr).not.toBe(decoyAddr);
    expect(s.serveAddr).not.toBe("127.0.0.1:8110");
    expect(String(s.serveTokenFile)).toBe(own.tokenFile);
    expect(String(s.serveTokenFile)).not.toContain(".autarch/serve.token");
    expect(s.serveProjectDirs).toEqual([parent]);
    expect(s.autarchBin).toBe(build.autarch_path);
    expect(readToken(own.tokenFile).length).toBeGreaterThan(10);
    // A plugin still pointed at the older decoy fails on the decoy's identity.
    const otherToken = join(t.dir, "other.token");
    writeFileSync(otherToken, "not-the-token\n");
    await expect(preflight(build, scratch(), readers({ health: fakePlugin(decoyAddr, otherToken, build.source_sha256) }))).rejects.toThrow(PreflightError);
  });

  it("aborts on a 401 from the serve", async () => {
    const wrong = join(t.dir, "wrong.token");
    writeFileSync(wrong, "wrong-token\n");
    await expect(preflight(build, scratch(), readers({ health: fakePlugin(own.addr, wrong, build.source_sha256) }))).rejects.toThrow(/did not resolve projects.*401/);
  });

  it("aborts when a scratch project root is missing from the serve's listing", async () => {
    mkdirSync(join(t.dir, "elsewhere", "gamma"), { recursive: true });
    const missing = [...scratch(), statProject("gamma", join(t.dir, "elsewhere", "gamma"))];
    await expect(preflight(build, missing, readers({ health: fakePlugin(own.addr, own.tokenFile, build.source_sha256) }))).rejects.toThrow(/does not list scratch project/);
  });

  it("aborts when a scratch root resolves to a different (dev, ino)", async () => {
    const [a, b] = scratch();
    await expect(preflight(build, [{ ...a!, ino: Number(a!.ino) + 1 }, b!], readers({ health: fakePlugin(own.addr, own.tokenFile, build.source_sha256) }))).rejects.toThrow(/resolves to/);
  });
});

describe("Home identity stamp", () => {
  it("the app bundle exposes the identity.json source as data-home-source", async () => {
    const { HOME_SOURCE } = await import("../ui/identity.js");
    const stamp = JSON.parse(readFileSync(join(PLUGIN_ROOT, "identity.json"), "utf8")).source_sha256;
    expect(HOME_SOURCE).toBe(stamp);
  });
});
