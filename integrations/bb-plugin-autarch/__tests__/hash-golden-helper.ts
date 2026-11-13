import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { parseCard, toV1, cardFingerprint } from '../cards.js';
import { canonicalJson, parseExistingAsk, identity, revision, semanticKey, normalizedJson } from '../model.js';
import { Service } from '../service.js';
export function resolveGolden(row: any) {
    if (row.ask_fixture) {
        const fixture = JSON.parse(readFileSync(new URL('../../../schema/home-ask/v1/vectors.json', import.meta.url), 'utf8')).asks.find((c: any) => c.name === row.ask_fixture);
        return {...row, ask: fixture.input, expected: {...row.expected, normalized: fixture.normalized}};
    }
    return row;
}
export function hashes(row: any, metadata: Record<string, unknown> = {}) {
    row = resolveGolden(row);
    if (row.fixture_file)
        row = { ...row, description: JSON.parse(readFileSync(new URL(`fixtures/cards/${row.fixture_file}`, import.meta.url), 'utf8')).cases.find((c: any) => c.name === row.fixture_name).description };
    let card: any, ask: any, wire: any;
    if (row.description) {
        const desc = row.description.replace(/```home-ask\n([\s\S]*?)\n```/, (_: string, body: string) => '```home-ask\n' + JSON.stringify({ ...JSON.parse(body), ...metadata }) + '\n```');
        card = parseCard(desc);
        wire = { ...card.ask };
        ask = toV1(card, row.thread);
        if (row.generation > 1) {
            // The real generation projection, without constructing a live service.
            const input = (Service.prototype as any).genInput.call({}, { id: 'fixture', projectId: 'fixture-project' }, card, row.thread, 'fp', row.generation, `card-fixture-g${row.generation - 1}`, { project_root: '/fixture', root_dev: '1', root_ino: '2', root_fsid: null });
            ask = parseExistingAsk(JSON.parse(input.body_json));
        }
    }
    else {
        ask = parseExistingAsk(row.ask);
        wire = { ...row.ask, schema: 'home-ask/v2' };
    }
    delete wire.evidence;
    delete wire.group;
    delete wire.subject_key;
    delete wire.signals;
    const blocks = card ? [...new Set(card.blocks.map((b: any) => b.ref))].sort() : [];
    const node: any = { project: wire.project, title: 'Fixture', blocks, ask: wire };
    if (card?.root_run)
        node.root_run = card.root_run;
    if (card?.move)
        node.move = card.move;
    return {
        h1: createHash('sha256').update(canonicalJson(node)).digest('hex').slice(0, 16),
        identity: identity(ask), revision: revision(ask), semantic_digest: semanticKey(ask), normalized: normalizedJson(ask),
        h5: card ? cardFingerprint({ title: 'Fixture', card, thread: row.thread, tasksProject: 'fixture-project' }) : null,
    };
}
