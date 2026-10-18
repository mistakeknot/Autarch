// Read-only dry run of the GitHub PR sync: lists what would get a card and who each PR waits on.
// Runs `gh repo list` and `gh pr list` only (argv arrays, no shell). Creates and changes nothing.
// Usage: npx tsx scripts/gh-sync-dryrun.ts [--json]
import { gh } from "./gh-util.js";
import { OWNERS, PR_LIMIT, REPO_LIMIT, classify, ghPrListArgv, ghRepoListArgv, parsePrList, renderDryRun, REPO } from "../ghsync.js";

const rows = [];
const failed: string[] = [];
for (const owner of OWNERS) {
  const repos = (JSON.parse(gh(ghRepoListArgv(owner))) as { nameWithOwner: string }[]).map((r) => r.nameWithOwner).filter((n) => REPO.test(n));
  if (repos.length >= REPO_LIMIT) failed.push(`${owner}: repo list hit the ${REPO_LIMIT} limit; counts are incomplete`);
  for (const repo of repos) {
    try {
      const prs = parsePrList(repo, gh(ghPrListArgv(repo)));
      if (prs.length >= PR_LIMIT) failed.push(`${repo}: hit the ${PR_LIMIT} PR limit; counts are incomplete`);
      rows.push(...prs.map(classify));
    } catch (e) {
      failed.push(`${repo}: ${String((e as Error).message).split("\n")[0]}`);
    }
  }
}
if (process.argv.includes("--json")) {
  console.log(JSON.stringify({ rows, notRead: failed }, null, 1));
} else {
  console.log(renderDryRun(rows));
  if (failed.length) console.log(`\nnot read (${failed.length}):\n  ${failed.join("\n  ")}`);
}
