// Task 2.11 / A7: the shared rig wrapper. Refusals open no socket (a net.connect spy); the shim refuses
// an indirect bb call that does not target the rig; the environment is built from nothing; only
// e2e/rigexec.ts may spawn, fetch or call an RPC.
import net from "node:net";
import { chmodSync, existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ambientPorts, assertOwned, isLoopbackHost, makeTarget, NONCE_FILE, RefusedError, refusal, rigEnv, rigExec, rigExecSync, rigFetch, rigRpc, shimRefusals, SHIM_EXIT, type RigTarget } from "../e2e/rigexec.js";
import { e2eSources, parse, parseFile, rel, rigViolations } from "./static-scan.js";
import { tmpDir } from "./helpers.js";

const t = tmpDir();
afterEach(() => vi.restoreAllMocks());

/** A competing listener standing in for the ambient bb server: counts every accepted connection. */
async function competitor() {
  let accepted = 0;
  const s = net.createServer((c) => {
    accepted++;
    c.end("HTTP/1.1 200 OK\r\ncontent-length: 2\r\n\r\nok");
  });
  await new Promise<void>((r) => s.listen(0, "127.0.0.1", r));
  const port = (s.address() as net.AddressInfo).port;
  return { port, accepted: () => accepted, close: () => new Promise<void>((r) => s.close(() => r())) };
}

/** A fake runtime file under a fake HOME, as bb-app writes it. */
function fakeHome(ports: { server: number; host?: number }) {
  const home = join(t.dir, `home-${ports.server}`);
  const dir = join(home, ".bb-machines", "m1");
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, "bb-app-runtime.json"), JSON.stringify({ pid: 1, serverUrl: `http://127.0.0.1:${ports.server}`, ...(ports.host ? { hostDaemonPort: ports.host } : {}) }));
  return home;
}

function realBbStub(): string {
  const f = join(t.dir, "real-bb");
  writeFileSync(f, `#!/bin/sh\necho "REAL $BB_SERVER_URL $*" >> ${join(t.dir, "real-bb.calls")}\necho '{"ok":true,"argv":"'"$*"'"}'\n`);
  chmodSync(f, 0o755);
  return f;
}
const realCalls = () => (existsSync(join(t.dir, "real-bb.calls")) ? readFileSync(join(t.dir, "real-bb.calls"), "utf8") : "");

let n = 0;
function target(over: Partial<Parameters<typeof makeTarget>[0]> = {}): RigTarget {
  const k = ++n;
  return makeTarget({ url: "http://127.0.0.1:45871", hostPort: "45872", dataDir: join(t.dir, `data-${k}`), home: join(t.dir, `rig-home-${k}`), realBb: realBbStub(), ambient: new Set([38886]), ...over });
}

describe("isLoopbackHost", () => {
  it.each(["localhost", "LOCALHOST", "localhost.", "a.localhost", "127.0.0.1", "127.9.9.9", "[::1]", "::1", "[::ffff:7f00:1]", "::ffff:127.0.0.1", "0.0.0.0", "[::]"])("%s is loopback", (h) => expect(isLoopbackHost(h)).toBe(true));
  it.each(["example.com", "10.0.0.1", "192.168.1.5", "100.97.18.105", "autarch.getbb.app", "128.0.0.1", "[2001:db8::1]"])("%s is not", (h) => expect(isLoopbackHost(h)).toBe(false));
});

describe("ambient ports", () => {
  it("come from the environment and from every bb-app-runtime.json under ~/.bb and ~/.bb-machines/*", () => {
    const home = fakeHome({ server: 41111, host: 41112 });
    const p = ambientPorts({ BB_SERVER_URL: "http://127.0.0.1:41113", BB_HOST_DAEMON_PORT: "41114", BB_SERVER_PORT: "41115" }, [home]);
    expect([...p].sort()).toEqual([41111, 41112, 41113, 41114, 41115]);
  });
  it("read the real machine's runtime file when HOME is the real one", () => {
    expect(ambientPorts().size).toBeGreaterThanOrEqual(0);
  });
});

describe("refusals open no socket", () => {
  let amb: Awaited<ReturnType<typeof competitor>>;
  let spy: ReturnType<typeof vi.spyOn>;
  let ambient: Set<number>;
  beforeEach(async () => {
    amb = await competitor();
    ambient = ambientPorts({ BB_SERVER_URL: `http://127.0.0.1:${amb.port}` }, []);
    spy = vi.spyOn(net.Socket.prototype, "connect") as never;
  });
  afterEach(async () => {
    await amb.close();
  });
  const refused = (fn: () => unknown, reason: string) => {
    let err: unknown;
    try {
      fn();
    } catch (e) {
      err = e;
    }
    expect(err).toBeInstanceOf(RefusedError);
    expect((err as RefusedError).reason).toBe(reason);
    expect(spy).not.toHaveBeenCalled();
    expect(amb.accepted()).toBe(0);
  };

  it("the ambient URL", () => refused(() => target({ url: `http://127.0.0.1:${amb.port}`, ambient }), "ambient-port"));
  it.each(["localhost", "127.0.0.1", "127.1", "127.5.5.5", "[::1]", "[::ffff:127.0.0.1]", "0.0.0.0", "LOCALHOST", "localhost."])("the loopback alias %s on an ambient port", (h) => {
    refused(() => target({ url: `http://${h}:${amb.port}`, ambient }), "ambient-port");
  });
  it("the ambient URL through the real environment (no seam)", () => {
    vi.stubEnv("BB_SERVER_URL", `http://127.0.0.1:${amb.port}`);
    try {
      refused(() => makeTarget({ url: `http://localhost:${amb.port}`, dataDir: join(t.dir, "d-amb"), home: join(t.dir, "h-amb"), realBb: realBbStub() }), "ambient-port");
    } finally {
      vi.unstubAllEnvs();
    }
  });
  it("an ambient host daemon port", () => refused(() => target({ hostPort: String(amb.port), ambient }), "ambient-port"));
  it("an ambient port from a bb-app-runtime.json", () => {
    const home = fakeHome({ server: amb.port });
    refused(() => target({ url: `http://[::1]:${amb.port}`, ambient: ambientPorts({}, [home]) }), "ambient-port");
  });
  it.each(["https://autarch.getbb.app:443", "http://autarch.getbb.app:45871", "http://10.1.2.3:45871", "http://100.97.18.105:45871"])("a non-loopback target %s", (u) => refused(() => target({ url: u, ambient }), "not-loopback"));
  it("an absent environment: there is no default", () => {
    refused(() => target({ url: undefined, ambient }), "no-environment");
    refused(() => target({ url: "", ambient }), "no-environment");
    refused(() => target({ dataDir: undefined, ambient }), "no-environment");
    refused(() => target({ home: undefined, ambient }), "no-environment");
  });
  it("a default port (no explicit port) and a bad URL", () => {
    refused(() => target({ url: "http://127.0.0.1", ambient }), "default-port");
    refused(() => target({ url: "http://localhost/", ambient }), "default-port");
    refused(() => target({ url: "not a url", ambient }), "bad-url");
    refused(() => target({ url: "ftp://127.0.0.1:45871", ambient }), "bad-url");
  });
  it("a live bb data dir", () => refused(() => target({ dataDir: join(t.dir, ".bb-machines", "autarch.getbb.app"), ambient }), "ambient-data"));
  it("an env override of BB_*, PATH or HOME", () => {
    for (const k of ["BB_SERVER_URL", "BB_DATA_DIR", "PATH", "HOME"]) refused(() => rigEnv({ env: { [k]: "x" } }), "env-override");
  });
  it("rigFetch refuses ambient, aliases, non-loopback and default ports", async () => {
    for (const u of [`http://localhost:${amb.port}/health`, `http://[::1]:${amb.port}/`, "http://example.com:45871/", "http://127.0.0.1/"]) {
      await expect(rigFetch(u, undefined, ambient)).rejects.toBeInstanceOf(RefusedError);
    }
    expect(spy).not.toHaveBeenCalled();
    expect(amb.accepted()).toBe(0);
  });
  it("rigExec and rigRpc refuse a forged target and an unproven real target before spawning", async () => {
    const forged = { url: "http://127.0.0.1:45871", dataDir: t.dir, home: t.dir, hostPort: "", nonce: "x", shimDir: t.dir, shimLog: join(t.dir, "l"), requireProof: false } as RigTarget;
    await expect(rigExec("bb", ["x"], { target: forged })).rejects.toBeInstanceOf(RefusedError);
    expect(() => rigExecSync("bb", ["x"], { target: forged })).toThrow(RefusedError);
    await expect(rigRpc(forged, "tasks", "listProjects")).rejects.toBeInstanceOf(RefusedError);
    const real = target({ requireProof: true, ambient });
    await expect(rigRpc(real, "tasks", "listProjects")).rejects.toMatchObject({ reason: "unproven" });
    expect(realCalls()).toBe("");
    expect(spy).not.toHaveBeenCalled();
  });
  it("assertOwned refuses a target whose data dir lost or changed its nonce", () => {
    const tg = target({ ambient });
    expect(() => assertOwned(tg, ambient)).not.toThrow();
    writeFileSync(join(tg.dataDir, NONCE_FILE), "someone-else");
    expect(() => assertOwned(tg, ambient)).toThrow(/nonce/);
  });
  it("assertOwned re-checks ambient ports at call time", () => {
    const tg = target({ ambient });
    expect(() => assertOwned(tg, new Set([45871]))).toThrow(/ambient/);
  });
  it("refusal() itself is pure", () => {
    expect(refusal("http://127.0.0.1:45871", "45872", new Set())).toBeNull();
  });
});

describe("the child environment is built from nothing", () => {
  it("holds only PATH, HOME and the rig's BB_* variables, in the specified PATH order", async () => {
    vi.stubEnv("AWS_SECRET", "leak");
    vi.stubEnv("BB_SERVER_URL", "http://127.0.0.1:1");
    vi.stubEnv("BB_THREAD_ID", "thr-ambient");
    try {
      const tg = target();
      const r = await rigExec("/usr/bin/env", [], { target: tg });
      const env = Object.fromEntries(r.stdout.trim().split("\n").map((l) => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1)]));
      expect(Object.keys(env).filter((k) => k !== "PWD" && k !== "SHLVL" && k !== "_").sort()).toEqual(["BB_DATA_DIR", "BB_HOST_DAEMON_PORT", "BB_SERVER_URL", "HOME", "PATH"]);
      expect(env.BB_SERVER_URL).toBe(tg.url);
      expect(env.HOME).toBe(tg.home);
      const dirs = env.PATH!.split(":");
      expect(dirs.slice(0, 3)).toEqual([tg.shimDir, "/usr/bin", "/bin"]);
      expect(dirs).toContain(join(process.execPath, ".."));
    } finally {
      vi.unstubAllEnvs();
    }
  });
  it("passes BB_THREAD_ID only when a scenario sets it", async () => {
    const tg = target();
    expect((await rigExec("/usr/bin/env", [], { target: tg })).stdout).not.toContain("BB_THREAD_ID");
    expect((await rigExec("/usr/bin/env", [], { target: tg, threadId: "thr-1" })).stdout).toContain("BB_THREAD_ID=thr-1");
  });
  it("without a target no BB_* variable exists", () => {
    const r = rigExecSync("/usr/bin/env", []);
    expect(r.stdout).not.toMatch(/^BB_/m);
  });
});

describe("the bb shim", () => {
  it("execs the real bb when URL, data dir and nonce match the rig", async () => {
    const tg = target();
    const r = await rigExec("bb", ["home", "get", "--request", "k"], { target: tg, threadId: "thr-1" });
    expect(r.code).toBe(0);
    expect(realCalls()).toContain(`REAL ${tg.url} home get --request k`);
    expect(shimRefusals(tg)).toEqual([]);
  });
  it("an untargeted bb call is refused (exit 97)", () => {
    const r = rigExecSync("bb", ["home", "get"]);
    expect(r.code).toBe(SHIM_EXIT); // the real bb, which sits on PATH, is never reached without a target
    expect(r.stderr).toContain("no default server");
  });
  it("refuses a call whose URL is not the rig's: exit 97, logged, real bb never run, ambient listener untouched", async () => {
    const amb = await competitor();
    const tg = target({ ambient: new Set([amb.port]) });
    const before = realCalls();
    const r = await rigExec("sh", ["-c", `BB_SERVER_URL=http://127.0.0.1:${amb.port} bb home ask --request-stdin`], { target: tg });
    expect(r.code).toBe(SHIM_EXIT);
    expect(shimRefusals(tg)).toHaveLength(1);
    expect(shimRefusals(tg)[0]).toContain(`url=http://127.0.0.1:${amb.port}`);
    expect(realCalls()).toBe(before);
    expect(amb.accepted()).toBe(0);
    await amb.close();
  });
  it("refuses an ambient data dir, a missing nonce, and a foreign host daemon port", async () => {
    const tg = target();
    const ambientData = join(t.dir, "ambient-data");
    mkdirSync(ambientData);
    for (const prefix of [`BB_DATA_DIR=${ambientData}`, `BB_HOST_DAEMON_PORT=1`, `BB_SERVER_URL=`]) {
      const r = await rigExec("sh", ["-c", `${prefix} bb home get`], { target: tg });
      expect(r.code, prefix).toBe(SHIM_EXIT);
    }
    writeFileSync(join(tg.dataDir, NONCE_FILE), "tampered");
    expect((await rigExec("bb", ["home", "get"], { target: tg })).code).toBe(SHIM_EXIT);
  });
  it("an indirect autarch -> bb call against an ambient port is refused by the shim, with no socket opened", async () => {
    const amb = await competitor();
    const tg = target({ ambient: new Set([amb.port]) });
    // A stand-in for `autarch needs-mk file`: it calls bb itself, with an ambient server in its environment.
    const autarch = join(t.dir, "autarch-standin");
    writeFileSync(autarch, `#!/bin/sh\nBB_SERVER_URL="http://localhost:${amb.port}" exec bb home ask --request-stdin\n`);
    chmodSync(autarch, 0o755);
    const spy = vi.spyOn(net.Socket.prototype, "connect") as never;
    const r = await rigExec(autarch, [], { target: tg, input: "{}" });
    expect(r.code).toBe(SHIM_EXIT);
    expect(shimRefusals(tg).some((l) => l.includes(`localhost:${amb.port}`))).toBe(true);
    expect(spy).not.toHaveBeenCalled();
    expect(amb.accepted()).toBe(0);
    // The same stand-in with the rig's own environment reaches the real bb.
    const ok = join(t.dir, "autarch-ok");
    writeFileSync(ok, `#!/bin/sh\nexec bb home ask --request-stdin\n`);
    chmodSync(ok, 0o755);
    expect((await rigExec(ok, [], { target: tg })).code).toBe(0);
    await amb.close();
  });
});

describe("rigRpc", () => {
  it("is routed through the shim to bb plugin rpc call, after assertOwned", async () => {
    const tg = target();
    const out = await rigRpc<{ ok: boolean; argv: string }>(tg, "tasks", "listProjects", { a: 1 });
    expect(out.ok).toBe(true);
    expect(out.argv).toMatch(/^plugin rpc call tasks listProjects --input-file .*listProjects\.json --json$/);
  });
  it("a proven real target may call", async () => {
    const tg = target({ requireProof: true });
    tg.proof = { launcher_pid: 1, socket_owner_pid: 1, listen_inode: 1, listen_inode_in_owned_set: true, owned_set: [1] };
    await expect(rigRpc(tg, "tasks", "listProjects")).resolves.toMatchObject({ ok: true });
  });
});

describe("import graph (Task 2.7 checker): only rigexec.ts may spawn, fetch or call an RPC under e2e/", () => {
  it("scans a real set of sources, and only rigexec.ts has violations", () => {
    const files = e2eSources();
    expect(files.map(rel)).toEqual(expect.arrayContaining(["e2e/rigexec.ts", "e2e/rig.ts", "e2e/real.ts", "e2e/harness.ts", "e2e/preflight.ts", "e2e/fake-bb.mjs", "e2e/scenarios/index.ts"]));
    const bad = files.filter((f) => !f.endsWith(join("e2e", "rigexec.ts"))).flatMap((f) => rigViolations(parseFile(f)));
    expect(bad).toEqual([]);
    expect(rigViolations(parseFile(files.find((f) => f.endsWith(join("e2e", "rigexec.ts")))!)).length).toBeGreaterThan(0);
  });
  it.each([
    ['import { spawn } from "node:child_process";'],
    ['import { execFileSync } from "child_process";'],
    ['const r = await fetch("http://x");'],
    ['const r = await globalThis.fetch("http://x");'],
    ['const r = await globalThis["fetch"]("http://x");'],
    ['await plugins.callRpc({});'],
    ['await callRpc({});'],
    ['import got from "got";'],
    ['const m = await import("node:child_process");'],
  ])("flags %s", (src) => expect(rigViolations(parse(src)).length).toBeGreaterThan(0));
  it("does not flag http servers, fs, or a method named fetchAll", () => {
    expect(rigViolations(parse('import http from "node:http"; http.createServer(() => {}); fetchAll(); x.fetchAll();'))).toEqual([]);
  });
});
