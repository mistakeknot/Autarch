/** Display metadata only; keys match exactly, order is bounded equally in Go. */
export type MergeGroup = { key: string; order: number };
export function validateGroup(raw: unknown): MergeGroup {
 const fail = (): never => { throw new Error("invalid group: expected key token and integer order 1-2147483647"); };
 if (!raw || typeof raw !== "object" || Array.isArray(raw)) return fail();
 const o = raw as Record<string, unknown>;
 if (Object.keys(o).length !== 2 || !Object.hasOwn(o, "key") || !Object.hasOwn(o, "order")) return fail();
 if (typeof o.key !== "string" || o.key.match(/^[a-z0-9][a-z0-9._-]{0,63}$/)?.[0] !== o.key || typeof o.order !== "number" || !Number.isInteger(o.order) || o.order < 1 || o.order > 2147483647) return fail();
 return { key: o.key, order: o.order };
}
