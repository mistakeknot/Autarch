// Same-user swap race against the root-pinned writer [A-19] [C-14]. A hooked fs runs a
// swap (a checked directory becomes a symlink to a directory outside the root) before
// the k-th fs call made inside writeRuling, for every k. Nothing may land outside.
import { existsSync, mkdirSync, mkdtempSync, readdirSync, renameSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const hook = vi.hoisted(() => ({
  active: false,
  calls: 0,
  swapAt: -1,
  swap: (() => {}) as () => void,
}));

vi.mock("node:fs", async (importOriginal) => {
  const real = await importOriginal<typeof import("node:fs")>();
  const wrap = <F extends (...a: any[]) => any>(f: F): F =>
    ((...a: any[]) => {
      if (hook.active) {
        hook.calls++;
        if (hook.calls === hook.swapAt) {
          hook.active = false;
          try {
            hook.swap();
          } finally {
            hook.active = true;
          }
        }
      }
      return f(...a);
    }) as F;
  return {
    ...real,
    lstatSync: wrap(real.lstatSync),
    openSync: wrap(real.openSync),
    mkdirSync: wrap(real.mkdirSync),
    renameSync: wrap(real.renameSync),
    readFileSync: wrap(real.readFileSync),
    writeSync: wrap(real.writeSync),
    fstatSync: wrap(real.fstatSync),
    readlinkSync: wrap(real.readlinkSync),
    unlinkSync: wrap(real.unlinkSync),
    fsyncSync: wrap(real.fsyncSync),
  };
});

const { pinRoot, renderRuling, rulingPath, writeRuling } = await import("../ruling.js");

let base: string;
let root: string;
let outside: string;
beforeEach(() => {
  base = mkdtempSync(join(tmpdir(), "autarch-ruling-race-"));
  root = join(base, "project");
  outside = join(base, "outside");
  mkdirSync(root);
  mkdirSync(outside);
});
afterEach(() => {
  hook.active = false;
  rmSync(base, { recursive: true, force: true });
});

const target = { scope: "project" as const, decision_id: "dec-1", subject: "Which store?", date: "2026-09-29" };
const ruling = {
  decision_id: "dec-1",
  pick_id: "p",
  revision: "r",
  asking_thread: "t",
  subject: "Which store?",
  options_shown: [{ id: "a", label: "A" }],
  picked: "a",
  ruled_by: "mk" as const,
  ruled_at: "2026-09-29T10:00:00.000Z",
  ruling: "Use A.",
  source: "home",
};

function attempt(swapAt: number, which: "docs" | "decisions"): void {
  const p = rulingPath(target);
  const pinned = pinRoot(root);
  const victim = which === "docs" ? join(root, "docs") : join(root, "docs", "decisions");
  hook.calls = 0;
  hook.swapAt = swapAt;
  hook.swap = () => {
    if (!existsSync(victim)) return;
    renameSync(victim, `${victim}-moved`);
    symlinkSync(outside, victim);
  };
  hook.active = true;
  try {
    writeRuling(pinned, p.dirs, p.file, renderRuling(ruling));
  } catch {
    /* refusing is allowed; writing outside is not */
  } finally {
    hook.active = false;
  }
}

describe("a directory swapped for a symlink between steps", () => {
  it("never lands a file outside the root, whichever step the swap follows", () => {
    // measure how many fs calls one clean write makes
    hook.calls = 0;
    hook.swapAt = -1;
    hook.active = true;
    const p = rulingPath(target);
    writeRuling(pinRoot(root), p.dirs, p.file, "probe");
    hook.active = false;
    const total = hook.calls;
    expect(total).toBeGreaterThan(3);
    rmSync(join(root, "docs"), { recursive: true });

    for (const which of ["docs", "decisions"] as const) {
      for (let k = 1; k <= total + 2; k++) {
        rmSync(root, { recursive: true, force: true });
        rmSync(outside, { recursive: true, force: true });
        mkdirSync(root);
        mkdirSync(outside);
        // pre-create so the swap has something to replace at every step
        mkdirSync(join(root, "docs", "decisions"), { recursive: true });
        attempt(k, which);
        expect(readdirSync(outside), `swap ${which} before fs call ${k}`).toEqual([]);
      }
    }
  });
});

// A held directory fd can be renamed OUT of the project by a same-uid process; writes
// through the fd then follow it. The writer must notice and clean up. [C-14]
function listAll(dir: string): string[] {
  const out: string[] = [];
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, e.name);
    if (e.isDirectory()) out.push(...listAll(p));
    else out.push(p);
  }
  return out;
}

describe("an identical existing file", () => {
  it("is not reported done when docs/decisions was moved outside after its fd opened", () => {
    const p = rulingPath(target);
    const content = renderRuling(ruling);
    writeRuling(pinRoot(root), p.dirs, p.file, content); // the file exists, identical
    const victim = join(root, "docs", "decisions");
    // count fs calls up to and including the existing-file open, then move right after
    hook.calls = 0;
    hook.swapAt = -1;
    hook.active = true;
    writeRuling(pinRoot(root), p.dirs, p.file, content);
    hook.active = false;
    const total = hook.calls;
    let sawRefusal = false;
    for (let k = 1; k <= total; k++) {
      rmSync(root, { recursive: true, force: true });
      rmSync(outside, { recursive: true, force: true });
      mkdirSync(root);
      mkdirSync(outside);
      writeRuling(pinRoot(root), p.dirs, p.file, content);
      hook.calls = 0;
      hook.swapAt = k;
      hook.swap = () => {
        if (existsSync(victim)) renameSync(victim, join(outside, "moved"));
      };
      hook.active = true;
      let done = false;
      try {
        writeRuling(pinRoot(root), p.dirs, p.file, content);
        done = true;
      } catch {
        sawRefusal = true;
      } finally {
        hook.active = false;
      }
      const moved = !existsSync(join(victim, p.file));
      expect(done && moved, `moved before fs call ${k} yet reported done`).toBe(false);
    }
    expect(sawRefusal).toBe(true);
  });
});

describe("a directory moved out of the project between steps", () => {
  it("leaves no ruling or temp file outside the root, whichever step the move precedes", () => {
    hook.calls = 0;
    hook.swapAt = -1;
    hook.active = true;
    const p = rulingPath(target);
    writeRuling(pinRoot(root), p.dirs, p.file, "probe");
    hook.active = false;
    const total = hook.calls;
    rmSync(join(root, "docs"), { recursive: true });
    for (const which of ["docs", "decisions"] as const) {
      for (let k = 1; k <= total + 2; k++) {
        rmSync(root, { recursive: true, force: true });
        rmSync(outside, { recursive: true, force: true });
        mkdirSync(root);
        mkdirSync(outside);
        mkdirSync(join(root, "docs", "decisions"), { recursive: true });
        const victim = which === "docs" ? join(root, "docs") : join(root, "docs", "decisions");
        hook.calls = 0;
        hook.swapAt = k;
        hook.swap = () => {
          if (existsSync(victim)) renameSync(victim, join(outside, "moved"));
        };
        hook.active = true;
        try {
          writeRuling(pinRoot(root), p.dirs, p.file, renderRuling(ruling));
        } catch {
          /* refusing is allowed */
        } finally {
          hook.active = false;
        }
        expect(listAll(outside), `move ${which} before fs call ${k}`).toEqual([]);
      }
    }
  });
});
