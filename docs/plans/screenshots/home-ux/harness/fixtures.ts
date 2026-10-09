// Invented data for the Home UX pass screenshots: one long free-text card with a dry run and a final command,
// a vizier-adoption notice, and a few catch-up rows. No real hostnames, users or private paths.
const SCRIPT = "/tmp/example/install-hook.sh";
export const QUESTION = `Install the reviewed hook on the build host. First run the dry run and read its report:\n\nbash ${SCRIPT} --check\n\nwhen it reports clean, run the real install. It needs the diff flag or it stops:\n\nbash ${SCRIPT} --apply --accept-install-diff\n\nIt writes the install diff and nothing else.`;
const opt = (id: string, label: string) => ({ id, label, kind: "ruling-only", reversible: true });
const owed = (i: number, subject: string, question: string) => ({
  id: `dec-${i}`, project: "Example", thread: `thr-${i}`, subject, asker: "thread", key: `EX-${130 + i}`,
  filed_at: "2026-10-07T16:00:00Z", revision: "r1", mentions: 0,
  ask: { question, recommendation: "a", options: [opt("a", "Ran --check then the real run"), opt("b", "Not yet")] },
});
export const asksData = {
  owed: [owed(3, "install the reviewed hook", QUESTION), owed(4, "pr #12: merge the eval-only change", "Merge it?"), owed(5, "retire the old triage prompt", "Retire it?")],
  runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: true }, machineOwners: {},
};
export const catchupItems = [
  { item: "failed:41", kind: "failure", at: "2026-10-07T20:00:00Z", text: "A run failed on thr-9: the build step timed out.", project: "Example" },
  { item: "note:7", kind: "note", at: "2026-10-07T21:00:00Z", text: "The coordinator noted that the two repos were admitted." },
  { item: "routine:a", kind: "routine", at: "2026-10-07T22:00:00Z", text: "2 routine updates", members: ["ruling:1", "closed:2"], lines: ["You ruled on retire the old prompt.", "Closed: merge the eval change."] },
];
export const waiting = {
  total: 5, decide: 3, moves: 1, notices: 1, held: 2, updates: 3, suspended: true,
  noticeItems: [{ item: "pinned:vizier-adopted:9", at: "2026-10-07T23:00:00Z", text: "Home adopted thr-viz as the vizier thread; delegated rulings stay suspended until you see this." }],
  definition: "Decisions waiting on you (not on hold), plus your open moves (not claimed, skipped or on hold), plus unseen notices that change what Home does. Catch-up reading and held cards are shown beside it, not in it.",
};
