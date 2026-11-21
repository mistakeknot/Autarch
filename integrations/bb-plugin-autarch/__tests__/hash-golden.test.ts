import { readFileSync } from 'node:fs';
import { expect, it } from 'vitest';
import { hashes, resolveGolden } from './hash-golden-helper.js';
const golden = JSON.parse(readFileSync(new URL('../../../testdata/hash-golden.json', import.meta.url), 'utf8'));
for (const row of golden)
    it(`frozen hashes: ${row.name}`, () => expect(hashes(row)).toEqual(resolveGolden(row).expected));
