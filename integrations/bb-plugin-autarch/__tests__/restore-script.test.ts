// Task 2.8a test 2 (A10, finding r5-4): scripts/home-restore-v2.sh and scripts/home-upgrade-v3.sh against a
// temporary install, in test mode (--test-as-current-user, --bbdata, --build). Never root, never sudo, never the
// live /home/mk/.bb-machines data. The launchers are executed by path, so the kernel runs the #!/bin/sh line.
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, realpathSync, rmSync, statSync, symlinkSync, writeFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { createServer, type Server } from "node:net";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { makeEnv, type Env } from "./service-helpers.js";
import { populateV2, startV2Home, type V2Fixture } from "./v2-home.js";

const SCRIPTS = resolve(__dirname, "../../../scripts");
const RESTORE = join(SCRIPTS, "home-restore-v2.sh");
const UPGRADE = join(SCRIPTS, "home-upgrade-v3.sh");
const THREAD = "thr_test123";

let tmpl: Env;
let fx: V2Fixture;
let tmplDb: string;
let tmplBackup: string;
const SUITE_START = Date.now();
const toClean: string[] = [];
const procs: ChildProcess[] = [];
const servers: Server[] = [];

const sha = (f: string) => createHash("sha256").update(readFileSync(f)).digest("hex");

beforeAll(async () => {
  tmpl = makeEnv(["Autarch"]);
  const v2 = await startV2Home(tmpl);
  fx = await populateV2(tmpl, v2);
  v2.close();
  tmpl.clock.t = Date.now() + 120_000;
  const svc = tmpl.open();
  tmplBackup = (svc.store.db.prepare("SELECT backup_path FROM migration_log").get() as { backup_path: string }).backup_path;
  svc.store.close();
  tmplDb = tmpl.file;
});
afterAll(() => {
  // Reports the scripts leave in /tmp (mktemp as the current user); only this run's files.
  for (const f of readdirSync("/tmp").filter((n) => /^home-(restore|upgrade)-report\./.test(n))) {
    try {
      if (statSync(join("/tmp", f)).mtimeMs >= SUITE_START - 1000) rmSync(join("/tmp", f));
    } catch {
      /* gone */
    }
  }
  for (const p of [tmpl.roots.Autarch!, join(tmpl.roots.Autarch!, "docs", "decisions")]) {
    try {
      chmodSync(p, 0o755);
    } catch {
      /* gone */
    }
  }
  tmpl.cleanup();
});
afterEach(() => {
  for (const p of procs.splice(0)) p.kill("SIGKILL");
  for (const s of servers.splice(0)) s.close();
  for (const d of toClean.splice(0)) rmSync(d, { recursive: true, force: true });
});

const SERVER_JS = `const http=require("http"),fs=require("fs");
fs.openSync(process.argv[2]+"/server.lock","w");
http.createServer((q,r)=>r.end("ok")).listen(0,"127.0.0.1",function(){console.log(this.address().port)});
setInterval(()=>{},1000);
`;
const STUB_BB = `#!/bin/bash
D="$(cd "$(dirname "$0")/../.." && pwd)/stub"; mkdir -p "$D"
n=$(( $(cat "$D/n" 2>/dev/null || echo 0) + 1 )); echo $n > "$D/n"
{ echo "== call $n"; printf 'ARGV'; for a in "$@"; do printf '\\t%s' "$a"; done; echo
  env | sort | grep -E '^(BB_|NODE_ENV=|HOME=|PATH=)' | sed 's/^/ENV /'; } >> "$D/calls.log"
case "$1 $2" in
  "plugin status") cat "$D/status" 2>/dev/null || echo "autarch: running" ;;
  "plugin logs") cat "$D/logs" 2>/dev/null || true ;;
esac
exit 0
`;

interface Install {
  root: string;
  bbdata: string;
  data: string;
  url: string;
  pid: number;
  build: string;
  plugin: string;
  repo: string;
}
type Call = { argv: string[]; env: Record<string, string> };

function writeStub(bbdata: string, body = STUB_BB, name = "bb") {
  mkdirSync(join(bbdata, "npm", "bin"), { recursive: true });
  const f = join(bbdata, "npm", "bin", name);
  writeFileSync(f, body);
  chmodSync(f, 0o755);
  return f;
}

async function install(): Promise<Install> {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "home-scripts-")));
  toClean.push(root);
  const bbdata = join(root, "bbdata");
  const data = join(bbdata, "plugins", "autarch");
  mkdirSync(data, { recursive: true });
  copyFileSync(tmplDb, join(data, "data.db"));
  copyFileSync(tmplBackup, join(data, basename(tmplBackup)));
  writeFileSync(join(data, "data.db-wal"), "");
  writeStub(bbdata);
  const entry = writeStub(bbdata, SERVER_JS, "bb-app");
  const child = spawn("node", [entry, bbdata], { stdio: ["ignore", "pipe", "ignore"] });
  procs.push(child);
  const port = await new Promise<number>((res, rej) => {
    child.stdout!.once("data", (b) => res(Number(String(b).trim())));
    child.once("error", rej);
    child.once("exit", () => rej(new Error("test server exited")));
  });
  const url = `http://127.0.0.1:${port}`;
  const build = join(root, "v2build");
  const plugin = join(build, "integrations", "bb-plugin-autarch");
  mkdirSync(plugin, { recursive: true });
  writeFileSync(join(plugin, "package.json"), "{}");
  const repo = join(root, "repo");
  mkdirSync(repo);
  writeFileSync(join(bbdata, "bb-app-runtime.json"), JSON.stringify({ entryPath: entry, pid: child.pid, serverUrl: url }));
  return { root, bbdata, data, url, pid: child.pid!, build, plugin, repo };
}

const calls = (i: Install): Call[] => {
  const f = join(i.bbdata, "stub", "calls.log");
  if (!existsSync(f)) return [];
  return readFileSync(f, "utf8")
    .split("== call ")
    .slice(1)
    .map((blk) => {
      const lines = blk.split("\n");
      const argv = (lines.find((l) => l.startsWith("ARGV")) ?? "").split("\t").slice(1);
      const env: Record<string, string> = {};
      for (const l of lines.filter((x) => x.startsWith("ENV "))) {
        const kv = l.slice(4);
        env[kv.slice(0, kv.indexOf("="))] = kv.slice(kv.indexOf("=") + 1);
      }
      return { argv, env };
    });
};
const verbs = (i: Install) => calls(i).map((c) => c.argv.slice(0, 2).join(" "));

const TM = ["--test-as-current-user"];
const restoreArgs = (i: Install, extra: string[] = []) => [...TM, "--bbdata", i.bbdata, "--build", i.build, "--thread", THREAD, "--repo", i.repo, ...extra];
const upgradeArgs = (i: Install, extra: string[] = []) => [...TM, "--bbdata", i.bbdata, "--thread", THREAD, "--plugin", i.plugin, ...extra];

function run(script: string, args: string[], env: NodeJS.ProcessEnv = { PATH: process.env.PATH }) {
  const r = spawnSync(script, args, { env, encoding: "utf8", timeout: 60_000 });
  const out = `${r.stdout}${r.stderr}`;
  for (const m of out.matchAll(/(\/tmp\/home-(?:restore|upgrade)-report\.[A-Za-z0-9]+)/g)) toClean.push(m[1]!);
  return { code: r.status, out };
}
const reportOf = (i: Install): string => {
  const tell = calls(i).find((c) => c.argv[0] === "thread" && c.argv[1] === "tell");
  expect(tell, "thread tell was not called").toBeTruthy();
  expect(tell!.argv.slice(0, 4)).toEqual(["thread", "tell", THREAD, "--message-file"]);
  const f = tell!.argv[4]!;
  toClean.push(f);
  return readFileSync(f, "utf8");
};
const movedAside = (i: Install) => readdirSync(i.data).filter((f) => f.startsWith("data.db.v3-"));

describe("launchers", () => {
  for (const name of ["home-restore-v2.sh", "home-upgrade-v3.sh"]) {
    it(`${name}: shebang #!/bin/sh, first executable line is the env -i exec, mode 0755`, () => {
      const lines = readFileSync(join(SCRIPTS, name), "utf8").split("\n");
      expect(lines[0]).toBe("#!/bin/sh");
      const first = lines.slice(1).find((l) => l.trim() !== "" && !l.startsWith("#"))!;
      expect(first).toBe(
        `exec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /bin/bash --noprofile --norc "\${0%/*}/${name.replace(/\.sh$/, ".bash")}" "$@"`,
      );
      expect(statSync(join(SCRIPTS, name)).mode & 0o777).toBe(0o755);
    });
  }
  it("the bodies are not executable on their own", () => {
    for (const b of ["home-restore-v2.bash", "home-upgrade-v3.bash", "home-common.bash"]) expect(statSync(join(SCRIPTS, b)).mode & 0o111, b).toBe(0);
  });
  it("syntax: sh -n on the launchers, bash -n on the bodies", () => {
    for (const s of ["home-restore-v2.sh", "home-upgrade-v3.sh"]) expect(spawnSync("sh", ["-n", join(SCRIPTS, s)]).status, s).toBe(0);
    for (const b of ["home-restore-v2.bash", "home-upgrade-v3.bash", "home-common.bash"]) expect(spawnSync("bash", ["-n", join(SCRIPTS, b)]).status, b).toBe(0);
  });
  const hasShellcheck = spawnSync("sh", ["-c", "command -v shellcheck"]).status === 0;
  it.skipIf(!hasShellcheck)("shellcheck passes on the launchers (sh) and the bodies (bash)", () => {
    for (const s of ["home-restore-v2.sh", "home-upgrade-v3.sh"]) expect(spawnSync("shellcheck", ["-s", "sh", join(SCRIPTS, s)]).status, s).toBe(0);
    for (const b of ["home-restore-v2.bash", "home-upgrade-v3.bash", "home-common.bash"]) expect(spawnSync("shellcheck", ["-s", "bash", join(SCRIPTS, b)]).status, b).toBe(0);
  });
  it("test-mode flags are refused without --test-as-current-user", async () => {
    const i = await install();
    const r = run(RESTORE, ["--bbdata", i.bbdata, "--thread", THREAD, "--repo", i.repo]);
    expect(r.code).toBe(64);
    expect(calls(i)).toEqual([]);
  });
});

describe("hostile ambient settings (finding r5-4)", () => {
  for (const which of ["restore", "upgrade"] as const) {
    it(`${which}: decoy URL, data dir, HOME, PATH, BASH_ENV and ENV have no effect; every call used the pinned stub and env`, async () => {
      const i = await install();
      const decoy = mkdtempSync(join(tmpdir(), "decoy-"));
      toClean.push(decoy);
      let connections = 0;
      const srv = createServer((s) => {
        connections++;
        s.destroy();
      });
      servers.push(srv);
      await new Promise<void>((r) => srv.listen(0, "127.0.0.1", r));
      const decoyPort = (srv.address() as { port: number }).port;
      const marker = (n: string) => join(decoy, n);
      const writer = (n: string) => {
        const f = join(decoy, `${n}.sh`);
        writeFileSync(f, `echo ran > ${marker(n)}\n`);
        return f;
      };
      writeStub(join(decoy, "install"), `#!/bin/sh\necho decoy-bb > ${marker("decoy-bb")}\n`);
      mkdirSync(join(decoy, "pathbin"));
      writeFileSync(join(decoy, "pathbin", "bb"), `#!/bin/sh\necho path-bb > ${marker("path-bb")}\n`);
      chmodSync(join(decoy, "pathbin", "bb"), 0o755);
      mkdirSync(join(decoy, "home"));
      const hostile: NodeJS.ProcessEnv = {
        PATH: `${join(decoy, "pathbin")}:${process.env.PATH}`,
        BB_SERVER_URL: `http://127.0.0.1:${decoyPort}`,
        BB_DATA_DIR: join(decoy, "install"),
        BB_HOST_DAEMON_PORT: String(decoyPort),
        NODE_ENV: "development",
        HOME: join(decoy, "home"),
        BASH_ENV: writer("BASH_ENV"),
        ENV: writer("ENV"),
      };
      const r = which === "restore" ? run(RESTORE, restoreArgs(i), hostile) : run(UPGRADE, upgradeArgs(i), hostile);
      expect(r.code, r.out).toBe(0);
      expect(connections).toBe(0);
      expect(readdirSync(decoy).filter((f) => f.endsWith("-bb") || f === "BASH_ENV" || f === "ENV")).toEqual([]);
      const cs = calls(i);
      expect(cs.length).toBeGreaterThanOrEqual(4);
      for (const c of cs) {
        const bb = Object.keys(c.env).filter((k) => k.startsWith("BB_")).sort();
        expect(bb).toEqual(["BB_DATA_DIR", "BB_SERVER_URL"]);
        expect(c.env.BB_SERVER_URL).toBe(i.url);
        expect(c.env.BB_DATA_DIR).toBe(i.bbdata);
        expect(c.env.NODE_ENV).toBe("production");
        expect(c.env.PATH).toBe("/usr/bin:/bin");
        expect(c.env.HOME).not.toContain("decoy");
      }
    });
  }
});

describe("unverifiable install: exit 6, nothing moved, no bb call, the report path printed", () => {
  const rewrite = (i: Install, patch: Record<string, unknown>) => {
    const f = join(i.bbdata, "bb-app-runtime.json");
    writeFileSync(f, JSON.stringify({ ...JSON.parse(readFileSync(f, "utf8")), ...patch }));
  };
  const cases: [string, (i: Install) => Promise<void> | void][] = [
    ["the runtime pid is dead", (i) => {
      const dead = spawnSync("true");
      void dead;
      const p = spawn("sleep", ["0"]);
      rewrite(i, { pid: p.pid });
      return new Promise<void>((r) => p.once("exit", () => setTimeout(r, 100)));
    }],
    ["the port's LISTEN socket belongs to an unrelated process", async (i) => {
      const other = spawn("node", [join(i.bbdata, "npm", "bin", "bb-app"), i.root], { stdio: ["ignore", "pipe", "ignore"] });
      procs.push(other);
      const port = await new Promise<number>((res) => other.stdout!.once("data", (b) => res(Number(String(b).trim()))));
      rewrite(i, { serverUrl: `http://127.0.0.1:${port}` });
    }],
    ["entryPath resolves outside BBDATA", (i) => {
      const other = join(i.root, "elsewhere-bb-app");
      writeFileSync(other, SERVER_JS);
      rewrite(i, { entryPath: other });
    }],
    ["the runtime file is a symlink", (i) => {
      const f = join(i.bbdata, "bb-app-runtime.json");
      const real = join(i.root, "real-runtime.json");
      copyFileSync(f, real);
      rmSync(f);
      symlinkSync(real, f);
    }],
    ["the server url is not loopback", (i) => rewrite(i, { serverUrl: "http://10.0.0.1:80" })],
  ];
  for (const script of ["restore", "upgrade"] as const) {
    for (const [name, mutate] of cases) {
      it(`${script}: ${name}`, async () => {
        const i = await install();
        await mutate(i);
        const before = sha(join(i.data, "data.db"));
        const r = script === "restore" ? run(RESTORE, restoreArgs(i)) : run(UPGRADE, upgradeArgs(i));
        expect(r.code, r.out).toBe(6);
        expect(calls(i)).toEqual([]);
        expect(movedAside(i)).toEqual([]);
        expect(sha(join(i.data, "data.db"))).toBe(before);
        expect(r.out).toMatch(/report not sent; read \/tmp\/home-(restore|upgrade)-report\./);
      });
    }
  }
});

describe("restore", () => {
  it("happy path: v3 files moved aside not deleted, backup in place 0600, the a9853e2 build runs on it, the thread told", async () => {
    const i = await install();
    const v3sha = sha(join(i.data, "data.db"));
    const r = run(RESTORE, restoreArgs(i));
    expect(r.code, r.out).toBe(0);
    const moved = movedAside(i);
    expect(moved.sort()).toEqual([expect.stringMatching(/^data\.db\.v3-\d{8}T\d{6}Z$/), expect.stringMatching(/^data\.db\.v3-\d{8}T\d{6}Z-wal$/)]);
    expect(sha(join(i.data, moved.find((m) => !m.endsWith("-wal"))!))).toBe(v3sha);
    expect(statSync(join(i.data, "data.db")).mode & 0o777).toBe(0o600);
    expect(sha(join(i.data, "data.db"))).toBe(sha(join(i.data, basename(tmplBackup))));
    expect(verbs(i)).toEqual(["plugin disable", "plugin install", "plugin enable", "plugin status", "plugin logs", "thread tell"]);
    expect(calls(i)[1]!.argv[2]).toBe(i.plugin);
    const report = reportOf(i);
    expect(report).toContain(join(i.data, basename(tmplBackup)));
    for (const m of moved) expect(report).toContain(join(i.data, m));
    expect(report).toMatch(/open asks in the restored DB: 4/);
    expect(report).toContain(`${i.url}, runtime pid ${i.pid}`);
    // The a9853e2 build starts on the restored file and lists the pre-migration open decide asks (steps and machine asks have their own lists, counted by the report).
    const v2 = await startV2Home({ ...tmpl, file: join(i.data, "data.db") } as Env);
    expect(v2.ready()).toBe(true);
    const owed = (await v2.handlers.listAsks(null as never)) as { owed: { id: string }[] };
    expect(owed.owed.map((o) => o.id).sort()).toEqual([fx.decide, fx.mycroft].sort());
    v2.close();
  });

  it("an explicit --backup is used; one outside the data directory is refused (exit 2, nothing touched)", async () => {
    const i = await install();
    const outside = join(i.root, "outside.db");
    copyFileSync(tmplBackup, outside);
    const r = run(RESTORE, restoreArgs(i, ["--backup", outside]));
    expect(r.code, r.out).toBe(2);
    expect(movedAside(i)).toEqual([]);
    expect(verbs(i)).toEqual(["thread tell"]);
    expect(reportOf(i)).toContain("not inside");
  });

  it("a corrupt backup exits 2 and touches nothing; the report still goes out", async () => {
    const i = await install();
    const before = sha(join(i.data, "data.db"));
    writeFileSync(join(i.data, basename(tmplBackup)), "this is not a database, it is long enough to look like a file ".repeat(80));
    const r = run(RESTORE, restoreArgs(i));
    expect(r.code, r.out).toBe(2);
    expect(movedAside(i)).toEqual([]);
    expect(sha(join(i.data, "data.db"))).toBe(before);
    expect(verbs(i)).toEqual(["thread tell"]);
    expect(reportOf(i)).toMatch(/backup failed integrity_check|schema_version/);
  });

  it("a backup that is a v3 database (min_reader_version 3) is refused with exit 2", async () => {
    const i = await install();
    copyFileSync(tmplDb, join(i.data, basename(tmplBackup)));
    const r = run(RESTORE, restoreArgs(i));
    expect(r.code, r.out).toBe(2);
    expect(movedAside(i)).toEqual([]);
    expect(verbs(i)).toEqual(["thread tell"]);
  });

  it("a DB holder that stays exits 3 after re-enabling; nothing moved; the holder is reported", async () => {
    const i = await install();
    const holder = spawn("bash", ["-c", `exec 3<"${join(i.data, "data.db")}"; sleep 60`], { stdio: "ignore" });
    procs.push(holder);
    await new Promise((r) => setTimeout(r, 300));
    const r = run(RESTORE, restoreArgs(i));
    expect(r.code, r.out).toBe(3);
    expect(movedAside(i)).toEqual([]);
    expect(verbs(i)).toEqual(["plugin disable", "plugin enable", "thread tell"]);
    const report = reportOf(i);
    expect(report).toContain("Home still holds the DB");
    expect(report).toContain(String(holder.pid));
    expect(report).toContain("mk-schu.2");
  });

  it("--check verifies and exits 0 without moving anything or stopping Home", async () => {
    const i = await install();
    const before = sha(join(i.data, "data.db"));
    const r = run(RESTORE, restoreArgs(i, ["--check"]));
    expect(r.code, r.out).toBe(0);
    expect(movedAside(i)).toEqual([]);
    expect(sha(join(i.data, "data.db"))).toBe(before);
    expect(verbs(i)).toEqual(["thread tell"]);
  });

  it("a plugin that never reports healthy exits 5, with the moved paths and the undo steps in the report", async () => {
    const i = await install();
    mkdirSync(join(i.bbdata, "stub"), { recursive: true });
    writeFileSync(join(i.bbdata, "stub", "status"), "autarch: failed to start\n");
    const r = run(RESTORE, restoreArgs(i));
    expect(r.code, r.out).toBe(5);
    const report = reportOf(i);
    expect(report).toContain("did not report a healthy start");
    expect(report).toContain("To undo");
    expect(movedAside(i).length).toBe(2);
  }, 30_000);
});

describe("upgrade", () => {
  it("happy path: disable, install, enable in that order; the backup path comes from migration_log; the thread told", async () => {
    const i = await install();
    const r = run(UPGRADE, upgradeArgs(i));
    expect(r.code, r.out).toBe(0);
    expect(verbs(i).slice(0, 3)).toEqual(["plugin disable", "plugin install", "plugin enable"]);
    expect(verbs(i).at(-1)).toBe("thread tell");
    expect(calls(i)[1]!.argv[2]).toBe(i.plugin);
    expect(reportOf(i)).toContain(tmplBackup);
  });

  it("QuiesceRequiredError in the log: the plugin is left disabled and the error is reported (exit 5)", async () => {
    const i = await install();
    mkdirSync(join(i.bbdata, "stub"), { recursive: true });
    writeFileSync(join(i.bbdata, "stub", "logs"), "autarch: QuiesceRequiredError: another connection holds the file\n");
    const r = run(UPGRADE, upgradeArgs(i));
    expect(r.code, r.out).toBe(5);
    const vs = verbs(i);
    expect(vs.slice(0, 3)).toEqual(["plugin disable", "plugin install", "plugin enable"]);
    expect(vs.at(-2)).toBe("plugin disable");
    expect(reportOf(i)).toMatch(/QuiesceRequiredError[\s\S]*left disabled/);
  });

  it("a DB holder exits 3 after re-enabling, before anything is installed", async () => {
    const i = await install();
    const holder = spawn("bash", ["-c", `exec 3<"${join(i.data, "data.db")}"; sleep 60`], { stdio: "ignore" });
    procs.push(holder);
    await new Promise((r) => setTimeout(r, 300));
    const r = run(UPGRADE, upgradeArgs(i));
    expect(r.code, r.out).toBe(3);
    expect(verbs(i)).toEqual(["plugin disable", "plugin enable", "thread tell"]);
  });

  it("a missing plugin build exits 4 before the plugin is stopped", async () => {
    const i = await install();
    rmSync(join(i.plugin, "package.json"));
    const r = run(UPGRADE, upgradeArgs(i));
    expect(r.code, r.out).toBe(4);
    expect(verbs(i)).toEqual(["thread tell"]);
  });
});
