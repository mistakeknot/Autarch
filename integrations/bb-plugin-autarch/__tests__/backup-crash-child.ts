// Crash-test child: opens a v3 Store and SIGKILLs itself at one migration hook step.
import Database from "better-sqlite3";
import { Store } from "../store.js";

const [file, crashAt] = process.argv.slice(2) as [string, string];
new Store(new Database(file), {
  migrate: {
    test: {
      hook: (step) => {
        if (step === crashAt) process.kill(process.pid, "SIGKILL");
      },
    },
  },
});
