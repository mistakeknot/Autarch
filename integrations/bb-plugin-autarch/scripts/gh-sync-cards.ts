// Plans (and, with --apply, files) Home cards for open PRs that wait on mk. Default is a dry run: it prints the plan and
// files nothing. --apply files each card through `autarch needs-mk file`, which dedupes on the Request key, so a re-run
// files nothing twice. Needs BB_THREAD_ID (the card routes its picks there) and --roots <json> mapping each Tasks prefix to
// its Home project name and root: {"AUTA":{"project":"autarch","root":"/abs/path"}}; a card with no entry is skipped.
// Usage: npx tsx scripts/gh-sync-cards.ts [--roots file.json] [--apply]
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { OWNERS, keyFamily, PR_LIMIT, REPO_LIMIT, REPO, classify, ghPrListArgv, ghRepoListArgv, parsePrList, planCards, renderPlan, type Classified } from "../ghsync.js";

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
/** Descriptions of the open cards in a Tasks project (every page), cached per run so cards filed here do not hold their siblings. */
const openCache = new Map<string, string[] | null>();
const openCards = (prefix: string): string[] | null => {
  if (openCache.has(prefix)) return openCache.get(prefix)!;
  const out: string[] = [];
  try {
    let cursor: string | null = null;
    do {
      const args = ["tasks", "list", "--project", prefix, "--status", "backlog,todo,in_progress,in_review", "--search", "Request:", "--limit", "500", "--json", ...(cursor ? ["--cursor", cursor] : [])];
      const page = JSON.parse(execFileSync("bb", args, { encoding: "utf8", timeout: 60_000, maxBuffer: 32 * 1024 * 1024 })) as { tasks: { description?: string }[]; nextCursor: string | null };
      out.push(...page.tasks.map((t) => t.description ?? ""));
      cursor = page.nextCursor;
    } while (cursor);
    openCache.set(prefix, out);
  } catch {
    openCache.set(prefix, null);
  }
  return openCache.get(prefix)!;
};
const failed: string[] = [];
const held: string[] = [];
const dir = mkdtempSync(join(tmpdir(), "gh-sync-"));
for (const c of plan.cards) {
  const r = roots[c.prefix];
  if (!r) {
    console.log(`skip ${c.key}: no root for ${c.prefix}`);
    continue;
  }
  const args = ["needs-mk", "file", "--project", c.prefix, "--title", c.title, "--request", c.key];
  // Every card carries its own project and root, so a PR in another repo never takes the checkout's.
  const ask = c.ask ?? { question: c.title, options: [{ id: "noted", label: "Noted", kind: "ruling-only" }, { id: "not-now", label: "Not now", kind: "ruling-only" }] };
  writeFileSync(join(dir, "ask.json"), JSON.stringify({ ...ask, project: r.project, project_root: r.root }));
  args.push("--ask-file", join(dir, "ask.json"));
  if (c.move) {
    writeFileSync(join(dir, "move.json"), JSON.stringify(c.move));
    args.push("--move", join(dir, "move.json"));
  }
  if (c.kind !== "pr") {
    // A batch key follows its PR set, so a changed set would file a second card beside the open one. Hold the repo while an
    // open card of the same family is not one of this run's cards; sibling chunks of this run never hold each other.
    const fam = keyFamily(c.key);
    const mine = new Set(plan.cards.filter((x) => keyFamily(x.key) === fam).map((x) => x.key));
    const list = openCards(c.prefix);
    if (!list) {
      failed.push(c.key);
      console.log(`FAILED ${c.key}: could not check for an open card of the same repo`);
      continue;
    }
    const inFamily = list.filter((d) => d.includes(fam));
    if (inFamily.some((d) => d.includes(c.key))) { console.log(`already filed ${c.key}`); continue; }
    if (inFamily.some((d) => ![...mine].some((k) => d.includes(k)))) { console.log(`HELD ${c.key}: an open card for an earlier set of this repo exists; close it, then re-run`); held.push(c.key); continue; }
  }
  try {
    console.log(execFileSync("autarch", args, { encoding: "utf8", timeout: 120_000 }).trim());
  } catch (e) {
    // One refused card (for example a PR whose title changed since it was filed) must not stop the rest.
    failed.push(c.key);
    console.log(`FAILED ${c.key}: ${String((e as Error).message).split("\n")[0]}`);
  }
}
if (held.length) console.log(`\n${held.length} card(s) held because an older card for the same repo is still open.`);
if (failed.length) {
  console.log(`\n${failed.length} card(s) not filed; the rest were. Re-running files only what is missing.`);
  process.exitCode = 1;
}
console.log("Note: a batch or catch-all card is keyed on its PR set; a changed set is held until the older card is closed.");
