// The Home RPC methods. The panel calls only these; nothing here carries an attested caller,
// so `pick` records `by: mk` as advisory attribution [P-10] [G-15]. Inputs are strict; outputs
// are shaped by the service and pass through as JSON.
import { z } from "zod";

const out = z.unknown();

export const homeMethods = {
  listAsks: { input: z.null(), output: out },
  // The blocks panel's read model (plan 1.4). `thread` pins the cards that involve that thread.
  queue: { input: z.object({ thread: z.string().min(1).max(128).optional() }), output: out },
  // A card's root-run display (plan 1.5, Task 2.8): the paste command and read-only status. Reads the card
  // from tasks and the script file from disk; writes nothing and runs nothing.
  rootRun: { input: z.object({ task_id: z.string().min(1).max(64) }), output: out },
  // mk's one-time confirmation of a tasks-project to Home-project binding (plan 1.3.6, Q5). RPC only: no CLI verb.
  setBinding: {
    input: z.object({
      tasks_project_id: z.string().min(1).max(128),
      state: z.enum(["confirmed", "rejected"]),
      home_project: z.string().min(1).max(200).optional(),
    }),
    output: out,
  },
  listRecent: { input: z.object({ project: z.string().min(1), limit: z.number().int().min(1).max(200).default(50) }), output: out },
  pick: {
    input: z.object({
      decision_id: z.string().min(1),
      option_id: z.string().min(1),
      revision: z.string().min(1),
      pick_id: z.string().min(1),
      reason: z.string().max(2000).optional(),
      /** Where the operator picked: Home (default) or the summoned overlay. The CLI records its own. */
      surface: z.enum(["home", "overlay"]).default("home"),
    }),
    output: out,
  },
  // Your move (plan Revision 2, 6). `claimMove` records mk's "I did it" as reported, not verified: it never closes
  // a move. `skipMove` is "Later / skip". Neither carries a caller identity, so both are advisory (unattested).
  moves: { input: z.null(), output: out },
  claimMove: {
    input: z.object({ task_id: z.string().min(1).max(64), generation: z.number().int().min(1).max(1_000_000), note: z.string().max(500).optional() }).strict(),
    output: out,
  },
  checkMove: { input: z.object({ task_id: z.string().min(1).max(64), generation: z.number().int().min(1).max(1_000_000) }).strict(), output: out },
  skipMove: { input: z.object({ task_id: z.string().min(1).max(64), generation: z.number().int().min(1).max(1_000_000) }).strict(), output: out },
  // The Other box's "Ask / note": a comment on the card plus an owner wake. Never rules or closes.
  note: { input: z.object({ task_id: z.string().min(1).max(64).optional(), decision_id: z.string().min(1).max(128).optional(), text: z.string().min(1).max(2000), note_id: z.string().min(1).max(64) }).strict(), output: out },
  // Conversation on the card: read-only views of the mirrored comments. Display only; nothing here rules or closes.
  conversation: { input: z.object({ task_id: z.string().min(1).max(64) }).strict(), output: out },
  conversationUnread: { input: z.null(), output: out },
  markConversationSeen: { input: z.object({ task_id: z.string().min(1).max(64), through: z.string().min(1).max(200) }).strict(), output: out },
  dismiss: { input: z.object({ decision_id: z.string().min(1), obligation_id: z.string().min(1) }), output: out },
  override: { input: z.object({ decision_id: z.string().min(1) }), output: out },
  setDelegation: {
    input: z.object({
      vizierThreadId: z.string().min(1),
      projects: z.array(z.string().min(1)).max(100),
      dailyCap: z.number().int().min(0).max(1000),
    }),
    output: out,
  },
  revokeApproval: { input: z.object({ approval_id: z.string().min(1) }), output: out },
  setViewing: { input: z.object({ decision_id: z.string().min(1).nullable() }), output: out },
  resend: { input: z.object({ id: z.string().min(1), attempt: z.number().int().min(0), click_id: z.string().min(1) }), output: out },
  markSeen: { input: z.object({ item: z.string().min(1) }), output: out },
  markAllSeen: { input: z.object({ ids: z.array(z.string().min(1)).max(500) }), output: out },
  catchup: { input: z.null(), output: out },
  stats: { input: z.object({ days: z.number().int().min(1).max(365).default(14) }), output: out },
  health: { input: z.null(), output: out },
};
