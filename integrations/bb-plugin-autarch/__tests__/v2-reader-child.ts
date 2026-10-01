// An idle v2 (a9853e2) reader: opens a v2 Store, reads once, reports ready, and stays open until stdin closes.
import Database from "better-sqlite3";
import { ensureV2Build } from "./v2build.js";
import { join } from "node:path";

const [file] = process.argv.slice(2) as [string];
const { Store } = (await import(join(ensureV2Build(), "store.ts"))) as typeof import("../store.js");
const s = new Store(new Database(file));
s.setting("store_id");
process.stdout.write("ready\n");
process.stdin.resume();
process.stdin.on("end", () => {
  s.close();
  process.exit(0);
});
