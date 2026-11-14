// Socket-free restart fixture: Node entrypoint registers the pinned TypeScript loader.
process.stderr.write("restart: loading runtime\n");
const { register } = await import("tsx/esm/api");
register();
const { default: Database } = await import("better-sqlite3");
const { Store } = await import("../../store.js");
const { Service } = await import("../../service.js");

const [file, at, decisionId, ledger] = process.argv.slice(2);
const store = new Store(new Database(file), { now: () => at });
const svc = new Service({ store, now: () => at, projects: async () => [], env: {},
  pickCaptureRuntime: { mode: () => "live" }, rulingsLedger: ledger ?? null });
svc.expirePickEvaluations();
process.stdout.write(JSON.stringify({
  action: store.db.prepare("SELECT * FROM pick_actions WHERE decision_id = ?").get(decisionId),
  obligations: store.obligationsFor(decisionId),
  readback: svc.readPickAction({ decision_id: decisionId, generation: 1 }),
}));
store.close();
