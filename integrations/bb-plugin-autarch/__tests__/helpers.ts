import Database from "better-sqlite3";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Store, type DecisionInput, type StoreOptions } from "../store.js";

export function tmpDir(): { dir: string; cleanup: () => void } {
  const dir = mkdtempSync(join(tmpdir(), "autarch-store-"));
  return { dir, cleanup: () => rmSync(dir, { recursive: true, force: true }) };
}

export function openStore(file: string, opts: StoreOptions = {}): Store {
  return new Store(new Database(file), opts);
}

let counter = 0;
export function decision(over: Partial<DecisionInput> = {}): DecisionInput {
  const n = ++counter;
  return {
    id: `dec-${n}`,
    request_id: `req-${n}`,
    identity: `ident-${n}`,
    revision: "rev-1",
    semantic_key: `sem-${n}`,
    subject: "which one",
    kind: "decide",
    project: "autarch",
    asker: "agent-a",
    thread: "thread-a",
    body_json: "{}",
    ...over,
  };
}

export function pickOf(decisionId: string, over: Record<string, unknown> = {}) {
  return {
    decision_id: decisionId,
    pick_id: `pick-${decisionId}-${++counter}`,
    option_id: "a",
    revision: "rev-1",
    by: "mk",
    surface: "home" as const,
    ...over,
  };
}
