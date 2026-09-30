// Scenario child process. Modes:
//   pick <file> <projects.json> <decision> <option> <revision> <pick_id> [--kill-after-commit]
//   lock <file>       hold an exclusive lock on a fresh rollback-journal database until stdin closes
import Database from "better-sqlite3";
import { readFileSync } from "node:fs";
import { Service } from "../service.js";
import { Store } from "../store.js";

const [mode, file, ...rest] = process.argv.slice(2) as [string, string, ...string[]];

if (mode === "lock") {
  const db = new Database(file);
  db.pragma("journal_mode = DELETE");
  db.exec("CREATE TABLE IF NOT EXISTS held(x); BEGIN EXCLUSIVE");
  (globalThis as { held?: unknown }).held = db; // keep it reachable: a collected handle drops the lock
  process.stdout.write("locked\n");
  process.stdin.resume();
  process.stdin.on("end", () => process.exit(0));
} else if (mode === "pick") {
  const [projectsFile, id, option, revision, pickId, flag] = rest as [string, string, string, string, string, string?];
  const svc = new Service({
    store: new Store(new Database(file), { busyTimeoutMs: 5000 }),
    projects: async () => JSON.parse(readFileSync(projectsFile, "utf8")),
  });
  if (flag === "--kill-after-commit") {
    // The pick transaction has committed by the time reconcile runs; die before the ruling file exists.
    svc.reconcile = () => process.kill(process.pid, "SIGKILL");
  }
  const r = await svc.pick(id, option, revision, pickId, "mk", "home");
  process.stdout.write(`${JSON.stringify({ ok: r.ok, status: r.status })}\n`);
  process.exit(0);
}
