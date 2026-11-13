// Reader-only claims: never evaluated as authority and never part of a v1 ask.
export type SubjectKey = {
    type: 'pr';
    repo: string;
    number: number;
    head: string | null;
    round: number | null;
} | {
    type: 'script';
    script_sha: string;
};
export type SignalEvidence = {
    known: true;
    value: unknown;
} | {
    known: false;
    reason: string;
};
export interface Signals {
    schema: 1;
    kind: 'merge' | 'round';
    approval: string;
    evidence: Record<string, SignalEvidence>;
    conditions: Record<string, 'pass' | 'fail' | 'unknown'>;
    failed: string[];
    checked_at: string;
}
const items = ['base', 'default_branch', 'head_start', 'head_end', 'visibility', 'checks', 'files', 'receipt', 'review_text', 'coverage', 'open_cards', 'rule_history'];
const mergeConditions = ['base.default', 'head.stable', 'repo.private', 'review.provenance', 'review.head', 'review.bound', 'review.verdict', 'review.tests', 'checks.green', 'coverage.entry', 'ask.recommendation', 'keep.paths', 'keep.complete', 'subject.single', 'subject.unvoided'];
const roundConditions = ['head.stable', 'repo.private', 'review.provenance', 'review.head', 'review.bound', 'keep.paths', 'keep.complete', 'subject.single', 'subject.unvoided', 'round.trigger', 'round.history', 'round.new', 'round.fewer', 'round.scope', 'ask.recommendation', 'coverage.entry'];
const reads: Record<string, string[]> = {
    'base.default': ['base', 'default_branch'], 'head.stable': ['head_start', 'head_end'], 'repo.private': ['visibility'],
    'review.provenance': ['receipt'], 'review.head': ['receipt', 'head_end'], 'review.bound': ['receipt', 'review_text'], 'review.verdict': ['receipt'], 'review.tests': ['receipt'],
    'checks.green': ['checks'], 'coverage.entry': ['coverage'], 'ask.recommendation': [], 'keep.paths': ['files'], 'keep.complete': ['files'], 'subject.single': ['open_cards'], 'subject.unvoided': ['rule_history'],
    'round.trigger': ['receipt'], 'round.history': ['receipt'], 'round.new': ['receipt'], 'round.fewer': ['receipt'], 'round.scope': ['receipt', 'files'],
};
function fail(message: string): never { throw new Error(message); }
function object(raw: unknown, what: string): Record<string, unknown> { if (raw === null || typeof raw !== 'object' || Array.isArray(raw))
    return fail(`${what} must be an object`); return raw as Record<string, unknown>; }
function exact(raw: unknown, keys: string[], what: string): Record<string, unknown> { const m = object(raw, what); for (const k of Object.keys(m).sort())
    if (!keys.includes(k))
        fail(`unknown field "${k}" in ${what}`); for (const k of keys)
    if (!Object.hasOwn(m, k))
        fail(`${what} missing ${k}`); return m; }
export function parseSubjectKey(raw: unknown): SubjectKey {
    const m = object(raw, 'subject_key');
    if (m.type === 'pr') {
        exact(m, ['type', 'repo', 'number', 'head', 'round'], 'subject_key');
        if (typeof m.repo !== 'string' || !/^[A-Za-z0-9_.-]{1,100}\/[A-Za-z0-9_.-]{1,100}$/.test(m.repo))
            fail('subject_key repo must be owner/name');
        if (typeof m.number !== 'number' || !Number.isInteger(m.number) || m.number < 1 || m.number > 999999999)
            fail('subject_key number must be a pull request number');
        if (m.head !== null && (typeof m.head !== 'string' || !/^[0-9a-f]{40}$/.test(m.head)))
            fail('subject_key head must be null or 40 lowercase hex characters');
        if (m.round !== null && (typeof m.round !== 'number' || !Number.isSafeInteger(m.round) || m.round < 1))
            fail('subject_key round must be null or a positive safe integer');
    }
    else if (m.type === 'script') {
        exact(m, ['type', 'script_sha'], 'subject_key');
        if (typeof m.script_sha !== 'string' || !/^[0-9a-f]{64}$/.test(m.script_sha))
            fail('subject_key script_sha must be 64 lowercase hex characters');
    }
    else
        fail('subject_key type must be pr or script');
    return m as SubjectKey;
}
export function parseSignals(raw: unknown): Signals {
    const m = exact(raw, ['schema', 'kind', 'approval', 'evidence', 'conditions', 'failed', 'checked_at'], 'signals');
    if (m.schema !== 1)
        fail('signals schema must be 1');
    if (m.kind !== 'merge' && m.kind !== 'round')
        fail('signals kind must be merge or round');
    if (typeof m.approval !== 'string' || !m.approval)
        fail('signals approval must be a nonempty string');
    if (typeof m.checked_at !== 'string' || !m.checked_at)
        fail('signals checked_at must be a nonempty string');
    const evidence = exact(m.evidence, items, 'signals evidence');
    for (const item of items) {
        const e = object(evidence[item], `signals evidence ${item}`);
        if (typeof e.known !== 'boolean')
            fail('signals evidence known must be boolean');
        exact(e, e.known ? ['known', 'value'] : ['known', 'reason'], `signals evidence ${item}`);
        if (!e.known && (typeof e.reason !== 'string' || [...e.reason].length > 300))
            fail('signals reason must be a string of at most 300 characters');
    }
    const ids = m.kind === 'merge' ? mergeConditions : roundConditions;
    const conditions = exact(m.conditions, ids, 'signals conditions');
    const failed: string[] = [];
    for (const id of ids) {
        const outcome = conditions[id];
        if (outcome !== 'pass' && outcome !== 'fail' && outcome !== 'unknown')
            fail('signals condition must be pass, fail or unknown');
        if (outcome === 'pass') {
            for (const item of reads[id]!)
                if (!(evidence[item] as SignalEvidence).known)
                    fail(`signals pass ${id} reads unknown ${item}`);
        }
        else
            failed.push(id);
    }
    if (!Array.isArray(m.failed) || m.failed.length !== failed.length || m.failed.some((v, i) => v !== failed[i]))
        fail('signals failed must list exactly the non-pass conditions in order');
    return m as unknown as Signals;
}
