// Merge recheck (bead mk-g1t73): when mk picks a merge option on a card and the named PR is still not merged
// five minutes later, the card comes back as a new generation. No model is involved: a timer, one read-only
// `gh pr view` per due pick, and the existing bring-back. mk's pick stays visible on the earlier generation.
//
// Only a pick whose option is itself a merge instruction is rechecked (see REWORK and MERGE_LEAD below).
// Which PR: the picked option's own text if it names a PR URL, else the card's one PR URL, else the merge
// approval's target (owner/repo#N). A card that names several different PRs is left alone rather than guessed at.
// Two looks (bead mk-qepye): the registrar's merge can land a few minutes after the five-minute check, and GitHub shows
// nothing in flight before it does. So the five-minute look only records that the PR was still open; the card comes back
// only if the PR is still open at a second look ten minutes after the pick. Merged or closed at either look settles it.
// Once per generation: the new generation has no pick, so it is checked only if mk picks merge on it again, and
// a card that has already been resurfaced MAX_GENERATIONS times is left alone.
// A card whose task has since been canceled or deleted is finished (bead mk-6k39i): the pick's own write-back only ever
// completes a task (done), so a canceled one was closed on purpose by a person or a thread, and the recheck leaves it.
// A done task is the normal state five minutes after a pick and still gets the check. The manual Bring back on the
// Archive tab is a separate path and keeps reopening a canceled task.
import type Database from "better-sqlite3";
import type { GhView } from "./movepoll.js";
import type { TaskState } from "./tasks.js";

export const RECHECK_AFTER_MS = 5 * 60_000;
/** A merge pick is brought back only if the PR is still open at a second look this long after the pick (bead mk-qepye). */
export const RECHECK_SECOND_LOOK_MS = 10 * 60_000;
/** Picks older than this are not rechecked (a Citadel outage longer than this skips the check). */
export const RECHECK_WINDOW_MS = 2 * 60 * 60_000;
export const RECHECK_POLL_MS = 60_000;
export const MAX_GENERATIONS = 4;

const PR_IN_TEXT = /https:\/\/github\.com\/([A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+)\/pull\/([1-9][0-9]{0,8})/g;
const APPROVAL_TARGET = /^([A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+)#([1-9][0-9]{0,8})$/;
const NOT_MERGE = /\b(do not|don't|dont|not|never|won't|no)\b[^.\n]{0,20}\bmerg/i;
// A merge pick is one whose own instruction is to merge (bead mk-q9m7q). Mentioning a merge is not enough: a fix-round
// option says "a pass goes to the registrar to merge" and must not be rechecked as a merge. The picked option counts as
// a merge when its label or its instruction opens with the merge as the action and is not rework (fix, round, review, another, ...)
// ("Merge PR 130", "mk merges ...", "Merge the PR ...", never the noun in "Merge readiness ..."; rework words rule it out), or when the pick minted a merge approval.
const REWORK = /\b(fix(?:es|ed|ing)?|rounds?|(?:re-?)?reviews?|rework|revis(?:e|ed|ion)|another|one more|extra|again|retry)\b/i;
const MERGE_LEAD = /^\W*(?:(?:yes|ok|okay|approve)\W+(?:and\s+)?)?(?:please\s+)?(?:(?:squash|rebase)(?:\s+and)?\s+)?(?:(?:mk|the vizier|vizier|the registrar|registrar|you|we)\s+)?(?:will\s+)?merges?(?=\s+(?:(?:it|this|that|them|the|pr|pull|at|in|now|with|using|via)\b|#|\d|https?:|--|[\w.-]+\/[\w.-]+)|\s*[.,;:!]|\s*$)/i;
const firstSentence = (t: string): string => t.trim().split(/(?<=[.!?])\s|\n/)[0] ?? "";
const DEFER = /\b(wait|hold|defer|postpone|delay|until)\b|\bnot yet\b|\bbefore\s+(?:you\s+)?(?:auto-?)?merg/i;

export interface PrRef {
  repo: string;
  number: number;
  url: string;
}

const ref = (repo: string, n: string | number): PrRef => ({ repo, number: Number(n), url: `https://github.com/${repo}/pull/${n}` });

function urlsIn(text: string): PrRef[] {
  const seen = new Map<string, PrRef>();
  for (const m of text.matchAll(PR_IN_TEXT)) {
    const r = ref(m[1]!, m[2]!);
    seen.set(r.url, r);
  }
  return [...seen.values()];
}

/** The PR a merge pick is about, or null when the pick is not a merge or the PR cannot be told apart. */
export function mergePrOf(bodyJson: string, optionId: string, approvalTarget: string | null): PrRef | null {
  let body: { question?: unknown; options?: unknown };
  try {
    body = JSON.parse(bodyJson);
  } catch {
    return null;
  }
  const opts = Array.isArray(body.options) ? (body.options as Record<string, unknown>[]) : [];
  const opt = opts.find((o) => o && o.id === optionId);
  if (!opt) return null;
  const label = typeof opt.label === "string" ? opt.label : "";
  const instruction = typeof opt.instruction === "string" ? opt.instruction : "";
  const text = `${label}\n${instruction}`;
  const approval = approvalTarget ? APPROVAL_TARGET.exec(approvalTarget) : null;
  if (NOT_MERGE.test(text) || DEFER.test(text)) return null;
  const leads = (t: string): boolean => MERGE_LEAD.test(firstSentence(t)) && !REWORK.test(firstSentence(t));
  const isMerge = leads(label) || leads(instruction);
  if (!isMerge && approval === null) return null;
  const own = urlsIn(text);
  if (own.length === 1) return own[0]!;
  if (own.length > 1) return null;
  if (approval) return ref(approval[1]!, approval[2]!);
  const card = urlsIn(typeof body.question === "string" ? body.question : "");
  return card.length === 1 ? card[0]! : null;
}

export interface DuePick {
  decision_id: string;
  task_id: string;
  generation: number;
  option_id: string;
  body_json: string;
  picked_at: string;
  approval_target: string | null;
}

/** Current-generation card picks inside the window and old enough to check. */
export function duePicks(db: Database.Database, nowMs: number): DuePick[] {
  const newest = new Date(nowMs - RECHECK_AFTER_MS).toISOString();
  const oldest = new Date(nowMs - RECHECK_WINDOW_MS).toISOString();
  return db
    .prepare(
      `SELECT d.id AS decision_id, d.task_id, d.generation, p.option_id, d.body_json, p.picked_at,
              (SELECT a.target FROM approvals a WHERE a.decision_id = d.id AND a.kind = 'merge' AND a.revoked_at IS NULL) AS approval_target
         FROM picks p JOIN decisions d ON d.id = p.decision_id
        WHERE d.source = 'card' AND d.task_id IS NOT NULL AND d.withdrawn_at IS NULL AND p."by" = 'mk'
          AND p.picked_at <= ? AND p.picked_at >= ?
          AND NOT EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = d.id)
        ORDER BY p.picked_at`,
    )
    .all(newest, oldest) as DuePick[];
}

export interface RecheckOptions {
  db: Database.Database;
  ghView: GhView;
  /** Reopens the card as the next generation (Delegation.bringBack through the panel path). */
  bringBack: (taskId: string, decisionId: string, pr: PrRef) => Promise<boolean>;
  /** The card task's current state (TasksClient.taskState). A failed read throws, and the pick is retried next pass. */
  taskState: (taskId: string) => Promise<TaskState>;
  clock?: () => number;
  log?: (msg: string) => void;
}

export class MergeRecheck {
  readonly stats = { checked: 0, resurfaced: 0, errors: 0 };
  /** Decisions already settled this run (merged, closed, skipped or resurfaced), so gh is not asked twice. */
  private readonly done = new Set<string>();
  /**
   * Picks whose PR was still open at a first look this run, with the time of that look (bead mk-qepye). Kept in memory
   * on purpose: after a restart the pick is still found by its age, takes a fresh first look, and is never brought back
   * on a look from before the restart. The cost is one extra poll after a restart past the ten-minute mark.
   */
  private readonly firstOpen = new Map<string, number>();
  private readonly clock: () => number;

  constructor(private readonly o: RecheckOptions) {
    this.clock = o.clock ?? Date.now;
  }

  private settle(decisionId: string): void {
    this.done.add(decisionId);
    this.firstOpen.delete(decisionId);
  }

  async pass(): Promise<void> {
    for (const p of duePicks(this.o.db, this.clock())) {
      if (this.done.has(p.decision_id)) continue;
      const pr = mergePrOf(p.body_json, p.option_id, p.approval_target);
      if (!pr || Number(p.generation) >= MAX_GENERATIONS) {
        this.done.add(p.decision_id);
        continue;
      }
      const now = this.clock();
      const first = this.firstOpen.get(p.decision_id);
      // The second look is due ten minutes after the pick and at least one poll after the first look.
      if (first !== undefined && now < Math.max(Date.parse(p.picked_at) + RECHECK_SECOND_LOOK_MS, first + RECHECK_POLL_MS)) continue;
      try {
        const v = await this.o.ghView(pr.url);
        this.stats.checked++;
        if (v.state === "OPEN") {
          if (first === undefined) {
            // A registrar merge can land just after this look, and GitHub shows nothing in flight: record it, look again later.
            this.firstOpen.set(p.decision_id, now);
            continue;
          }
          const st = await this.o.taskState(p.task_id);
          if (st.state === "deleted" || st.task.status === "canceled") {
            this.settle(p.decision_id); // finished on purpose after the pick; nothing is owed
            continue;
          }
          if (await this.o.bringBack(p.task_id, p.decision_id, pr)) this.stats.resurfaced++;
          this.settle(p.decision_id);
        } else if (v.state === "MERGED" || v.state === "CLOSED") {
          this.settle(p.decision_id); // merged is the goal; closed unmerged is mk's own call
        } else {
          throw new Error(`unknown PR state ${JSON.stringify(String(v.state).slice(0, 40))}`);
        }
      } catch (e) {
        this.stats.errors++; // retried on the next pass while the pick is inside the window
        this.o.log?.(`merge recheck ${pr.url}: ${e instanceof Error ? e.message : String(e)}`);
      }
    }
  }

  async run(signal: AbortSignal, sleep: (ms: number, signal: AbortSignal) => Promise<void>): Promise<void> {
    while (!signal.aborted) {
      try {
        await this.pass();
      } catch (e) {
        this.o.log?.(`merge recheck pass: ${e instanceof Error ? e.message : String(e)}`);
      }
      await sleep(RECHECK_POLL_MS, signal);
    }
  }
}
