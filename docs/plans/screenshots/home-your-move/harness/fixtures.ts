// Fixture data for the screenshots. Nothing here is real: names, shas and paths are invented.
export const SHA = "3f9a1c0e5b7d42a8c6e1f0b9d8a7c6b5e4d3c2b1a09f8e7d6c5b4a3928170615";
export const LONG_ARG = "--set=cutover-2026-10-07-primary-region-eu-west-1-with-a-very-long-descriptive-suffix-to-prove-the-line-scrolls-sideways";

export const asksData = {
  owed: [
    {
      id: "dec-1", project: "Autarch", thread: "thr-cutover", subject: "Which window for the cutover?", asker: "thread",
      filed_at: "2026-10-05T10:00:00Z", revision: "r1", mentions: 0,
      ask: {
        question: "The migration can run tonight or wait for the weekend. See /home/example/plans/cutover.md.",
        recommendation: "tonight",
        options: [
          { id: "tonight", label: "Run tonight", kind: "instruction", reversible: true, instruction: "Run the cutover tonight and report back." },
          { id: "weekend", label: "Wait for the weekend", kind: "ruling-only", reversible: true },
          { id: "cancel", label: "Cancel the migration", kind: "ruling-only", reversible: false },
        ],
      },
    },
    {
      id: "dec-2", project: "Aleph", thread: "thr-b", subject: "Rename the plugin?", asker: "thread",
      filed_at: "2026-10-06T10:00:00Z", revision: "r2", mentions: 1,
      ask: { question: "Rename now or later?", options: [{ id: "now", label: "Rename now", kind: "ruling-only", reversible: true }] },
    },
  ],
  runbook: [], lane: [], asks: [],
  undeliverable: [
    { id: "ob-1", decision_id: "dec-0", kind: "ruling-wake", recipient: "thr-gone", last_error: "Thread is archived and no live successor was found" },
  ],
  failures: [], uncertain: [],
  delegation: { settings: {}, suspended: false }, machineOwners: {},
};

const base = {
  generation: 1, state: "open", owner: "thr-cutover", opened_at: "2026-10-06T09:00:00.000Z", claimed_at: null, skipped_at: null,
  checked_at: null, report_deadline_at: null, url: null, need: null, closed_at: null, closed_by: null, evidence: null, report: null,
};
const cmd = (path: string, args: string[]) => {
  const q = (s: string) => `'${s}'`;
  return [
    { label: "sha256sum", command: `sha256sum ${q(path)}`, expectedSha: SHA },
    { label: "check", command: `bash ${q(path)} --check` },
    { label: "run", command: ["bash", q(path), ...args.map(q)].join(" ") },
    { label: "recover", command: `bash ${q("/home/example/scripts/undo-cutover.sh")}`, expectedSha: "b".repeat(64) },
  ];
};
export const movesData = {
  yourMove: [
    { ...base, task_id: "mv-script", kind: "script", title: "Run the cutover script on the host", script: { path: "/home/example/scripts/cutover.sh", sha256: SHA }, commands: cmd("/home/example/scripts/cutover.sh", ["--set", LONG_ARG]) },
    { ...base, task_id: "mv-pr", kind: "pr", title: "Merge the Home your-move PR", url: "https://github.com/mistakeknot/autarch/pull/412", script: null, commands: [] },
    { ...base, task_id: "mv-ctx", kind: "context", title: "Need the production domain", need: "Which Cloudflare account owns getbb.app?", script: null, commands: [] },
  ],
  reported: [
    { ...base, task_id: "mv-wait", kind: "script", state: "claimed", claimed_at: "2026-10-06T11:00:00.000Z", title: "Backup rotation", report_deadline_at: "2026-10-06T13:00:00.000Z", script: { path: "/home/example/scripts/rotate.sh", sha256: SHA }, commands: [] },
    { ...base, task_id: "mv-fail", kind: "script", state: "claimed", claimed_at: "2026-10-06T11:00:00.000Z", title: "Schema migration", script: { path: "/home/example/scripts/migrate.sh", sha256: SHA }, commands: [], report: { outcome: "failed", failing_step: "apply-0042", error_line: "ERROR: relation \"cards\" already exists", report_link: "/home/example/thread-storage/reports/migrate-1.txt", reported_at: "2026-10-06T11:20:00.000Z", deadline_at: null } },
    { ...base, task_id: "mv-ok", kind: "script", state: "claimed", claimed_at: "2026-10-06T11:00:00.000Z", title: "Prune old snapshots", script: { path: "/home/example/scripts/prune.sh", sha256: SHA }, commands: [], report: { outcome: "succeeded", failing_step: null, error_line: null, report_link: "/home/example/thread-storage/reports/prune-1.txt", reported_at: "2026-10-06T11:05:00.000Z", deadline_at: null } },
    { ...base, task_id: "mv-none", kind: "script", state: "claimed", claimed_at: "2026-10-06T09:30:00.000Z", title: "Reindex search", script: { path: "/home/example/scripts/reindex.sh", sha256: SHA }, commands: [], report: { outcome: "no-report", failing_step: null, error_line: null, report_link: null, reported_at: null, deadline_at: "2026-10-06T10:30:00.000Z" } },
  ],
  later: [{ ...base, task_id: "mv-later", kind: "read", title: "Read the design note", skipped_at: "2026-10-06T10:00:00.000Z", url: "https://example.test/note", script: null, commands: [] }],
  hidden: [],
  audit: [{ task_id: "mv-done", generation: 1, kind: "pr", closed_at: "2026-10-05T12:00:00.000Z", closed_by: "github", evidence: "merged" }],
};

export const unboundQueue = {
  rows: [
    { id: "row-ok", decision_id: "dec-9", task_id: "t9", card_key: "AU-9", binding_state: "confirmed", project: "Autarch", title: "Pick the retry policy", refs: [], blocks_count: 1, created_at: "2026-10-05T00:00:00.000Z", thread: "thr-a", pinned: false, ask: { question: "Which retry policy?", options: [{ id: "exp", label: "Exponential", kind: "ruling-only", reversible: true }] }, revision: "rv", mentions: 0, display_reason: null, display_only: false, overrides_generation: null, changed_after_ruling: false, root: { state: "verified", reason: null } },
    { id: "row-un", decision_id: "dec-10", task_id: "t10", card_key: "NW-3", binding_state: "suggested", project: null, title: "Pick the queue backend", refs: [], blocks_count: 2, created_at: "2026-10-05T00:00:00.000Z", thread: "thr-n", pinned: false, ask: { question: "Which backend?", options: [{ id: "sqs", label: "SQS", kind: "ruling-only", reversible: true }] }, revision: "rv", mentions: 0, display_reason: null, display_only: false, overrides_generation: null, changed_after_ruling: false, root: { state: "unverified", reason: "no root run for this project" } },
  ],
  legacy: { count: 0, owed: [], runbook: [], machine: { lane: [], asks: [] } }, bindings: [], inactive_projects: [],
};
