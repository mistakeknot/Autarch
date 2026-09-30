// Crash-test child for the store: runs one primitive and SIGKILLs itself at a hook.
import Database from "better-sqlite3";
import { Store } from "../store.js";

const [file, op, id, crashAt] = process.argv.slice(2) as [string, string, string, string];
const hook = (step: string) => {
  if (step === crashAt) process.kill(process.pid, "SIGKILL");
};
const store = new Store(new Database(file), { hook });

if (op === "pick") {
  store.recordPick(
    { decision_id: id, pick_id: `pick-${id}`, option_id: "a", revision: "rev-1", by: "mk", surface: "home" },
    [
      { id: `${id}-o1`, kind: "notify", recipient: "x" },
      { id: `${id}-o2`, kind: "ruling-file" },
    ],
  );
  hook("committed");
} else if (op === "recipients") {
  store.insertObligations(
    id,
    ["a", "b", "c"].map((th) => ({ id: `${id}-${th}`, kind: "steps-done", recipient: th, op: `steps-done:${id}:${th}` })),
  );
} else if (op === "file") {
  store.insertDecision({
    id,
    request_id: `req-${id}`,
    identity: "i",
    revision: "rev-1",
    semantic_key: "s",
    subject: "s",
    kind: "decide",
    project: "p",
    asker: "a",
    thread: "t",
    body_json: "{}",
  });
}
