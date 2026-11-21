import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

/** Actual pre-PR1 reader source pinned by the checked-in fixture manifest. */
export const V6_COMMIT = 'fce590be80157a03ebd89e203ea324fd9bfb7717';

export async function loadV6() {
  const root = new URL('./fixtures/v6-reader/', import.meta.url);
  const manifest = readFileSync(new URL('MANIFEST.sha256', root), 'utf8');
  for (const line of manifest.trimEnd().split('\n')) {
    const match = /^([a-f0-9]{64})  (.+)$/.exec(line);
    if (!match) throw new Error('Invalid v6 fixture manifest entry');
    const [, expected, relative] = match;
    if (relative.startsWith('/') || relative.split('/').includes('..')) {
      throw new Error('Invalid v6 fixture path');
    }
    const path = new URL(relative, root);
    const actual = createHash('sha256').update(readFileSync(path)).digest('hex');
    if (actual !== expected) throw new Error(`V6 fixture digest mismatch: ${fileURLToPath(path)}`);
  }
  const [store, wakes, model, migrations] = await Promise.all([
    import('./fixtures/v6-reader/store.js'),
    import('./fixtures/v6-reader/wakes.js'),
    import('./fixtures/v6-reader/model.js'),
    import('./fixtures/v6-reader/migrations.js'),
  ]);
  return { Store: store.Store, WakeLoop: wakes.WakeLoop, parseExistingAsk: model.parseExistingAsk, ...migrations };
}
