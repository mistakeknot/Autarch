// A7 network isolation (finding r5-3): real-bb runs only inside `bwrap --unshare-net`. These tests start real
// bwrap namespaces; they need no bb server.
import { spawn, spawnSync } from "node:child_process";
import { readlinkSync, writeFileSync } from "node:fs";
import net from "node:net";
import { join } from "node:path";
import { afterAll, describe, expect, it } from "vitest";
import { tmpDir } from "./helpers.js";

const ROOT = join(import.meta.dirname, "..");
const OUTER = readlinkSync("/proc/self/ns/net");
const t = tmpDir();
afterAll(() => t.cleanup());

const bwrapOk = spawnSync("bwrap", ["--dev-bind", "/", "/", "--unshare-net", "--die-with-parent", "true"]).status === 0;
const inBwrap = (args: string[], env: Record<string, string> = {}) =>
  spawnSync("bwrap", ["--dev-bind", "/", "/", "--unshare-net", "--die-with-parent", "--setenv", "HOME_E2E_OUTER_NETNS", OUTER, ...Object.entries(env).flatMap(([k, v]) => ["--setenv", k, v]), ...args], { encoding: "utf8", timeout: 60_000 });

const listen = (host: string, port: number, onConn: () => void): Promise<net.Server> =>
  new Promise((res, rej) => {
    const s = net.createServer((c) => (onConn(), c.destroy()));
    s.once("error", rej);
    s.listen(port, host, () => res(s));
  });

const HARNESS = ["--mode", "real-bb", "--run-id", "r", "--out", "/tmp/never.jsonl", "--build", "/tmp/never-build.json", "--install"];
const tsx = join(ROOT, "node_modules", ".bin", "tsx");

describe.skipIf(!bwrapOk)("rig isolation under bwrap --unshare-net", () => {
  it("has a different network namespace inside", () => {
    const r = inBwrap(["readlink", "/proc/self/ns/net"]);
    expect(r.stdout.trim()).not.toBe("");
    expect(r.stdout.trim()).not.toBe(OUTER);
  });

  it("a competing listener on the rig's exact port (v4 and v6) receives zero connections", async () => {
    // Reserve a port free on both families, then hold it in the outer namespace.
    const probe = await listen("127.0.0.1", 0, () => {});
    const port = (probe.address() as net.AddressInfo).port;
    probe.close();
    let hits = 0;
    const v4 = await listen("127.0.0.1", port, () => hits++);
    const v6 = await listen("::1", port, () => hits++).catch(() => null); // hosts without ::1 still prove v4
    try {
      const script = join(t.dir, "inside.cjs");
      writeFileSync(script, `
        const net = require("node:net");
        const port = ${port};
        const hosts = ["127.0.0.1", "::1"];
        const serve = (h) => new Promise((res) => { const s = net.createServer((c) => c.end("mine")); s.once("error", () => res(null)); s.listen(port, h, () => res(s)); });
        const dial = (h) => new Promise((res) => { const c = net.connect(port, h); let d = ""; c.on("data", (x) => d += x); c.on("error", () => res("error")); c.on("close", () => res(d)); });
        (async () => {
          const out = {};
          for (const h of hosts) { const s = await serve(h); out[h] = s ? await dial(h) : "no-listener"; s && s.close(); }
          process.stdout.write(JSON.stringify(out));
          process.exit(0);
        })();`);
      const r = await new Promise<{ code: number | null; out: string }>((res) => {
        const p = spawn("bwrap", ["--dev-bind", "/", "/", "--unshare-net", "--die-with-parent", process.execPath, script], { stdio: ["ignore", "pipe", "inherit"] });
        let out = "";
        p.stdout.on("data", (d) => (out += d));
        p.on("close", (code) => res({ code, out }));
      });
      expect(r.code).toBe(0);
      const seen = JSON.parse(r.out) as Record<string, string>;
      expect(seen["127.0.0.1"]).toBe("mine"); // the rig's own server, in its own namespace
      expect(hits).toBe(0);
      expect(v6 === null || seen["::1"] === "mine" || seen["::1"] === "no-listener").toBe(true);
    } finally {
      v4.close();
      v6?.close();
    }
  });

  it("the harness refuses outside bwrap (netns equal to HOME_E2E_OUTER_NETNS)", () => {
    const r = spawnSync(tsx, ["e2e/harness.ts", ...HARNESS], { cwd: ROOT, encoding: "utf8", env: { PATH: process.env.PATH, HOME_E2E_OUTER_NETNS: OUTER }, timeout: 60_000 });
    expect(r.status).toBe(2);
    expect(r.stderr).toMatch(/outer network namespace/);
  });

  it("the harness refuses when HOME_E2E_OUTER_NETNS is not recorded", () => {
    const r = inBwrap([tsx, "e2e/harness.ts", ...HARNESS].map((x) => x), { HOME_E2E_OUTER_NETNS: "" });
    expect(r.status).toBe(2);
    expect(r.stderr).toMatch(/HOME_E2E_OUTER_NETNS is not set/);
  });

  it("the harness refuses when a LISTEN socket already exists in the namespace", () => {
    const script = join(t.dir, "preexisting.cjs");
    writeFileSync(script, `
      const net = require("node:net");
      const { spawnSync } = require("node:child_process");
      net.createServer().listen(0, "127.0.0.1", () => {
        const r = spawnSync(${JSON.stringify(tsx)}, ["e2e/harness.ts", ...${JSON.stringify(HARNESS)}], { cwd: ${JSON.stringify(ROOT)}, encoding: "utf8" });
        process.stdout.write(JSON.stringify({ status: r.status, stderr: r.stderr }));
        process.exit(0);
      });`);
    const r = inBwrap([process.execPath, script]);
    const got = JSON.parse(r.stdout) as { status: number; stderr: string };
    expect(got.status).toBe(2);
    expect(got.stderr).toMatch(/LISTEN socket already exists/);
  });

  it("the harness gets past the namespace check inside an empty namespace", () => {
    const r = inBwrap([tsx, "e2e/harness.ts", ...HARNESS, "--owned-server"]);
    expect(r.stderr).not.toMatch(/outer network namespace|LISTEN socket already exists/);
    expect(r.status).toBe(2);
    expect(r.stderr).toMatch(/HOME_E2E_BB_APP|Task 2.12/);
  });
});
