export const KINDS: string[];
export type Counts = Record<string, Record<string, number>>;
export function countEscapes(text: string): { as_unknown_as: number; as_any: number; lint_disable: number };
export function scanTree(root: string): Counts;
export function mergeWarnings(counts: Counts, perFile: Record<string, number>): Counts;
export function diff(base: Counts, live: Counts): { rose: string[]; fell: string[] };
export function normalize(counts: Counts): Counts;
export function main(argv?: string[], env?: Record<string, string | undefined>): number;
