// Segmented event export [D-13]. Nightly, immutable segments outside the plugin folder
// so they outlive `bb plugin remove`: events-<first_seq>-<last_seq>.jsonl, written to a
// temp file, fsynced, renamed, and only then is the cursor advanced. At start the
// cursor is the larger of the stored cursor and the highest last_seq among existing
// segments, so a crash on either side of the rename neither duplicates nor loses events.
import { closeSync, fsyncSync, mkdirSync, openSync, readdirSync, renameSync, unlinkSync, writeSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import type { Store } from "./store.js";

export const CURSOR_KEY = "export_cursor";
const SEGMENT = /^events-(\d+)-(\d+)\.jsonl$/;

export function defaultExportRoot(): string {
  return join(homedir(), ".autarch", "home-export");
}

export interface ExportOptions {
  root?: string;
  /** Test seam: called at "before-rename", "after-rename", "after-cursor". */
  hook?: (step: string) => void;
  /** Max events per segment. Default 5000. */
  batch?: number;
}

export interface ExportResult {
  dir: string;
  exported: number;
  segment?: string;
  cursor: number;
}

function fsyncDir(dir: string): void {
  const fd = openSync(dir, "r");
  try {
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
}

export function exportEvents(store: Store, opts: ExportOptions = {}): ExportResult {
  const storeId = store.storeId;
  const dir = join(opts.root ?? defaultExportRoot(), storeId);
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  let highest = 0;
  for (const name of readdirSync(dir)) {
    if (name.startsWith(".tmp-")) {
      unlinkSync(join(dir, name)); // a crash before the rename left it; it was never a segment
      continue;
    }
    const m = SEGMENT.exec(name);
    if (m) highest = Math.max(highest, Number(m[2]));
  }
  const stored = Number(store.setting(CURSOR_KEY) ?? 0);
  const cursor = Math.max(stored, highest);
  if (cursor !== stored) store.setSetting(CURSOR_KEY, String(cursor));

  const rows = store.db
    .prepare("SELECT * FROM events WHERE seq > ? ORDER BY seq LIMIT ?")
    .all(cursor, opts.batch ?? 5000) as {
    seq: number;
    at: string;
    type: string;
    decision_id: string | null;
    detail_json: string;
  }[];
  if (rows.length === 0) return { dir, exported: 0, cursor };

  const first = rows[0]!.seq;
  const last = rows[rows.length - 1]!.seq;
  const body =
    rows
      .map((r) =>
        JSON.stringify({
          store_id: storeId,
          seq: r.seq,
          at: r.at,
          type: r.type,
          decision_id: r.decision_id,
          detail: JSON.parse(r.detail_json),
        }),
      )
      .join("\n") + "\n";
  const name = `events-${first}-${last}.jsonl`;
  const tmp = join(dir, `.tmp-${process.pid}-${name}`);
  const fd = openSync(tmp, "wx", 0o600);
  try {
    writeSync(fd, body);
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
  opts.hook?.("before-rename");
  renameSync(tmp, join(dir, name));
  fsyncDir(dir);
  opts.hook?.("after-rename");
  store.setSetting(CURSOR_KEY, String(last));
  opts.hook?.("after-cursor");
  return { dir, exported: rows.length, segment: name, cursor: last };
}
