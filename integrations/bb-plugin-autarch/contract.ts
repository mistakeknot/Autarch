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
      reason: z.string().max(1000).optional(),
    }),
    output: out,
  },
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
  resend: { input: z.object({ id: z.string().min(1), attempt: z.number().int().min(0), click_id: z.string().min(1) }), output: out },
  markSeen: { input: z.object({ item: z.string().min(1) }), output: out },
  markAllSeen: { input: z.object({ ids: z.array(z.string().min(1)).max(500) }), output: out },
  catchup: { input: z.null(), output: out },
  stats: { input: z.object({ days: z.number().int().min(1).max(365).default(14) }), output: out },
  health: { input: z.null(), output: out },
};
