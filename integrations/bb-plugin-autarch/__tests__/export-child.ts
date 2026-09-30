// Crash-test child: exports, and SIGKILLs itself at the named step.
import Database from "better-sqlite3";
import { exportEvents } from "../export.js";
import { Store } from "../store.js";

const [file, root, crashAt] = process.argv.slice(2) as [string, string, string];
const store = new Store(new Database(file));
exportEvents(store, {
  root,
  hook: (step) => {
    if (step === crashAt) process.kill(process.pid, "SIGKILL");
  },
});
