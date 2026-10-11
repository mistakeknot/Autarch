// Plans (and, with --apply, files) Home cards for open PRs that wait on mk. Default is a dry run: it prints the plan and
// files nothing. --apply files each card through `autarch needs-mk file`, which dedupes on the Request key, so a re-run
// files nothing twice. Needs BB_THREAD_ID (the card routes its picks there) and --roots <json> mapping each Tasks prefix to
// its Home project name and root: {"AUTA":{"project":"autarch","root":"/abs/path"}}; a card with no entry is skipped.
// Usage: npx tsx scripts/gh-sync-cards.ts [--roots file.json] [--apply]
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { OWNERS, PR_LIMIT, REPO_LIMIT, REPO, classify, ghPrListArgv, ghRepoListArgv, parsePrList, planCards, renderPlan, type Classified } from "../ghsync.js";

const argValue = (n: string) => (process.argv.includes(n) ? process.argv[process.argv.indexOf(n) + 1] : undefined);
const gh = (argv: string[]) => execFileSync("gh", argv, { encoding: "utf8", maxBuffer: 32 * 1024 * 1024, timeout: 60_000, env: { ...process.env, GH_PROMPT_DISABLED: "1", NO_COLOR: "1" } });
const rows: Classified[] = [];
const notRead: string[] = [];
for (const owner of OWNERS) {
  const repos = (JSON.parse(gh(ghRepoListArgv(owner))) as { nameWithOwner: string }[]).map((r) => r.nameWithOwner).filter((n) => REPO.test(n));
  if (repos.length >= REPO_LIMIT) notRead.push(`${owner}: repo list hit the ${REPO_LIMIT} limit`);
  for (const repo of repos) {
    try {
      const prs = parsePrList(repo, gh(ghPrListArgv(repo)));
      if (prs.length >= PR_LIMIT) notRead.push(`${repo}: hit the ${PR_LIMIT} PR limit`);
      rows.push(...prs.map(classify));
    } catch (e) {
      notRead.push(`${repo}: ${String((e as Error).message).split("\n")[0]}`);
    }
  }
}
const plan = planCards(rows);
console.log(renderPlan(plan));
if (notRead.length) console.log(`\nnot read (${notRead.length}):\n  ${notRead.join("\n  ")}`);
if (!process.argv.includes("--apply")) process.exit(0);

// Applying: refuse when any list was incomplete, so a partial read never files a partial set.
if (notRead.length) throw new Error("some lists were not fully read; nothing filed");
const rootsFile = argValue("--roots");
if (!rootsFile) throw new Error("--apply needs --roots <json>");
const roots = JSON.parse(readFileSync(rootsFile, "utf8")) as Record<string, { project: string; root: string }>;
const dir = mkdtempSync(join(tmpdir(), "gh-sync-"));
for (const c of plan.cards) {
  const r = roots[c.prefix];
  if (!r) {
    console.log(`skip ${c.key}: no root for ${c.prefix}`);
    continue;
  }
  const args = ["needs-mk", "file", "--project", c.prefix, "--title", c.title, "--request", c.key];
  if (c.move) {
    writeFileSync(join(dir, "move.json"), JSON.stringify(c.move));
    args.push("--move", join(dir, "move.json"));
  } else {
    writeFileSync(join(dir, "ask.json"), JSON.stringify({ ...c.ask, project: r.project, project_root: r.root }));
    args.push("--ask-file", join(dir, "ask.json"));
  }
  console.log(execFileSync("autarch", args, { encoding: "utf8", timeout: 120_000 }).trim());
}
