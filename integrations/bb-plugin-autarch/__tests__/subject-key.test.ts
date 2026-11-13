import { readFileSync } from 'node:fs';
import { it, expect } from 'vitest';
import { parseCard, toV1 } from '../cards.js';
import { parseAsk, identity } from '../model.js';
import { hashes } from './hash-golden-helper.js';
const vectors = JSON.parse(readFileSync(new URL('../../../testdata/subject-key-vectors.json', import.meta.url), 'utf8'));
const golden = JSON.parse(readFileSync(new URL('../../../testdata/hash-golden.json', import.meta.url), 'utf8'));
it('at least 60 shared vectors', () => expect(vectors.length).toBeGreaterThanOrEqual(60));
for (const v of vectors)
    it(`subject vector ${v.name}`, () => {
        const ask = { schema: 'home-ask/v2', project: 'fixture', project_root: '/fixture', question: 'Choice?', options: [{ id: 'a', label: 'A', kind: 'ruling-only' }, { id: 'b', label: 'B', kind: 'ruling-only' }], ...v.metadata };
        let desc = 'Request: fixture sha256:0123456789abcdef\n```home-ask\n' + JSON.stringify(ask) + '\n```';
        if (v.root_run)
            desc += '\n```root-run\n' + Object.entries(v.root_run).map(([k, x]) => `${k}: ${x}`).join('\n') + '\n```';
        if (v.valid) {
            const c = parseCard(desc);
            expect((c as any).subject_key ?? null).toEqual(v.metadata.subject_key ?? null);
            expect((c as any).signals ?? null).toEqual(v.metadata.signals ?? null);
        }
        else
            expect(() => parseCard(desc)).toThrow();
    });
for (const row of golden.filter((r: any) => r.fixture_file))
    it(`metadata frozen ${row.name}`, () => expect(hashes(row, { subject_key: vectors[1].metadata.subject_key, signals: vectors[1].metadata.signals })).toEqual(row.expected));
it('v1 rejects metadata and keeps legacy subject', () => {
    const card = parseCard('Request: fixture sha256:0123456789abcdef\n```home-ask\n' + JSON.stringify({ schema: 'home-ask/v2', project: 'fixture', project_root: '/fixture', question: 'Choice?', subject: 'legacy A', options: [{ id: 'a', label: 'A', kind: 'ruling-only' }, { id: 'b', label: 'B', kind: 'ruling-only' }] }) + '\n```');
    const a = toV1(card, 'thr_fixture');
    for (const key of ['subject_key', 'signals'])
        expect(() => parseAsk({ ...a, [key]: null })).toThrow();
    expect(identity(a)).not.toBe(identity(parseAsk({ ...a, subject: 'legacy B' })));
    expect(hashes({ description: 'Request: fixture sha256:0123456789abcdef\n```home-ask\n' + JSON.stringify(card.ask) + '\n```', thread: 'thr_fixture' }).h1).not.toBe(hashes({ description: 'Request: fixture sha256:0123456789abcdef\n```home-ask\n' + JSON.stringify({ ...card.ask, subject: 'legacy B' }) + '\n```', thread: 'thr_fixture' }).h1);
});
