import { spawnSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { tmpDir } from "./helpers.js";

const JQ = join(import.meta.dirname, "..", "scripts", "rig-acceptance.jq");
const rig = (o: Record<string, unknown> = {}) => ({ launcher_pid: 10, runtime_file_pid: 10, socket_owner_pid: 10, listen_inode: 555, listen_inode_in_owned_set: true, owned_set: [10, 11], nonce_in_tasks_db: true, stopped: true, netns_isolated: true, ...o });
const rec = (o: { rig?: unknown; threads?: string[]; archived?: string[] } = {}) => {
  const threads = o.threads ?? ["thr_1"];
  return { scenario: "s", evidence: { rig: "rig" in o ? o.rig : rig(), threads, cleanup: { archived: o.archived ?? threads } } };
};
const run = (text: string) => {
  const f = join(tmpDir().dir, "e.jsonl");
  writeFileSync(f, text);
  return spawnSync("jq", ["-se", "-f", JQ, f], { encoding: "utf8" });
};
const jsonl = (...r: unknown[]) => r.map((x) => JSON.stringify(x)).join("\n") + "\n";
const ok = (...r: unknown[]) => {
  const x = run(jsonl(...r));
  expect(x.stderr, "a jq runtime error is a failure").toBe("");
  expect(x.status).toBe(0);
};
const bad = (...r: unknown[]) => expect(run(jsonl(...r)).status).not.toBe(0);

describe("rig-acceptance.jq", () => {
  it("accepts valid evidence: socket owner is the launcher in one record and a descendant in the other", () => {
    ok(rec(), rec({ rig: rig({ socket_owner_pid: 11 }) }));
  });
  it.each([
    ["socket owner not in owned_set", { socket_owner_pid: 99 }],
    ["launcher differs from runtime file pid", { runtime_file_pid: 12 }],
    ["listen inode not in owned set", { listen_inode_in_owned_set: false }],
    ["owned_set not an array", { owned_set: "10" }],
    ["not stopped", { stopped: false }],
    ["nonce not in tasks db", { nonce_in_tasks_db: false }],
    ["netns not isolated", { netns_isolated: false }],
    ["netns_isolated missing", { netns_isolated: undefined }],
  ])("rejects: %s", (_n, o) => bad(rec({ rig: rig(o) })));
  it("rejects a missing evidence.rig", () => bad(rec({ rig: undefined })));
  it("rejects no threads", () => bad(rec({ threads: [] })));
  it("rejects archived threads that differ from created threads", () => bad(rec({ threads: ["thr_1", "thr_2"], archived: ["thr_1"] })));
  it("rejects an empty file", () => expect(run("").status).not.toBe(0));
  it("reports a jq runtime error as failure (the rev-5.3 form indexes the array)", () => {
    const f = join(tmpDir().dir, "old.jq");
    writeFileSync(f, "all(.[]; .evidence.rig | type == \"object\") and (.evidence.threads | length) > 0");
    const e = join(tmpDir().dir, "e.jsonl");
    writeFileSync(e, jsonl(rec()));
    const x = spawnSync("jq", ["-se", "-f", f, e], { encoding: "utf8" });
    expect(x.status).not.toBe(0);
    expect(x.stderr).toMatch(/Cannot index array with string "evidence"/);
  });
});
