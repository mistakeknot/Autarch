// Native-wrapper-owned race: each worker opens its own SQLite connection before the barrier.
import { parentPort, workerData } from "node:worker_threads";
import { register } from "tsx/esm/api";
register();
const { default: Database } = await import("better-sqlite3");
const { Store } = await import("../../store.js");
const { capturePick, claimPickRelay } = await import("../../pickcapture.js");
const { file, now, decision, relay, barrier, operation } = workerData;
const store = new Store(new Database(file), { now: () => now });
const gate = new Int32Array(barrier);
parentPort.postMessage({ ready: true });
Atomics.wait(gate, 0, 0);
try {
  const result = operation === "capture"
    ? capturePick(store, decision, "shadow", now)
    : claimPickRelay(store, relay, 0, now);
  parentPort.postMessage({ result });
} finally { store.close(); }
