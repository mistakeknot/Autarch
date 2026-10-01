import { spawn, type ChildProcess } from "node:child_process";
import { mkdirSync, symlinkSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { afterAll, describe, expect, it } from "vitest";
import { checkRuntimeFile, listeners, OwnershipError, ownedSet, parseProcNetTcp, proveSocket, requireNetns, socketHolders } from "../e2e/ownership.js";
import { tmpDir } from "./helpers.js";

const t = tmpDir();
afterAll(() => t.cleanup());

const HEAD = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnmt   uid  timeout inode\n";
const row = (port: number, st: string, inode: number) => `   0: 0100007F:${port.toString(16).toUpperCase().padStart(4, "0")} 00000000:0000 ${st} 00000000:00000000 00:00000000 00000000  1000        0 ${inode} 1 0000000000000000 100 0 0 10 0\n`;

describe("/proc parsing", () => {
  it("reads port, state and inode", () => {
    const e = parseProcNetTcp(HEAD + row(8080, "0A", 4242) + row(9000, "01", 7));
    expect(e).toEqual([{ port: 8080, state: "0A", inode: 4242 }, { port: 9000, state: "01", inode: 7 }]);
  });
  it("keeps only LISTEN entries from tcp and tcp6", () => {
    const proc = join(t.dir, "p1");
    mkdirSync(join(proc, "net"), { recursive: true });
    writeFileSync(join(proc, "net", "tcp"), HEAD + row(1, "0A", 1) + row(2, "01", 2));
    writeFileSync(join(proc, "net", "tcp6"), HEAD + row(3, "0A", 3));
    expect(listeners(proc).map((e) => e.port)).toEqual([1, 3]);
  });
});

describe("requireNetns on fixtures", () => {
  function proc(name: string, own: string, tcp = HEAD): string {
    const p = join(t.dir, name);
    mkdirSync(join(p, "self", "ns"), { recursive: true });
    mkdirSync(join(p, "net"), { recursive: true });
    symlinkSync(own, join(p, "self", "ns", "net"));
    writeFileSync(join(p, "net", "tcp"), tcp);
    return p;
  }
  it("refuses when the outer namespace is unknown", () => {
    expect(() => requireNetns({}, proc("a", "net:[2]"))).toThrow(OwnershipError);
  });
  it("refuses when the namespace equals the outer one", () => {
    expect(() => requireNetns({ HOME_E2E_OUTER_NETNS: "net:[2]" }, proc("b", "net:[2]"))).toThrow(/outer network namespace/);
  });
  it("refuses when a LISTEN socket already exists", () => {
    expect(() => requireNetns({ HOME_E2E_OUTER_NETNS: "net:[1]" }, proc("c", "net:[2]", HEAD + row(5000, "0A", 9)))).toThrow(/LISTEN socket already exists/);
  });
  it("accepts an empty, different namespace", () => {
    expect(requireNetns({ HOME_E2E_OUTER_NETNS: "net:[1]" }, proc("d", "net:[2]"))).toBe(true);
  });
});

describe("runtime file", () => {
  const dir = (body: unknown) => {
    const d = join(t.dir, `rt-${Math.random().toString(36).slice(2)}`);
    mkdirSync(d);
    writeFileSync(join(d, "bb-app-runtime.json"), JSON.stringify(body));
    return d;
  };
  it("accepts the launcher pid and chosen port", () => expect(checkRuntimeFile(dir({ pid: 5, serverUrl: "http://127.0.0.1:4100" }), 5, 4100)).toBe(5));
  it("rejects another pid", () => expect(() => checkRuntimeFile(dir({ pid: 6, serverUrl: "http://127.0.0.1:4100" }), 5, 4100)).toThrow(/not the launcher pid/));
  it("rejects another port", () => expect(() => checkRuntimeFile(dir({ pid: 5, serverUrl: "http://127.0.0.1:4101" }), 5, 4100)).toThrow(/not the chosen port/));
  it("rejects a missing file", () => expect(() => checkRuntimeFile(join(t.dir), 5, 4100)).toThrow(/cannot read/));
});

describe("a real process tree", () => {
  const kids: ChildProcess[] = [];
  afterAll(() => kids.forEach((k) => k.kill("SIGKILL")));
  const LISTEN = `require("node:net").createServer().listen(0,"127.0.0.1",function(){console.log(this.address().port)});setInterval(()=>{},1000)`;

  /** A launcher that spawns a grandchild which holds the listener; resolves with the launcher pid and port. */
  async function tree(): Promise<{ launcher: number; port: number }> {
    const launcher = spawn(process.execPath, ["-e", `const c=require("node:child_process").spawn(process.execPath,["-e",${JSON.stringify(LISTEN)}],{stdio:["ignore","inherit","inherit"]});setInterval(()=>{},1000)`], { stdio: ["ignore", "pipe", "inherit"] });
    kids.push(launcher);
    const port = await new Promise<number>((res) => launcher.stdout!.once("data", (d) => res(Number(String(d).trim()))));
    return { launcher: launcher.pid!, port };
  }

  it("proves a listener held by a descendant of the launcher", async () => {
    const { launcher, port } = await tree();
    const p = proveSocket(launcher, port);
    expect(p.listen_inode_in_owned_set).toBe(true);
    expect(p.owned_set).toContain(launcher);
    expect(p.owned_set.length).toBeGreaterThanOrEqual(2);
    expect(p.owned_set).toContain(p.socket_owner_pid);
    expect(p.socket_owner_pid).not.toBe(launcher);
    expect(socketHolders(p.owned_set).has(p.listen_inode)).toBe(true);
  });

  it("refuses a listener the launcher's tree does not hold", async () => {
    const { port } = await tree();
    const other = spawn("sleep", ["30"]);
    kids.push(other);
    expect(() => proveSocket(other.pid!, port)).toThrow(/held by no process of the owned set/);
    expect(ownedSet(other.pid!)).toEqual([other.pid]);
  });

  it("refuses a port with no listener", () => {
    expect(() => proveSocket(process.pid, 1)).toThrow(/no LISTEN socket/);
  });
});
