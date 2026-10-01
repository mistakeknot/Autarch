// Finding s1-review3 P1: root must never execute mk-writable script bodies. The launchers refuse (exit 6) as root
// unless the launcher, its .bash body and home-common.bash are root-owned, not group/other-writable, on a root-owned
// path. Real root is unavailable in tests, so: HOME_LAUNCHER_ASSUME_ROOT=1 (only ever makes the check apply) and,
// where installed, fakeroot (id -u is 0 and stat reports the faked owner). Nothing here uses sudo.
import { spawnSync } from "node:child_process";
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";
import { afterAll, describe, expect, it } from "vitest";

const SCRIPTS = resolve(__dirname, "../../../scripts");
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

describe("launcher refuses mk-owned scripts as root", () => {
  for (const l of LAUNCHERS) {
    it(`${l}: assume-root over the mk-owned checkout exits 6 before any body runs`, () => {
      const r = spawnSync(join(SCRIPTS, l), ["--thread", "thr_x"], { env: { PATH: process.env.PATH, HOME_LAUNCHER_ASSUME_ROOT: "1" }, encoding: "utf8" });
      expect(r.status, r.stderr).toBe(6);
      expect(r.stderr).toContain("refusing to run as root");
      expect(r.stderr).toContain("home-install-root-copy.sh");
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
      "${dir}/${l}" --thread thr_x
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
    it(`${l}: mk-owned home-common.bash is refused`, () => {
      expect(run(l, `chown ${process.getuid!()}:${process.getgid!()} "@DIR@/home-common.bash"`).status).toBe(6);
    });
    it(`${l}: world-writable parent directory is refused`, () => {
      expect(run(l, `chmod 0777 "@D@"`).status).toBe(6);
    });
  }
});

describe("home-install-root-copy.sh (test mode, temp git repo)", () => {
  function repo() {
    const d = base();
    copyTo(join(d, "scripts"));
    copyFileSync(join(SCRIPTS, "home-install-root-copy.sh"), join(d, "scripts", "home-install-root-copy.sh"));
    chmodSync(join(d, "scripts", "home-install-root-copy.sh"), 0o755);
    const g = (...a: string[]) => spawnSync("git", ["-C", d, "-c", "user.name=t", "-c", "user.email=t@t", ...a], { encoding: "utf8" });
    g("init", "-q");
    g("add", ".");
    g("commit", "-qm", "x");
    return { d, head: g("rev-parse", "HEAD").stdout.trim() };
  }
  const go = (d: string, extra: string[] = []) =>
    spawnSync(join(d, "scripts", "home-install-root-copy.sh"), ["--test-as-current-user", "--dest", join(d, "out", "home-v3"), ...extra], { env: { PATH: process.env.PATH }, encoding: "utf8" });
  it("installs the five verified files and prints the exact next commands", () => {
    const { d, head } = repo();
    const r = go(d, ["--expect-head", head]);
    expect(r.status, r.stdout + r.stderr).toBe(0);
    for (const f of FILES) expect(readFileSync(join(d, "out", "home-v3", f), "utf8")).toBe(readFileSync(join(SCRIPTS, f), "utf8"));
    expect(statSync(join(d, "out", "home-v3", "home-upgrade-v3.sh")).mode & 0o777).toBe(0o755);
    expect(statSync(join(d, "out", "home-v3", "home-common.bash")).mode & 0o777).toBe(0o644);
    expect(r.stdout).toContain(`sudo ${join(d, "out", "home-v3")}/home-upgrade-v3.sh --thread`);
    expect(r.stdout).toContain("home-restore-v2.sh --thread");
  });
  it("refuses when a working-tree file differs from the git HEAD blob; installs nothing", () => {
    const { d } = repo();
    writeFileSync(join(d, "scripts", "home-common.bash"), "# tampered\n", { flag: "a" });
    const r = go(d);
    expect(r.status).toBe(7);
    expect(r.stdout).toContain("MISMATCH home-common.bash");
    expect(existsSync(join(d, "out", "home-v3"))).toBe(false);
  });
  it("refuses an unexpected HEAD", () => {
    const { d } = repo();
    expect(go(d, ["--expect-head", "0".repeat(40)]).status).toBe(7);
  });
});
