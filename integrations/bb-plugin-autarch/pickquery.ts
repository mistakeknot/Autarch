import { z } from "zod";

/** Browser-safe RPC input contract. Original identities only; never a latest-generation lookup. */
export const PickActionQuerySchema = z.union([
  z.object({ pick_id: z.string().min(1) }).strict(),
  z.object({ decision_id: z.string().min(1), generation: z.number().int().positive().max(Number.MAX_SAFE_INTEGER) }).strict(),
]);
export type PickActionQuery = z.infer<typeof PickActionQuerySchema>;

export interface PickActionSummary {
  decision_id: string;
  generation: number;
  status: string;
  observation_only: boolean;
  ordinary_continuation: boolean;
  target: string | null;
  deadline: string | null;
  deadline_outcome: string;
  first_failure: boolean;
  readback: string;
}
