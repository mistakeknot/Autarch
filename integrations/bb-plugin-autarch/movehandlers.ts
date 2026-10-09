// The "Your move" actions. A claim, skip or check changes what is waiting on mk, so each one refreshes the moves
// list and Home's totals (the badge, the Queue header and the tab) together [bead mk-yjp7].
import type { MoveHandlers, MoveActionOutcome } from "./ui/yourmove.js";

type MoveMethod = "checkMove" | "claimMove" | "skipMove";
type RpcLike = { call: (method: MoveMethod | "unlater", input: { task_id: string; generation: number } | { ref: string }) => Promise<unknown> };
type RpcResult = { ok?: boolean; status?: number; error?: string };

/** A send becomes an outcome the card can show; a thrown error or a non-ok result is a failure with its reason. */
export async function rpcOutcome(send: Promise<unknown>): Promise<{ ok: boolean; error?: string }> {
  try {
    const r = (await send) as RpcResult | null;
    if (r?.ok === true) return { ok: true };
    return { ok: false, error: r?.error ?? `failed${r?.status ? ` (${r.status})` : ""}` };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}

export function buildMoveHandlers(
  rpc: RpcLike,
  refetchMoves: () => void,
  onChanged: () => void,
  note: (target: { task_id: string }, text: string) => ReturnType<MoveHandlers["onNote"]>,
): MoveHandlers {
  const act = (method: MoveMethod) => (m: { task_id: string; generation: number }): Promise<MoveActionOutcome> =>
    rpcOutcome(rpc.call(method, { task_id: m.task_id, generation: m.generation })).finally(() => {
      refetchMoves();
      onChanged();
    });
  const back = (m: { task_id: string }): Promise<MoveActionOutcome> =>
    rpcOutcome(rpc.call("unlater", { ref: m.task_id })).finally(() => {
      refetchMoves();
      onChanged();
    });
  return { onUnlater: back, onCheck: act("checkMove"), onClaim: act("claimMove"), onSkip: act("skipMove"), onNote: (m, text) => note({ task_id: m.task_id }, text) };
}
