// The send-and-report helper the note actions share. Moves are retired (bead mk-1k8pg), so the claim, skip and check
// handlers that lived here are gone; what stays turns an RPC send into an outcome a card can show.
type RpcResult = { ok?: boolean; status?: number; error?: string; reason?: string };

/** A send becomes an outcome the card can show; a thrown error or a non-ok result is a failure with its reason (`error`, or `reason` where the service names it that). */
export async function rpcOutcome(send: Promise<unknown>): Promise<{ ok: boolean; error?: string }> {
  try {
    const r = (await send) as RpcResult | null;
    if (r?.ok === true) return { ok: true };
    return { ok: false, error: r?.error ?? r?.reason ?? `failed${r?.status ? ` (${r.status})` : ""}` };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}
