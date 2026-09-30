import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import Database from "better-sqlite3";
import { exportEvents } from "../export.js";
import { decision, openStore, tmpDir } from "./helpers.js";
import { Store } from "../store.js";

let t: ReturnType<typeof tmpDir>;
let file: string;
let root: string;
beforeEach(() => {
  t = tmpDir();
  file = join(t.dir, "data.db");
  root = join(t.dir, "export");
});
afterEach(() => t.cleanup());

function fill(s: Store, n: number) {
  for (let i = 0; i < n; i++) s.insertDecision(decision());
}

function exportedKeys(dir: string): string[] {
  const keys: string[] = [];
  for (const f of readdirSync(dir).filter((f) => f.endsWith(".jsonl")).sort()) {
    for (const line of readFileSync(join(dir, f), "utf8").trim().split("\n")) {
      const e = JSON.parse(line);
      keys.push(`${e.store_id}:${e.seq}`);
    }
  }
  return keys;
}

const CHILD = join(import.meta.dirname, "export-child.ts");
function runChild(crashAt: string): number | null {
  const r = spawnSync(process.execPath, ["--import", "tsx", CHILD, file, root, crashAt], {
    encoding: "utf8",
    cwd: join(import.meta.dirname, ".."),
  });
  return r.signal === "SIGKILL" ? 137 : r.status;
}

describe("export", () => {
  it("writes an immutable segment named by its seq range, one line per event, then only new events", () => {
    const s = openStore(file);
    fill(s, 3);
    const r1 = exportEvents(s, { root });
    expect(r1).toMatchObject({ exported: 3, segment: "events-1-3.jsonl", cursor: 3 });
    const first = readFileSync(join(r1.dir, "events-1-3.jsonl"), "utf8");
    expect(exportEvents(s, { root }).exported).toBe(0);
    fill(s, 2);
    expect(exportEvents(s, { root }).segment).toBe("events-4-5.jsonl");
    expect(readFileSync(join(r1.dir, "events-1-3.jsonl"), "utf8")).toBe(first);
    expect(exportedKeys(r1.dir)).toHaveLength(5);
    expect(JSON.parse(first.split("\n")[0]!)).toMatchObject({ store_id: s.storeId, seq: 1, type: "filed" });
  });

  it("takes the larger of the stored cursor and the highest segment", () => {
    const s = openStore(file);
    fill(s, 4);
    const dir = join(root, s.storeId);
    mkdirSync(dir, { recursive: true });
    writeFileSync(join(dir, "events-1-2.jsonl"), "{}\n");
    expect(exportEvents(s, { root }).segment).toBe("events-3-4.jsonl");
    // stored cursor ahead of segments: nothing is re-exported
    s.setSetting("export_cursor", "4");
    expect(exportEvents(s, { root }).exported).toBe(0);
  });

  for (const step of ["before-rename", "after-rename", "after-cursor"]) {
    it(`a crash at ${step} ends, after a restart and one more run, with every seq exported once`, () => {
      const s = openStore(file);
      fill(s, 3);
      s.close();
      expect(runChild(step)).toBe(137);
      const s2 = openStore(file);
      fill(s2, 2);
      exportEvents(s2, { root });
      exportEvents(s2, { root });
      const keys = exportedKeys(join(root, s2.storeId));
      expect(new Set(keys).size).toBe(keys.length);
      expect(keys.map((k) => Number(k.split(":")[1])).sort((a, b) => a - b)).toEqual([1, 2, 3, 4, 5]);
      expect(readdirSync(join(root, s2.storeId)).some((f) => f.startsWith(".tmp-"))).toBe(false);
    });
  }

  it("a reinstall over the retained file keeps the store_id; a reset starts a new directory", () => {
    const s = openStore(file);
    fill(s, 1);
    const id = s.storeId;
    exportEvents(s, { root });
    s.close();
    const again = openStore(file);
    expect(again.storeId).toBe(id);
    again.close();
    const reset = openStore(join(t.dir, "fresh.db"));
    fill(reset, 1);
    expect(reset.storeId).not.toBe(id);
    exportEvents(reset, { root });
    expect(existsSync(join(root, id))).toBe(true);
    expect(existsSync(join(root, reset.storeId))).toBe(true);
    expect(readdirSync(root).sort()).toEqual([id, reset.storeId].sort());
    void Database;
  });
});
