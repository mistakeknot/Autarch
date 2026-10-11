// Read-only dry run of the GitHub PR sync: lists what would get a card and who each PR waits on.
// Runs `gh repo list` and `gh pr list` only (argv arrays, no shell). Creates and changes nothing.
// Usage: npx tsx scripts/gh-sync-dryrun.ts [--json]
import { execFileSync } from "node:child_process";
import { OWNERS, classify, ghPrListArgv, ghRepoListArgv, parsePrList, renderDryRun, REPO } from "../ghsync.js";

const gh = (argv: string[]) => execFileSync("gh", argv, { encoding: "utf8", maxBuffer: 32 * 1024 * 1024, timeout: 60_000, env: { ...process.env, GH_PROMPT_DISABLED: "1", NO_COLOR: "1" } });
const rows = [];
const failed: string[] = [];
for (const owner of OWNERS) {
  const repos = (JSON.parse(gh(ghRepoListArgv(owner))) as { nameWithOwner: string }[]).map((r) => r.nameWithOwner).filter((n) => REPO.test(n));
  for (const repo of repos) {
    try {
      rows.push(...parsePrList(repo, gh(ghPrListArgv(repo))).map(classify));
    } catch (e) {
      failed.push(`${repo}: ${String((e as Error).message).split("\n")[0]}`);
    }
  }
}
console.log(process.argv.includes("--json") ? JSON.stringify(rows, null, 1) : renderDryRun(rows));
if (failed.length) console.log(`\nnot read (${failed.length}):\n  ${failed.join("\n  ")}`);
