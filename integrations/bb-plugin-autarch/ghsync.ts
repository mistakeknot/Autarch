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
      files: r.files.map((f: any) => String(f.path ?? "")),
      updatedAt: String(r.updatedAt ?? ""),
    };
  });
}

/** The only gh argv this feature may use: read verbs, fixed fields. */
export const PR_FIELDS = "number,title,url,isDraft,author,reviewRequests,reviewDecision,mergeable,statusCheckRollup,files,updatedAt";
export const ghRepoListArgv = (owner: string): string[] => ["repo", "list", owner, "--no-archived", "--limit", String(REPO_LIMIT), "--json", "nameWithOwner"];
export const ghPrListArgv = (repo: string): string[] => ["pr", "list", "--repo", repo, "--state", "open", "--limit", String(PR_LIMIT), "--json", PR_FIELDS];
