// Pinned Vitest 3 metadata query: transform import graphs, never collect or run tests.
// Failure or an unsupported API makes the caller select the full suite.
import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
const require = createRequire(resolve('package.json'));
const { createVitest } = await import(pathToFileURL(require.resolve('vitest/node')));
const ctx = await createVitest('test', { watch: false, maxWorkers: 2, related: process.argv.slice(2) });
try {
  const specs = await ctx.specifications.getRelevantTestSpecifications();
  console.log(JSON.stringify(specs.map(s => s.moduleId)));
} finally {
  await ctx.close();
}
