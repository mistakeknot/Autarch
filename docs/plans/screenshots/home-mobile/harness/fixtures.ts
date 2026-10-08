// Invented queue data shaped like the phone screenshot: many asks, all from one project, long titles.
const TITLES = [
  "shadow work: deploy ops #26, then advance the registry pin", "forrester: drop fluxrig-data #2 after the near-miss tests land",
  "let agents share preview servers with the owner's browser", "parallaxon: the new shdwwrk.com homepage and stage script",
  "fluxrig-data pr #3: merge the eval-only rubric change", "naradan: admit its two repos to the shared build service",
  "autarch pr #33: pr merge cards show what merging does", "interflux: retire the old triage prompt", "jawntology #4: approve the review",
  "bb plugin build: pin the toolchain digest",
];
const RECS = ["Ran --check then the real run", "Close #2; ship the near-miss tests only", "Done, ran it", "Merge #79 then #80, run the stage script, then I review the URL", "Merge it (I'll merge in GitHub, or vizier merges on my word)"];
export const asksData = {
  owed: Array.from({ length: 26 }, (_, i) => ({
    id: `dec-${i}`, project: "Sylveste", thread: `thr-${i}`, subject: TITLES[i % TITLES.length]!, asker: "thread",
    filed_at: "2026-10-07T16:00:00Z", revision: "r1", mentions: 0,
    ask: { question: "Which way?", recommendation: "a", options: [{ id: "a", label: RECS[i % RECS.length]!, kind: "ruling-only", reversible: true }, { id: "b", label: "Not yet", kind: "ruling-only", reversible: true }] },
  })),
  runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {},
};
