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
export function renderDryRun(all: readonly Classified[], unmappedRepos: readonly string[] = []): string {
  const seen = new Set<string>();
  const rows = all.filter((r) => !seen.has(r.pr.url) && seen.add(r.pr.url));
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
  "mistakeknot/interbase": "IBAS",
  "mistakeknot/interkasten": "IKAS",
  "mistakeknot/intermute": "IMUT",
  "mistakeknot/intermute-lane": "IMUL",
  "mistakeknot/interphase": "IPHS",
  "mistakeknot/pattern-royale": "PR",
  "mistakeknot/ushas": "USH",
};

export interface CardSpec {
  /** The Request key the filer dedupes on: [A-Za-z0-9:_.-] only. */
  key: string;
  prefix: string;
  title: string;
  kind: "pr" | "batch" | "unmapped";
  urls: string[];
  /** home-move/v1 body for a single PR; null for a batch. */
  move: Record<string, unknown> | null;
  /** home-ask body: merge / close / leave for every kind. */
  ask: Record<string, unknown>;
}

export interface CardPlan {
  cards: CardSpec[];
  /** Repos with PRs waiting on mk but no Tasks project: no card is made for them. */
  unmapped: { repo: string; urls: string[] }[];
}

const prQuestion = (repo: string, n: number, title: string, url: string): string =>
  `${url}\n${repo}#${n} ${cleanTitle(title, 80)} waits on you. Merge it, close it or leave it?`;
/** Key an earlier gh-sync filed a single-PR card under (a Noted / Not now ask); an open one stands in for the new card. */
export const legacyPrKey = (key: string): string => key.replace(/^gh-ask:/, "gh-pr:");
const isBotCiOnly = (c: Classified): boolean => c.pr.authorIsBot && c.pr.files.length > 0 && c.pr.files.every((f) => f.startsWith(".github/workflows/"));
const BATCH_QUESTION_MAX = 1900;
const batchQuestion = (repo: string, urls: string[]): string =>
  `${urls.length} bot-opened pull requests in ${repo} change only CI workflows. Merge, close or leave them all?\n${urls.map((u) => `- ${u}`).join("\n")}`;
const chunkUrls = (question: (urls: string[]) => string, all: string[]): string[][] => {
  const chunks: string[][] = [];
  for (const url of [...all].sort()) {
    const cur = chunks[chunks.length - 1];
    if (cur && question([...cur, url]).length <= BATCH_QUESTION_MAX) cur.push(url);
    else chunks.push([url]);
  }
  return chunks;
};
const urlHash = (urls: string[]): string => createHash("sha256").update(urls.join("\n")).digest("hex").slice(0, 8);
/** Prefix of the catch-all project: mk-waiting PRs in repos with no Tasks project get one card per repo here. */
export const CATCH_ALL_PREFIX = "MSQ";
const unmappedQuestion = (repo: string, urls: string[]): string =>
  `${urls.length} open pull request${urls.length === 1 ? "" : "s"} in ${repo} wait on you, and Home has no project to file ${repo} under. Look at them, close them or leave them?\n${urls.map((u) => `- ${u}`).join("\n")}`;
/** Filer-safe and short: the filer takes keys of at most 128 characters, and a repo name can be 100. */
const keyPart = (s: string): string => {
  const clean = s.replace(/[^A-Za-z0-9_.-]/g, "-");
  // A long name keeps a hash of the whole name, so two names that differ only after the cut still get different keys.
  return clean.length <= 50 && clean === s ? clean : `${clean.slice(0, 40)}-${createHash("sha256").update(s).digest("hex").slice(0, 8)}`;
};
/** A set-keyed card key minus its trailing hash: what an earlier card for the same repo shares with this one. */
export const keyFamily = (key: string): string => key.replace(/:[0-9a-f]{8}$/, ":");

/**
 * The cards to file for the PRs that wait on mk (and only those): one card per PR, except that a repo's bot-opened
 * CI-workflow-only PRs share one batched card with merge-all / close-all / leave. Pure; filing is the caller's job.
 */
export function planCards(rows: readonly Classified[], bound?: ReadonlySet<string>): CardPlan {
  // A PR read twice (a repo listed under two owners or twice in one list) is one PR.
  const seenUrls = new Set<string>();
  const mine = rows.filter((r) => r.waitingOn === "mk" && !seenUrls.has(r.pr.url) && seenUrls.add(r.pr.url));
  const cards: CardSpec[] = [];
  const unmapped = new Map<string, string[]>();
  const batches = new Map<string, Classified[]>();
  for (const r of mine) {
    const prefix = REPO_PROJECTS[r.pr.repo];
    // A prefix Home cannot file to (not bound, when the caller knows which are) goes to the catch-all like an unmapped repo.
    if (!prefix || (bound && !bound.has(prefix))) {
      unmapped.set(r.pr.repo, [...(unmapped.get(r.pr.repo) ?? []), r.pr.url]);
      continue;
    }
    if (isBotCiOnly(r)) {
      batches.set(r.pr.repo, [...(batches.get(r.pr.repo) ?? []), r]);
      continue;
    }
    const [owner, name] = r.pr.repo.split("/") as [string, string];
    cards.push({
      key: `gh-ask:${keyPart(owner)}:${keyPart(name)}:${r.pr.number}`,
      prefix,
      title: `Merge or decide: ${r.pr.url} ${cleanTitle(r.pr.title, 60)}`,
      kind: "pr",
      urls: [r.pr.url],
      // The move stays so a PR merged or closed on GitHub still clears the card; the Queue shows only the decision.
      move: { schema: "home-move/v1", kind: "pr", pr: { url: r.pr.url } },
      ask: {
        question: prQuestion(r.pr.repo, r.pr.number, r.pr.title, r.pr.url),
        options: [
          { id: "merge", label: "Merge it", kind: "ruling-only" },
          { id: "close", label: "Close it", kind: "ruling-only" },
          { id: "leave", label: "Leave", kind: "ruling-only" },
        ],
      },
    });
  }
  for (const [repo, group] of batches) {
    const [owner, name] = repo.split("/") as [string, string];
    // The question lists every URL and must stay under the ask parser's 2000-character limit, so a big set is split.
    const chunks = chunkUrls((u) => batchQuestion(repo, u), group.map((g) => g.pr.url));
    for (const urls of chunks) {
      const hash = urlHash(urls);
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
  for (const [repo, all] of unmapped) {
    const [owner, name] = repo.split("/") as [string, string];
    for (const urls of chunkUrls((u) => unmappedQuestion(repo, u), all)) {
      cards.push({
        key: `gh-unmapped:${keyPart(owner)}:${keyPart(name)}:${urlHash(urls)}`,
        prefix: CATCH_ALL_PREFIX,
        title: `${urls.length} open PR${urls.length === 1 ? "" : "s"} in ${repo} (no project of its own)`,
        kind: "unmapped",
        urls,
        move: null,
        ask: {
          question: unmappedQuestion(repo, urls),
          options: [
            { id: "look", label: "Look at them", kind: "ruling-only" },
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
  const n = (k: CardSpec["kind"]) => plan.cards.filter((c) => c.kind === k);
  const covered = (k: CardSpec["kind"]) => n(k).reduce((t, c) => t + c.urls.length, 0);
  const out = [`cards to file: ${plan.cards.length} (${n("batch").length} batched, covering ${covered("batch")} PRs; ${n("unmapped").length} catch-all under ${CATCH_ALL_PREFIX}, covering ${covered("unmapped")} PRs)`];
  for (const c of plan.cards) out.push(`  [${c.prefix}] ${c.kind === "pr" ? "PR" : `${c.kind} of ${c.urls.length}`}  ${c.title}  (${c.key})`);
  if (plan.unmapped.length) out.push("", `no Tasks project of their own, filed under ${CATCH_ALL_PREFIX}: ` + plan.unmapped.map((u) => `${u.repo} (${u.urls.length})`).join(", "));
  return out.join("\n");
}

/** The key on a card's own `Request: <key> sha256:<id>` line; a description that has none is not a filed card. */
export const requestKey = (desc: string): string | null => /^Request: (\S+) sha256:\S+$/m.exec(desc)?.[1] ?? null;

/** Descriptions from one `bb tasks list --json` page. A row without a description cannot be checked, so the read fails. */
export function pageDescriptions(page: { tasks: { description?: unknown }[] }): string[] {
  if (page.tasks.some((t) => typeof t.description !== "string")) throw new Error("task list rows carry no description");
  return page.tasks.map((t) => t.description as string);
}

/** Rejects a cursor already seen, so a server that repeats one cannot loop the read forever. */
export function nextCursor(seen: Set<string>, cursor: string | null | undefined): string | null {
  if (cursor === null) return null;
  if (typeof cursor !== "string") throw new Error("task list page has no nextCursor");
  if (seen.has(cursor)) throw new Error("task list repeated a cursor");
  seen.add(cursor);
  return cursor;
}

/** Splits argv into known flags; an unknown flag is an error rather than a silent dry run. */
export function parseSyncArgs(argv: string[]): { apply: boolean; roots?: string } {
  let apply = false;
  let roots: string | undefined;
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--apply") apply = true;
    else if (argv[i] === "--roots" && argv[i + 1] !== undefined) roots = argv[++i];
    else throw new Error(`unknown argument ${argv[i]}; usage: gh-sync-cards.ts [--roots file.json] [--apply]`);
  }
  return { apply, roots };
}

/** True when the text lists this exact PR URL (`/pull/1` does not match `/pull/10`). */
export const mentionsPr = (text: string, url: string): boolean => text.split(/[\s,()<>"']+/).includes(url);
