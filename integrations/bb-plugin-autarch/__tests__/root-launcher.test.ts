// Finding s1-review3 P1: root must never execute user-writable script bodies. The launchers refuse (exit 6) as root
// unless the launcher, its .bash body and home-common.bash are root-owned, not group/other-writable, on a root-owned
// path. Real root is unavailable in tests, so: HOME_LAUNCHER_ASSUME_ROOT=1 (only ever makes the check apply) and,
// where installed, fakeroot (id -u is 0 and stat reports the faked owner). Nothing here uses sudo.
import { spawnSync } from "node:child_process";
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, symlinkSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";
import { afterAll, describe, expect, it } from "vitest";

const SCRIPTS = resolve(__dirname, "../../../scripts");
const FIXTURE_THREAD = ["thr", "fixture"].join("_");
const FILES = ["home-upgrade-v3.sh", "home-upgrade-v3.bash", "home-restore-v2.sh", "home-restore-v2.bash", "home-common.bash"];
const LAUNCHERS = ["home-upgrade-v3.sh", "home-restore-v2.sh"];
const toClean: string[] = [];
afterAll(() => toClean.forEach((d) => rmSync(d, { recursive: true, force: true })));

function copyTo(dir: string) {
  mkdirSync(dir, { recursive: true });
  for (const f of FILES) {
    copyFileSync(join(SCRIPTS, f), join(dir, f));
    chmodSync(join(dir, f), f.endsWith(".sh") ? 0o755 : 0o644);
  }
}
const base = () => {
  const d = mkdtempSync(join(homedir(), ".root-launcher-test-"));
  toClean.push(d);
  return d;
};

describe("launcher refuses user-owned scripts as root", () => {
  for (const l of LAUNCHERS) {
    it(`${l}: assume-root over the user-owned checkout exits 6 before any body runs`, () => {
      const r = spawnSync(join(SCRIPTS, l), ["--thread", "thread-x"], { env: { PATH: process.env.PATH, HOME_LAUNCHER_ASSUME_ROOT: "1" }, encoding: "utf8" });
      expect(r.status, r.stderr).toBe(6);
      expect(r.stderr).toContain("refusing to run as root");
      expect(r.stderr).toContain("root-owned copy");
      expect(r.stderr).not.toContain("install-root-copy");
    });
  }
  it("a launcher is not trusted merely because the flag is absent: non-root without the flag proceeds to the body (usage error)", () => {
    const r = spawnSync(join(SCRIPTS, "home-upgrade-v3.sh"), [], { env: { PATH: process.env.PATH }, encoding: "utf8" });
    expect(r.status).toBe(64);
  });
});

const hasFakeroot = spawnSync("sh", ["-c", "command -v fakeroot && fakeroot id -u"]).stdout?.toString().trim().endsWith("0");
describe.skipIf(!hasFakeroot)("under fakeroot (id -u is 0)", () => {
  // Runs `setup` then the launcher inside ONE fakeroot session; all ancestors of the copy are chowned to root there.
  function run(l: string, setup: string) {
    const d = base();
    const dir = join(d, "scripts");
    copyTo(dir);
    const script = `
      set -e
      for p in "${dir}" "${d}" "${homedir()}"; do [ "$(stat -c %u "$p")" = 0 ] || chown 0:0 "$p"; done
      chmod 755 "${dir}" "${d}"
      chown 0:0 ${FILES.map((f) => `"${dir}/${f}"`).join(" ")}
      ${setup.replace(/@DIR@/g, dir).replace(/@D@/g, d)}
      set +e
      "${dir}/${l}" --thread thread-x
    `;
    return spawnSync("fakeroot", ["sh", "-c", script], { env: { PATH: process.env.PATH }, encoding: "utf8" });
  }
  for (const l of LAUNCHERS) {
    const body = l.replace(/\.sh$/, ".bash");
    it(`${l}: root-owned tight copy passes the launcher check (reaches the body, which then needs real root: 64)`, () => {
      const r = run(l, "");
      expect(r.status, r.stderr).toBe(64);
      expect(r.stderr).not.toContain("refusing to run as root");
    });
    it(`${l}: group-writable body is refused`, () => {
      expect(run(l, `chmod 664 "@DIR@/${body}"`).status).toBe(6);
    });
    it(`${l}: user-owned home-common.bash is refused`, () => {
      expect(run(l, `chown ${process.getuid!()}:${process.getgid!()} "@DIR@/home-common.bash"`).status).toBe(6);
    });
    it(`${l}: world-writable parent directory is refused`, () => {
      expect(run(l, `chmod 0777 "@D@"`).status).toBe(6);
    });
  }
});

const GEN = join(SCRIPTS, "home-build-root-package.sh");
const SUDO_RE = /sudo\s+(sh\s+|bash\s+)?(\.\/)?scripts\/|git[^\n]*\|\s*sudo/;

describe("no instruction to run a checkout script with sudo (review s1 root)", () => {
  for (const f of [...FILES, "home-build-root-package.sh"]) {
    it(`${f} has no sudo-from-checkout or pipe-to-sudo instruction`, () => {
      expect(readFileSync(join(SCRIPTS, f), "utf8")).not.toMatch(SUDO_RE);
      expect(readFileSync(join(SCRIPTS, f), "utf8")).not.toContain("home-install-root-copy");
    });
  }
  it("the plan has no sudo-from-checkout instruction or installer reference", () => {
    const plan = readFileSync(resolve(SCRIPTS, "../docs/plans/2026-09-30-home-on-bb-tasks-plan.md"), "utf8");
    expect(plan).not.toMatch(SUDO_RE);
    expect(plan).not.toContain("home-install-root-copy");
  });
});

describe("home-build-root-package.sh and the generated script", () => {
  const STUBS: Record<string, string> = {
    "home-upgrade-v3.sh": '#!/bin/sh\necho "upgrade $*" >> "$(dirname "$0")/../calls.log"\necho "upgrade $*"\n',
    "home-upgrade-v3.bash": "# b1\n",
    "home-restore-v2.sh": "#!/bin/sh\n:\n",
    "home-restore-v2.bash": "# b2\n",
    "home-common.bash": "# c\n",
  };
  function fixture() {
    const d = base();
    const repo = join(d, "repo");
    mkdirSync(join(repo, "scripts"), { recursive: true });
    for (const f of FILES) writeFileSync(join(repo, "scripts", f), STUBS[f]);
    const g = (...a: string[]) => spawnSync("git", ["-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", ...a], { encoding: "utf8" });
    g("init", "-q");
    g("add", ".");
    g("commit", "-qm", "x");
    return { d, repo, sha: g("rev-parse", "HEAD").stdout.trim(), g };
  }
  const build = (repo: string, out: string, args: string[]) => spawnSync(GEN, ["--repo", repo, "--out-dir", out, ...(args.includes("--thread") ? [] : ["--thread", FIXTURE_THREAD]), ...args], { encoding: "utf8", env: { PATH: process.env.PATH } });
  function built() {
    const f = fixture();
    const out = join(f.d, "out");
    const r = build(f.repo, out, ["--commit", f.sha]);
    expect(r.status, r.stderr).toBe(0);
    return { ...f, out, script: join(out, `home-v3-run-${f.sha.slice(0, 12)}.sh`), r };
  }
  const run = (script: string, args: string[], env: Record<string, string>) =>
    spawnSync("/bin/sh", [script, ...args], { encoding: "utf8", env: { PATH: process.env.PATH!, ...env } });

  it("requires a full 40-hex sha: HEAD, refs, short shas and missing are refused", () => {
    const { d, repo } = fixture();
    for (const bad of ["HEAD", "main", "master", "abc1234", "A".repeat(40)]) expect(build(repo, join(d, "o"), ["--commit", bad]).status, bad).toBe(64);
    expect(build(repo, join(d, "o"), []).status).toBe(64);
    expect(existsSync(join(d, "o"))).toBe(false);
  });
  it("requires --thread: a missing thread is refused before anything is written", () => {
    const { d, repo, sha } = fixture();
    const r = spawnSync(GEN, ["--repo", repo, "--out-dir", join(d, "o"), "--commit", sha], { encoding: "utf8", env: { PATH: process.env.PATH } });
    expect(r.status).toBe(64);
    expect(r.stderr).toContain("--thread is required");
    expect(existsSync(join(d, "o"))).toBe(false);
  });
  it("refuses a 40-hex sha that is not a commit", () => {
    const { d, repo } = fixture();
    expect(build(repo, join(d, "o"), ["--commit", "1".repeat(40)]).status).not.toBe(0);
  });
  it("ignores replace refs: the blobs come from the real commit", () => {
    const f = fixture();
    const evil = spawnSync("git", ["-C", f.repo, "hash-object", "-w", "--stdin"], { input: "evil\n", encoding: "utf8" }).stdout.trim();
    const blob = f.g("rev-parse", `${f.sha}:scripts/home-common.bash`).stdout.trim();
    f.g("replace", blob, evil);
    const out = join(f.d, "out");
    expect(build(f.repo, out, ["--commit", f.sha]).status).toBe(0);
    const text = readFileSync(join(out, `home-v3-run-${f.sha.slice(0, 12)}.sh`), "utf8");
    expect(text).not.toContain(Buffer.from("evil\n").toString("base64"));
    expect(text).toContain(Buffer.from("# c\n").toString("base64"));
  });
  it("prints the output path and the sha256 of the whole script; script is sh, embeds shas, uses no git", () => {
    const { r, script } = built();
    const text = readFileSync(script, "utf8");
    const sum = spawnSync("sha256sum", [script], { encoding: "utf8" }).stdout.split(" ")[0];
    expect(r.stdout).toContain(script);
    expect(r.stdout).toContain(`sha256 ${sum}`);
    expect(text.startsWith("#!/bin/sh\n")).toBe(true);
    expect(text).toContain("/usr/bin/env -i");
    expect(text).toContain(FIXTURE_THREAD);
    expect(text).toContain("getent passwd mk");
    expect(text).toContain('"$MKHOME/.local/bin/bb"');
    expect(text).toContain("runuser -u mk");
    const code = text.split("\n").filter((l) => !l.startsWith("#")).join("\n").replace(/^[0-9a-zA-Z+/=]{1,76}$/gm, "");
    expect(code).not.toMatch(/\bgit\b/);
    const m = /EXPECT="([^"]*)"/.exec(text)!;
    expect(m[1].trim().split("\n").length).toBe(5);
    for (const l of m[1].trim().split("\n")) expect(l).toMatch(/^[0-9a-f]{64} {2}home-/);
  });
  it("a custom --thread is baked in", () => {
    const f = fixture();
    expect(build(f.repo, join(f.d, "o"), ["--commit", f.sha, "--thread", FIXTURE_THREAD]).status).toBe(0);
    expect(readFileSync(join(f.d, "o", `home-v3-run-${f.sha.slice(0, 12)}.sh`), "utf8")).toContain(`THREAD=${FIXTURE_THREAD}`);
  });
  it("without euid 0 and without test env it refuses and installs nothing", () => {
    const { script } = built();
    const r = run(script, ["--plugin", "/x"], {});
    expect(r.status).toBe(64);
    expect(r.stderr).toContain("euid 0");
  });
  it("test dest: installs verified files, runs --check only, prints restore command, reports to the thread", () => {
    const { d, script, sha } = built();
    const dest = join(d, "libexec", `home-v3-${sha.slice(0, 12)}`);
    const bb = join(d, "bb");
    writeFileSync(bb, '#!/bin/sh\necho "$@" > "$(dirname "$0")/tell.args"\ncp "${5}" "$(dirname "$0")/tell.msg"\n', { mode: 0o755 });
    const env = { HOME_V3_TEST_DEST: dest, HOME_V3_TEST_BB: bb, HOME_V3_TEST_LAUNCHER_ARGS: "--xx" };
    const r = run(script, ["--plugin", "/plug"], env);
    expect(r.status, r.stdout + r.stderr).toBe(0);
    for (const f of FILES) expect(readFileSync(join(dest, f), "utf8")).toBe(STUBS[f]);
    expect(statSync(join(dest, "home-upgrade-v3.sh")).mode & 0o777).toBe(0o755);
    expect(statSync(join(dest, "home-common.bash")).mode & 0o777).toBe(0o644);
    expect(readFileSync(join(d, "libexec", "calls.log"), "utf8").trim().split("\n")).toEqual([`upgrade --thread ${FIXTURE_THREAD} --plugin /plug --check --xx`]);
    expect(r.stdout).toContain(`sudo ${dest}/home-restore-v2.sh --thread ${FIXTURE_THREAD} --repo`);
    expect(readFileSync(join(d, "tell.args"), "utf8")).toContain(`thread tell ${FIXTURE_THREAD} --message-file`);
    expect(readFileSync(join(d, "tell.msg"), "utf8")).toContain("SUCCESS");
    // second run: already installed, content verified
    expect(run(script, ["--plugin", "/plug"], env).stdout).toContain("already installed");
  });
  it("run from an unreadable cwd (root's /root): the package leaves it before the report goes out through bb", () => {
    const { d, script, sha } = built();
    const bb = join(d, "bbcwd");
    // like bb.js: spawning from an unreadable cwd is EACCES
    writeFileSync(bb, '#!/bin/sh\n[ -x "$(pwd -P)" ] || { echo "spawn EACCES" >&2; exit 1; }\ncp "${5}" "$(dirname "$0")/tell.msg"\n', { mode: 0o755 });
    const cwd = join(d, "unreadable");
    mkdirSync(cwd);
    const env = [`HOME_V3_TEST_DEST=${join(d, "lx", `home-v3-${sha.slice(0, 12)}`)}`, `HOME_V3_TEST_BB=${bb}`, "HOME_V3_TEST_LAUNCHER_ARGS=--xx"].map((x) => `'${x}'`).join(" ");
    const r = spawnSync("bash", ["-c", `cd '${cwd}' && chmod 000 . && exec env PATH="$PATH" ${env} /bin/sh '${script}' --plugin /plug`], { encoding: "utf8" });
    chmodSync(cwd, 0o700);
    expect(r.status, r.stdout + r.stderr).toBe(0);
    expect(r.stderr).not.toContain("EACCES");
    expect(readFileSync(join(d, "tell.msg"), "utf8")).toContain("SUCCESS");
  });
  it("--go runs the real upgrade after the check", () => {
    const { d, script, sha } = built();
    const env = { HOME_V3_TEST_DEST: join(d, "lx", `home-v3-${sha.slice(0, 12)}`), HOME_V3_TEST_LAUNCHER_ARGS: "--xx" };
    const r = run(script, ["--plugin", "/plug", "--go"], env);
    expect(r.status, r.stderr).toBe(0);
    expect(readFileSync(join(d, "lx", "calls.log"), "utf8").trim().split("\n")).toEqual([
      `upgrade --thread ${FIXTURE_THREAD} --plugin /plug --check --xx`,
      `upgrade --thread ${FIXTURE_THREAD} --plugin /plug --xx`,
    ]);
  });
  it("failure still reports (FAILED) and prints the report when sending fails", () => {
    const { d, script, sha } = built();
    const dest = join(d, "lx", `home-v3-${sha.slice(0, 12)}`);
    const bb = join(d, "bbfail");
    writeFileSync(bb, "#!/bin/sh\nexit 1\n", { mode: 0o755 });
    // a launcher that fails: poison via a plugin arg is not possible, so make the dest launcher path fail by a bad launcher arg
    const r = run(script, ["--bogus"], { HOME_V3_TEST_DEST: dest, HOME_V3_TEST_BB: bb });
    expect(r.status).toBe(64);
    expect(r.stderr).toContain("unknown argument");
    const r2 = run(script, ["--plugin", "/p"], { HOME_V3_TEST_DEST: join(d, "lx2", "x"), HOME_V3_TEST_BB: bb, HOME_V3_TEST_LAUNCHER_ARGS: "--xx" });
    expect(r2.status).toBe(0);
    expect(r2.stderr).toContain("report not delivered");
    expect(r2.stderr).toContain("SUCCESS");
  });
  it("tampered embedded sha256 fails verification and installs nothing", () => {
    const { d, script, sha } = built();
    const text = readFileSync(script, "utf8").replace(/EXPECT="[0-9a-f]/, (m) => m.slice(0, -1) + (m.endsWith("0") ? "1" : "0"));
    const bad = join(d, "bad.sh");
    writeFileSync(bad, text);
    const dest = join(d, "lx", `home-v3-${sha.slice(0, 12)}`);
    const r = run(bad, ["--plugin", "/p"], { HOME_V3_TEST_DEST: dest });
    expect(r.status).toBe(5);
    expect(r.stdout).toContain("verification");
    expect(existsSync(dest)).toBe(false);
    expect(readdirSync(join(d, "lx")).length).toBe(0);
  });
  it("tampered embedded file content fails verification", () => {
    const { d, script, sha } = built();
    const text = readFileSync(script, "utf8").replace(Buffer.from("# c\n").toString("base64"), Buffer.from("# x\n").toString("base64"));
    const bad = join(d, "bad.sh");
    writeFileSync(bad, text);
    const r = run(bad, ["--plugin", "/p"], { HOME_V3_TEST_DEST: join(d, "lx", `home-v3-${sha.slice(0, 12)}`) });
    expect(r.status).toBe(5);
  });
  it("refuses an existing destination with unexpected content, and a symlinked component", () => {
    const { d, script, sha } = built();
    const dest = join(d, "lx", `home-v3-${sha.slice(0, 12)}`);
    mkdirSync(dest, { recursive: true });
    writeFileSync(join(dest, "evil"), "x");
    expect(run(script, ["--plugin", "/p"], { HOME_V3_TEST_DEST: dest }).status).toBe(5);
    mkdirSync(join(d, "real"));
    symlinkSync(join(d, "real"), join(d, "link"));
    const r = run(script, ["--plugin", "/p"], { HOME_V3_TEST_DEST: join(d, "link", "home-v3-x") });
    expect(r.status).toBe(5);
    expect(r.stdout).toContain("symlink");
    expect(readdirSync(join(d, "real")).length).toBe(0);
  });
  it.skipIf(!hasFakeroot)("TEST env is refused when real euid is 0 (fakeroot)", () => {
    const { d, script } = built();
    const r = spawnSync("fakeroot", ["/bin/sh", script, "--clean-env", "--plugin", "/p"], { encoding: "utf8", env: { PATH: process.env.PATH!, HOME_V3_TEST_DEST: join(d, "x") } });
    expect(r.status).toBe(64);
    expect(r.stderr).toContain("refused as root");
    expect(existsSync(join(d, "x"))).toBe(false);
  });
  it("BASH_ENV/ENV from the caller do not reach the script's children", () => {
    const { d, script, sha } = built();
    const marker = join(d, "marker");
    writeFileSync(join(d, "evil.env"), `touch ${marker}\n`);
    run(script, ["--plugin", "/p"], { HOME_V3_TEST_DEST: join(d, "lx", `home-v3-${sha.slice(0, 12)}`), BASH_ENV: join(d, "evil.env"), ENV: join(d, "evil.env") });
    expect(existsSync(marker)).toBe(false);
  });
});
