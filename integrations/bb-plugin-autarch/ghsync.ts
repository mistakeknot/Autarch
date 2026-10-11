import { createHash } from "node:crypto";

// GitHub PR sync, the pure part (plan mk-8loz7): which open PRs wait on whom. Nothing here runs a process or
// touches the network; the caller hands in rows read from `gh pr list` and gets back a classification.
// GitHub text is untrusted: titles are cleaned with `cleanTitle` before they go anywhere.

export type WaitingOn = "mk" | "vizier" | "coordinator";

export interface GhPrRow {
  repo: string; // owner/name
  number: number;
  url: string;
  title: string;
  isDraft: boolean;
  authorLogin: string;
  authorIsBot: boolean;
  reviewRequests: string[]; // logins
  reviewDecision: string; // "", APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED
  mergeable: string; // MERGEABLE, CONFLICTING, UNKNOWN
  checks: string[]; // each check's conclusion or state
  files: string[]; // changed paths
  updatedAt: string;
}

export interface Classified {
  pr: GhPrRow;
  /** null: draft, so no card. */
  waitingOn: WaitingOn | null;
  reason: string;
}

export const OWNERS = ["mistakeknot", "gensysven"] as const;
export const PR_URL = /^https:\/\/github\.com\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\/pull\/[1-9][0-9]{0,8}$/;
export const REPO = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/;

/** Paths whose merge only mk makes (the delegated-merge exclusions): instruction files, settings, hooks, CI, release scripts. */
const MK_ONLY = [
  /(^|\/)(AGENTS|CLAUDE|GEMINI)\.md$/,
  /(^|\/)\.claude\//,
  /(^|\/)\.codex\//,
  /(^|\/)hooks?\//,
  /(^|\/)settings(\.[a-z]+)?\.json$/,
  /(^|\/)\.agents\//,
  /(^|\/)SKILL\.md$/,
  /^\.github\/(workflows\/|[^/]*instructions[^/]*\.md$)/,
  /(^|\/)(release|install|publish)[^/]*$/, // any extension, or none
];

export const mkOnlyPaths = (files: readonly string[]): string[] => files.filter((f) => MK_ONLY.some((re) => re.test(f)));

/** `gh pr list --json files` returns at most this many paths per PR. */
export const FILE_CAP = 100;
export const REPO_LIMIT = 300;
export const PR_LIMIT = 100;

const FAIL = new Set(["FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "STARTUP_FAILURE", "ACTION_REQUIRED"]);
const GREEN = new Set(["SUCCESS", "NEUTRAL", "SKIPPED"]);

/** Who the PR waits on. Drafts wait on nobody (no card). The vizier column is a hint, not an authorization. */
export function classify(pr: GhPrRow): Classified {
  if (pr.isDraft) return { pr, waitingOn: null, reason: "draft" };
  // An empty or capped file list cannot clear the protected-path gate, so it goes to mk.
  if (pr.files.length === 0 || pr.files.length >= FILE_CAP) return { pr, waitingOn: "mk", reason: "changed files unknown or truncated" };
  const protectedPaths = mkOnlyPaths(pr.files);
  if (protectedPaths.length) return { pr, waitingOn: "mk", reason: `touches ${protectedPaths.slice(0, 3).map((f) => cleanTitle(f, 80)).join(", ")}` };
  if (pr.mergeable === "CONFLICTING") return { pr, waitingOn: "coordinator", reason: "conflicts with base" };
  if (pr.reviewDecision === "CHANGES_REQUESTED") return { pr, waitingOn: "coordinator", reason: "changes requested" };
  if (pr.checks.some((c) => FAIL.has(c))) return { pr, waitingOn: "coordinator", reason: "a check is failing" };
  if (pr.checks.length === 0 || !pr.checks.every((c) => GREEN.has(c))) return { pr, waitingOn: "coordinator", reason: "checks not green yet" };
  if (pr.mergeable !== "MERGEABLE") return { pr, waitingOn: "coordinator", reason: "mergeability unknown" };
  return { pr, waitingOn: "vizier", reason: "green and mergeable, no protected paths; needs review evidence and a ruling" };
}

/** One line of text with control characters removed and a length cap; GitHub text is untrusted. */
export function cleanTitle(s: string, max = 120): string {
  const t = s.replace(/[\u0000-\u001f\u007f-\u009f\u2028\u2029]/g, " ").replace(/\s+/g, " ").trim();
  return (t.length > max ? t.slice(0, max - 1) + "…" : t) || "(no title)";
}

/** The dry-run table mk reads: count by waiting-on, then one line per PR. */
export function renderDryRun(rows: readonly Classified[], unmappedRepos: readonly string[] = []): string {
  const count = (w: WaitingOn | null) => rows.filter((r) => r.waitingOn === w).length;
  const out = [
    `open PRs: ${rows.length}; would get a card: ${rows.length - count(null)} (waiting on mk ${count("mk")}, vizier ${count("vizier")}, coordinator ${count("coordinator")}); drafts, no card: ${count(null)}`,
  ];
  for (const w of ["mk", "vizier", "coordinator", null] as const) {
    const group = rows.filter((r) => r.waitingOn === w);
    if (!group.length) continue;
    out.push("", w === null ? "no card (draft)" : `waiting on ${w}`);
    for (const r of group) out.push(`  ${r.pr.url}  ${cleanTitle(r.pr.title, 70)}  [${r.reason}]`);
  }
  if (unmappedRepos.length) out.push("", `repos with no Tasks project (no card): ${unmappedRepos.join(", ")}`);
  return out.join("\n");
}

/** Maps `gh pr list --json ...` output for one repo. Throws on a malformed row so a bad read never classifies. */
export function parsePrList(repo: string, json: string): GhPrRow[] {
  if (!REPO.test(repo)) throw new Error("bad repo name");
  const v: unknown = JSON.parse(json);
  if (!Array.isArray(v)) throw new Error("gh pr list did not return an array");
  return v.map((r: Record<string, any>) => {
    if (typeof r.number !== "number" || typeof r.url !== "string" || !PR_URL.test(r.url) || !Array.isArray(r.files)) throw new Error("malformed PR row");
    return {
      repo,
      number: r.number,
      url: r.url,
      title: String(r.title ?? ""),
      isDraft: r.isDraft === true,
      authorLogin: String(r.author?.login ?? ""),
      authorIsBot: r.author?.is_bot === true,
      reviewRequests: (r.reviewRequests ?? []).map((q: any) => String(q.login ?? q.slug ?? "")),
      reviewDecision: String(r.reviewDecision ?? ""),
      mergeable: String(r.mergeable ?? "UNKNOWN"),
      checks: (r.statusCheckRollup ?? []).map((c: any) => String(c.conclusion || c.state || c.status || "")).filter(Boolean),
      files: r.files.map((f: any) => {
        if (typeof f?.path !== "string" || !f.path) throw new Error("malformed file entry");
        return f.path;
      }),
      updatedAt: String(r.updatedAt ?? ""),
    };
  });
}

/** The only gh argv this feature may use: read verbs, fixed fields. */
export const PR_FIELDS = "number,title,url,isDraft,author,reviewRequests,reviewDecision,mergeable,statusCheckRollup,files,updatedAt";
export const ghRepoListArgv = (owner: string): string[] => ["repo", "list", owner, "--no-archived", "--limit", String(REPO_LIMIT), "--json", "nameWithOwner"];
export const ghPrListArgv = (repo: string): string[] => ["pr", "list", "--repo", repo, "--state", "open", "--limit", String(PR_LIMIT), "--json", PR_FIELDS];

/** GitHub repo -> Tasks project prefix. A repo not listed gets no card and shows up as unmapped in the plan. */
export const REPO_PROJECTS: Readonly<Record<string, string>> = {
  "mistakeknot/Autarch": "AUTA",
  "mistakeknot/Clavain": "CLAV",
  "mistakeknot/Sylveste": "SYLV",
  "mistakeknot/Sylveste-lane": "SYLV",
  "mistakeknot/amtiskaw-hermes-agent": "AMTK",
  "mistakeknot/dotfiles": "DOTF",
  "mistakeknot/intercore": "ICOR",
  "mistakeknot/pattern-royale": "PR",
  "mistakeknot/ushas": "USH",
};

export interface CardSpec {
  /** The Request key the filer dedupes on: [A-Za-z0-9:_.-] only. */
  key: string;
  prefix: string;
  title: string;
  kind: "pr" | "batch";
  urls: string[];
  /** home-move/v1 body for a single PR; null for a batch. */
  move: Record<string, unknown> | null;
  /** home-ask body for a batch; null for a single PR (the filer adds a minimal ruling-only ask). */
  ask: Record<string, unknown> | null;
}

export interface CardPlan {
  cards: CardSpec[];
  /** Repos with PRs waiting on mk but no Tasks project: no card is made for them. */
  unmapped: { repo: string; urls: string[] }[];
}

const isBotCiOnly = (c: Classified): boolean => c.pr.authorIsBot && c.pr.files.length > 0 && c.pr.files.every((f) => f.startsWith(".github/workflows/"));
const BATCH_QUESTION_MAX = 1900;
const batchQuestion = (repo: string, urls: string[]): string =>
  `${urls.length} bot-opened pull requests in ${repo} change only CI workflows. Merge, close or leave them all?\n${urls.map((u) => `- ${u}`).join("\n")}`;
const keyPart = (s: string): string => s.replace(/[^A-Za-z0-9_.-]/g, "-");

/**
 * The cards to file for the PRs that wait on mk (and only those): one card per PR, except that a repo's bot-opened
 * CI-workflow-only PRs share one batched card with merge-all / close-all / leave. Pure; filing is the caller's job.
 */
export function planCards(rows: readonly Classified[]): CardPlan {
  const mine = rows.filter((r) => r.waitingOn === "mk");
  const cards: CardSpec[] = [];
  const unmapped = new Map<string, string[]>();
  const batches = new Map<string, Classified[]>();
  for (const r of mine) {
    const prefix = REPO_PROJECTS[r.pr.repo];
    if (!prefix) {
      unmapped.set(r.pr.repo, [...(unmapped.get(r.pr.repo) ?? []), r.pr.url]);
      continue;
    }
    if (isBotCiOnly(r)) {
      batches.set(r.pr.repo, [...(batches.get(r.pr.repo) ?? []), r]);
      continue;
    }
    const [owner, name] = r.pr.repo.split("/") as [string, string];
    cards.push({
      key: `gh-pr:${keyPart(owner)}:${keyPart(name)}:${r.pr.number}`,
      prefix,
      title: `Merge or decide: ${r.pr.repo}#${r.pr.number} ${cleanTitle(r.pr.title, 80)}`,
      kind: "pr",
      urls: [r.pr.url],
      move: { schema: "home-move/v1", kind: "pr", pr: { url: r.pr.url, summary: `${r.pr.repo}#${r.pr.number} is waiting on you` } },
      ask: null,
    });
  }
  for (const [repo, group] of batches) {
    const [owner, name] = repo.split("/") as [string, string];
    // The question lists every URL and must stay under the ask parser's 2000-character limit, so a big set is split.
    const chunks: string[][] = [];
    for (const url of group.map((g) => g.pr.url).sort()) {
      const cur = chunks[chunks.length - 1];
      if (cur && batchQuestion(repo, [...cur, url]).length <= BATCH_QUESTION_MAX) cur.push(url);
      else chunks.push([url]);
    }
    for (const urls of chunks) {
      const hash = createHash("sha256").update(urls.join("\n")).digest("hex").slice(0, 8);
      cards.push({
        key: `gh-bots:${keyPart(owner)}:${keyPart(name)}:${hash}`,
        prefix: REPO_PROJECTS[repo]!,
        title: `${urls.length} dependency bumps to CI workflows in ${repo}`,
        kind: "batch",
        urls,
        move: null,
        ask: {
          question: batchQuestion(repo, urls),
          options: [
            { id: "merge-all", label: "Merge all", kind: "ruling-only" },
            { id: "close-all", label: "Close all", kind: "ruling-only" },
            { id: "leave", label: "Leave", kind: "ruling-only" },
          ],
        },
      });
    }
  }
  cards.sort((a, b) => (a.key < b.key ? -1 : 1));
  return { cards, unmapped: [...unmapped].map(([repo, urls]) => ({ repo, urls })) };
}

/** The plan as text for mk and the vizier: counts first, then one line per card, then repos with no project. */
export function renderPlan(plan: CardPlan): string {
  const out = [`cards to file: ${plan.cards.length} (${plan.cards.filter((c) => c.kind === "batch").length} batched, covering ${plan.cards.filter((c) => c.kind === "batch").reduce((n, c) => n + c.urls.length, 0)} PRs)`];
  for (const c of plan.cards) out.push(`  [${c.prefix}] ${c.kind === "batch" ? `batch of ${c.urls.length}` : "PR"}  ${c.title}  (${c.key})`);
  if (plan.unmapped.length) out.push("", "no Tasks project, no card: " + plan.unmapped.map((u) => `${u.repo} (${u.urls.length})`).join(", "));
  return out.join("\n");
}
