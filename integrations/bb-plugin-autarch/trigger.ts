// Touches the update trigger file so the runner's systemd .path unit starts it right away, instead
// of waiting for the periodic timer (the slow fallback). The .path unit's PathChanged= fires on
// IN_CLOSE_WRITE, not on a bare mtime update, so this always opens the file for append and closes
// it again -- that alone fires the event whether or not any bytes are written. Never writes content.
import { closeSync, openSync } from "node:fs";
import { join } from "node:path";
import { TRIGGER_FILE } from "./updates.js";

/** Touch TRIGGER_FILE under dir so the .path unit starts the runner right away; best-effort, since
 * the timer still covers a request if this fails or the .path unit is not installed. Runs nothing itself. */
export function touchTrigger(dir: string): void {
  try {
    closeSync(openSync(join(dir, TRIGGER_FILE), "a"));
  } catch {
    /* the timer still covers it */
  }
}
