// The Home RPC methods. The panel calls only these; nothing here carries an attested caller,
// so `pick` records `by: mk` as advisory attribution [P-10] [G-15]. Inputs are strict; outputs
// are checked loosely (see `result`) and pass through as JSON.
import { z } from "zod";

// Outputs are checked only for the shape every caller relies on, and every object passes extra fields
// through, so a service that adds a field never breaks a panel. Most write methods answer with the
// `{ ok }` envelope; the read models answer with the one key their panel reads.
const result = z.object({ ok: z.boolean() }).passthrough();
const readModel = z.custom<object>((v) => typeof v === "object" && v !== null && !Array.isArray(v), "expected an object");
const list = z.array(z.unknown());
const docResult = z.union([
  z.object({ ok: z.literal(true), path: z.string(), size: z.number(), sha256: z.string(), text: z.string(), markdown: z.boolean(), matches: z.boolean().nullable() }),
  z.object({ ok: z.literal(false), error: z.string() }),
]);
const conversationView = z.object({ task_id: z.string(), comments: list, unread: z.number(), stale: z.boolean(), last_ok_at: z.string().nullable(), options: list });
const moveGroups = z.object({ yourMove: list, reported: list, later: list, hidden: list, audit: z.unknown() });

export const homeMethods = {
  listAsks: { input: z.null(), output: readModel },
  // The blocks panel's read model (plan 1.4). `thread` pins the cards that involve that thread.
  queue: { input: z.object({ thread: z.string().min(1).max(128).optional() }), output: readModel },
  // A card's root-run display (plan 1.5, Task 2.8): the paste command and read-only status. Reads the card
  // from tasks and the script file from disk; writes nothing and runs nothing.
  rootRun: { input: z.object({ task_id: z.string().min(1).max(64) }), output: result },
  // mk's one-time confirmation of a tasks-project to Home-project binding (plan 1.3.6, Q5). RPC only: no CLI verb.
  setBinding: {
    input: z.object({
      tasks_project_id: z.string().min(1).max(128),
      state: z.enum(["confirmed", "rejected"]),
      home_project: z.string().min(1).max(200).optional(),
    }),
    output: result,
  },
  listRecent: { input: z.object({ project: z.string().min(1), limit: z.number().int().min(1).max(200).default(50) }), output: z.object({ recent: list }).passthrough() },
  pick: {
    input: z.object({
      decision_id: z.string().min(1),
      option_id: z.string().min(1),
      revision: z.string().min(1),
      pick_id: z.string().min(1),
      reason: z.string().max(20000).optional(),
      /** Images with the Other box answer (png, jpeg, gif, webp); delivered to the owner thread next to the text. */
      images: z.array(z.object({ mime: z.string().max(32), data_base64: z.string().max(7_300_000) }).strict()).max(4).optional(),
      /** A multi-pick ask's answer: question id to option id, one per question. Goes with option_id "answers"; `reason` is then an optional note. */
      answers: z.record(z.string().max(32), z.string().max(32)).optional(),
      /** Where the operator picked: Home (default). "overlay" is historical (the overlay panel was removed) and stays so old rows read. The CLI records its own. */
      surface: z.enum(["home", "overlay"]).default("home"),
    }),
    output: result,
  },
  // Your move (plan Revision 2, 6). `claimMove` records mk's "I did it" as reported, not verified: it never closes
  // a move. `skipMove` is "Later / skip". Neither carries a caller identity, so both are advisory (unattested).
  moves: { input: z.null(), output: moveGroups },
  claimMove: {
    input: z.object({ task_id: z.string().min(1).max(64), generation: z.number().int().min(1).max(1_000_000), note: z.string().max(500).optional() }).strict(),
    output: result,
  },
  checkMove: { input: z.object({ task_id: z.string().min(1).max(64), generation: z.number().int().min(1).max(1_000_000) }).strict(), output: result },
  skipMove: { input: z.object({ task_id: z.string().min(1).max(64), generation: z.number().int().min(1).max(1_000_000) }).strict(), output: result },
  // Later (bead mk-8741): set an open card aside, below the active ones and outside the Waiting count; undo moves it back.
  // `ref` is a task id or a tasks key. Neither rules, closes or refuses a pick.
  later: { input: z.object({ ref: z.string().min(1).max(128) }).strict(), output: result },
  unlater: { input: z.object({ ref: z.string().min(1).max(128) }).strict(), output: result },
  // The Other box's "Ask / note": a comment on the card plus an owner wake. Never rules or closes.
  note: { input: z.object({ task_id: z.string().min(1).max(64).optional(), decision_id: z.string().min(1).max(128).optional(), text: z.string().min(1).max(20000), note_id: z.string().min(1).max(64), images: z.array(z.object({ mime: z.string().max(32), data_base64: z.string().max(7_300_000) }).strict()).max(4).optional() }).strict(), output: result },
  // Conversation on the card: read-only views of the mirrored comments. Display only; nothing here rules or closes.
  conversation: { input: z.object({ task_id: z.string().min(1).max(64) }).strict(), output: conversationView },
  conversationUnread: { input: z.null(), output: z.object({ unread: z.record(z.string(), z.number()) }) },
  // The document reader (bead mk-5zfbg): a read-only view of one allowlisted text file, see docread.ts.
  readDoc: { input: z.object({ path: z.string().min(1).max(1024), sha256: z.string().regex(/^[0-9a-fA-F]{64}$/).optional() }).strict(), output: docResult },
  // Open one allowlisted file in bb's own file panel (bead mk-98y4s): the server checks the path and names the panel target.
  openTarget: { input: z.object({ path: z.string().min(1).max(1024) }).strict(), output: z.union([z.object({ ok: z.literal(true), target: z.object({ kind: z.literal("host"), hostId: z.string().min(1), path: z.string().min(1) }).strict() }).strict(), z.object({ ok: z.literal(false), error: z.string() }).strict()]) },
  markConversationSeen: { input: z.object({ task_id: z.string().min(1).max(64), through: z.string().min(1).max(200) }).strict(), output: result },
  // The Idea box and weekly digest (bead mk-2zojo). An idea is a tracker card; none of these rules, closes or starts anything.
  ideaProjects: { input: z.null(), output: result },
  ideas: { input: z.null(), output: result },
  // The Learn deck (bead mk-uoiwp): off by default; the marks live in the settings table. Neither rules or starts anything.
  learn: { input: z.null(), output: result },
  learnSet: { input: z.object({ enabled: z.boolean().optional(), card: z.string().regex(/^\d{2}-[a-z0-9-]{1,60}$/).optional(), mark: z.enum(["got", "later", "clear"]).optional() }).strict(), output: result },
  fileIdea: { input: z.object({ project_id: z.string().min(1).max(64), text: z.string().max(4000), idea_id: z.string().min(1).max(64) }).strict(), output: result },
  actIdea: { input: z.object({ task_id: z.string().min(1).max(64), action: z.enum(["pursue", "park", "drop"]) }).strict(), output: result },
  clearIdeaDigest: { input: z.null(), output: result },
  dismiss: { input: z.object({ decision_id: z.string().min(1), obligation_id: z.string().min(1) }), output: result },
  override: { input: z.object({ decision_id: z.string().min(1) }), output: result },
  setDelegation: {
    input: z.object({
      vizierThreadId: z.string().min(1),
      projects: z.array(z.string().min(1)).max(100),
      dailyCap: z.number().int().min(0).max(1000),
    }),
    output: result,
  },
  revokeApproval: { input: z.object({ approval_id: z.string().min(1) }), output: result },
  updateInfo: { input: z.null(), output: readModel },
  requestUpdate: { input: z.object({ sha: z.string().regex(/^[0-9a-f]{40}$/) }).strict(), output: result },
  setViewing: { input: z.object({ decision_id: z.string().min(1).nullable() }), output: result },
  cardState: { input: z.object({ key: z.string().regex(/^[A-Z][A-Z0-9]{1,11}-\d{1,7}$/) }), output: readModel },
  resend: { input: z.object({ id: z.string().min(1), attempt: z.number().int().min(0), click_id: z.string().min(1) }), output: result },
  markSeen: { input: z.object({ item: z.string().min(1) }), output: result },
  markAllSeen: { input: z.object({ ids: z.array(z.string().min(1)).max(500) }), output: z.object({ marked: list }).passthrough() },
  catchup: { input: z.null(), output: z.object({ items: list }).passthrough() },
  waiting: { input: z.null(), output: readModel },
  stats: { input: z.object({ days: z.number().int().min(1).max(365).default(14) }), output: readModel },
  health: { input: z.null(), output: z.object({ ready: z.boolean() }).passthrough() },
};

// How each RPC method meets the CLI (bead mk-uoiwp). A method either has a CLI verb, whose summary
// and input limits cli.ts reads from here, or is RPC only: the panel calls it and no verb may. A
// method added to `homeMethods` without an entry here fails the typecheck, and the CLI test fails
// when a verb is neither listed here nor in `cliOnlyVerbs`.
export type CliMapping = { verb: string; summary: string } | { rpcOnly: string };

export const homeCliMap = {
  listAsks: { verb: "list", summary: "List the asks waiting for a ruling" },
  moves: { verb: "moves", summary: "Every Your move row with its claim, report and closing evidence (rollback export)" },
  stats: { verb: "stats", summary: "Picks, delegation and filing counts over a window" },
  listRecent: { verb: "feed", summary: "The recent-rulings feed a thread sees" },
  queue: { rpcOnly: "the panel's read model" },
  rootRun: { rpcOnly: "a card's paste command display" },
  setBinding: { rpcOnly: "mk's confirmation of a binding; the CLI can only read (`binding`) or, as the vizier, bind" },
  pick: { rpcOnly: "mk's ruling, advisory attribution from the browser" },
  claimMove: { rpcOnly: "mk's own report" },
  checkMove: { rpcOnly: "mk's own check" },
  skipMove: { rpcOnly: "mk's own choice" },
  later: { rpcOnly: "mk's own choice" },
  unlater: { rpcOnly: "mk's own choice" },
  note: { rpcOnly: "the Other box; the CLI `note` is a different, vizier-only note" },
  conversation: { rpcOnly: "the panel's read model" },
  conversationUnread: { rpcOnly: "the panel's read model" },
  readDoc: { rpcOnly: "the document reader pane; the CLI has no file-reading verb" },
  openTarget: { rpcOnly: "the file link on a card; bb's file panel does the showing" },
  markConversationSeen: { rpcOnly: "the panel's own state" },
  ideaProjects: { rpcOnly: "the Idea box" },
  ideas: { rpcOnly: "the Idea box" },
  learn: { rpcOnly: "the Learn tab" },
  learnSet: { rpcOnly: "the Learn tab" },
  fileIdea: { rpcOnly: "the Idea box" },
  actIdea: { rpcOnly: "the Idea box" },
  clearIdeaDigest: { rpcOnly: "the Idea box" },
  dismiss: { rpcOnly: "mk's own choice" },
  override: { rpcOnly: "delegation settings are never reachable from the CLI" },
  setDelegation: { rpcOnly: "delegation settings are never reachable from the CLI" },
  revokeApproval: { rpcOnly: "mk's own choice" },
  setViewing: { rpcOnly: "the panel reports what it shows; the CLI reads it with `viewing`" },
  cardState: { rpcOnly: "the panel's read model for a card-key link" },
  resend: { rpcOnly: "mk's own choice" },
  markSeen: { rpcOnly: "the panel's own state" },
  markAllSeen: { rpcOnly: "the panel's own state" },
  catchup: { rpcOnly: "the panel's read model" },
  waiting: { rpcOnly: "the panel's read model" },
  updateInfo: { rpcOnly: "the panel's read model" },
  requestUpdate: { rpcOnly: "mk's own choice; the runner outside the plugin installs" },
  health: { rpcOnly: "the panel's read model" },
} satisfies Record<keyof typeof homeMethods, CliMapping>;

/** CLI verbs with no RPC twin: lookups, machine-blocker lifecycle, and the vizier-only writes. */
export const cliOnlyVerbs = [
  "ask", "get", "viewing", "progress", "resolve", "withdraw", "binding", "bind", "hold", "unhold",
  "unbind", "rule", "handoff", "approval check", "report", "note",
] as const;

/** The summary of a verb that mirrors an RPC method. */
export function verbSummary<K extends keyof typeof homeCliMap>(method: K): string {
  const m: CliMapping = homeCliMap[method];
  if ("verb" in m) return m.summary;
  throw new Error(`${method} is RPC only`);
}
