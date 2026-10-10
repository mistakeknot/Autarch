// "At a glance" for play and walk cards (bead mk-a4o0.19). A card that asks mk to go and look at something (play a
// build, walk a route, check a page) buried its link, its time and the things to judge in one long paragraph, so
// the phone showed a wall of text before any of it. This reads those out of the card's own words and puts them in
// one short block at the top: what it is, the link, how long, what is being judged and what each answer does.
// Display only: nothing is sent, the full text stays on the card below, and where the card does not say a thing
// (a time, a list) the block says so instead of guessing. Filers who want exact fields need the v2 contract
// extended in both parsers; that is not done here.
import type { OwedAsk } from "./asks.js";
import { buttonClass } from "./buttons.js";
import { splitCommands } from "./commandtext.js";

export interface GlanceLink {
  kind: "url" | "path";
  href: string;
  text: string;
}
export interface Glance {
  /** The card's first sentence: what it is and why it is here. */
  lead: string;
  link: GlanceLink | null;
  /** "About 2 min", from the title or the text; null when the card states no time. */
  time: string | null;
  /** What mk is asked to judge: the card's numbered questions, or the sentence that says what nobody has checked yet. */
  judging: string[];
}

/** A title that asks mk to go and do something: the verb opens the title or a clause of it ("Prototype G1: play the slice"). A link alone does not make a card a check card. */
const CHECK = /(?:^|[:—–?]\s*)(?:please\s+)?(?:play|playtest|walk|walkthrough|check|try)\b/i;
/** A check word elsewhere in the title counts only with a stated time ("Street check: three questions (2 min)"). */
const CHECK_WORD = /\b(?:play|playtest|walk|walkthrough|check|try)\b/i;
/** A check word in the title is not enough ("Which CI check should block merging?"): the card must also ask for a time or a go-and-do. */
const ACTION = /\b(open|play|walk|try|visit|look at|go through|click|run it)\b/i;
const TIME = /\b(\d{1,3})(?:\s*(?:-|–|to)\s*(\d{1,3}))?\s*(minutes?|mins?|hours?|hrs?)\b/gi;
/** What may stand right before a duration for it to be how long the check takes, and what may follow it for it to be a moment instead. */
const DURATION_CUE = /(?:\(|~|\btakes?)\s*(?:about\s+|around\s+|roughly\s+)?$/i;
const MOMENT_AFTER = /^\)?\s+(?:ago|after|before|from now|later)\b/i;
/** A second number and unit right after ("1 hour 30 minutes"): not a duration this block can show whole. */
const COMPOUND_AFTER = /^\s*(?:and\s+|,\s*)?\d{1,3}\s*(?:minutes?|mins?|hours?|hrs?)\b/i;
const URL_RE = /https?:\/\/[^\s<>"']+/;
const PATH_RE = /(?<![\w:/.~-])\/(?:[\w.@+-]+\/)+[\w.@+-]*\.[A-Za-z0-9]{1,8}\b/;
const MARK = /(?:^|[\s(;:])\(?(\d{1,2})\)\s*/g;
const MAX_ITEMS = 6;
const clip = (s: string, n: number) => (s.length > n ? `${s.slice(0, n - 1).trimEnd()}…` : s);

/** The one duration the text states for the check, or null. A second, different duration (or one the text does not call the check's own) makes the card ambiguous, so it says nothing. */
function timeOf(t: string): string | null {
  const found = new Set<string>();
  for (const m of t.matchAll(TIME)) {
    const before = t.slice(Math.max(0, m.index! - 24), m.index!);
    const after = t.slice(m.index! + m[0].length);
    if (MOMENT_AFTER.test(after)) continue;
    if (!DURATION_CUE.test(before) || COMPOUND_AFTER.test(after)) return null;
    const unit = m[3]!.toLowerCase().startsWith("h") ? "hr" : "min";
    const span = m[2] ? `${m[1]}–${m[2]}` : m[1]!;
    found.add(`${/(?:about|around|roughly|~)\s*(?:about\s+|around\s+)?$/i.test(before) ? "About " : ""}${span} ${unit}`);
  }
  return found.size === 1 ? [...found][0]! : null;
}

/** Drops prose punctuation after a link, and a closing bracket only while it has no opener in the link. */
function trimUrl(raw: string): string {
  let u = raw;
  for (;;) {
    const last = u.at(-1);
    if (last !== undefined && ".,;:!?".includes(last)) u = u.slice(0, -1);
    else if (last === ")" && (u.match(/\(/g)?.length ?? 0) < (u.match(/\)/g)?.length ?? 0)) u = u.slice(0, -1);
    else if (last === "]" && (u.match(/\[/g)?.length ?? 0) < (u.match(/\]/g)?.length ?? 0)) u = u.slice(0, -1);
    else return u;
  }
}

function linkOf(prose: string): GlanceLink | null {
  const u = URL_RE.exec(prose);
  if (u) {
    const href = trimUrl(u[0]);
    return { kind: "url", href, text: href.replace(/^https?:\/\//, "").replace(/\/$/, "") };
  }
  const p = PATH_RE.exec(prose);
  return p ? { kind: "path", href: p[0], text: p[0] } : null;
}

const firstSentence = (s: string): string => /^(.*?[.!?])(?=\s|$)/s.exec(s)?.[1] ?? s;

/** "1) a; 2) b" or "(1) a; (2) b": the markers must count up from 1, so a version or a PR number is never a list. */
function numbered(prose: string): string[] {
  const marks: { at: number; end: number }[] = [];
  for (const m of prose.matchAll(MARK)) {
    const n = Number(m[1]);
    if (n === marks.length + 1) marks.push({ at: m.index!, end: m.index! + m[0].length });
    else return []; // the first marker is not 1, or a skip, repeat or swap: not a list we can read
  }
  if (marks.length < 2) return [];
  return marks.slice(0, MAX_ITEMS).map((m, i) => {
    const next = marks[i + 1];
    const raw = prose.slice(m.end, next ? next.at : undefined).trim();
    const sentence = firstSentence(raw).replace(/[;,.]+$/, "").replace(/\s+(and|or)$/i, "").trim();
    return clip(sentence, 200);
  }).filter((s) => s !== "");
}

/** Reads a check card's context out of its words. Null for any card that is not a check card, so every other card is unchanged. */
export function glance(ask: Pick<OwedAsk, "subject" | "ask">): Glance | null {
  const subject = ask.subject ?? "";
  const timeInTitle = timeOf(subject);
  if (!CHECK.test(subject) && !(timeInTitle !== null && CHECK_WORD.test(subject))) return null;
  // Commands are the card's own copyable blocks; a path inside one is not the thing to open.
  const prose = splitCommands(ask.ask.question).flatMap((p) => (p.type === "prose" ? [p.text] : [])).join("\n");
  const flat = prose.replace(/\s+/g, " ").trim();
  const link = linkOf(prose);
  let judging = numbered(flat);
  if (judging.length === 0) {
    const w = /\bwhether\b[^.!?]*/.exec(flat);
    if (w) judging = [clip(w[0].trim(), 200)];
  }
  // The title and the text must agree: two different durations mean the card does not say.
  const timeInText = timeOf(flat);
  const time = timeInTitle !== null && timeInText !== null && timeInTitle !== timeInText ? null : (timeInTitle ?? timeInText);
  if (time === null && !ACTION.test(flat)) return null;
  if (link === null && time === null && judging.length === 0) return null;
  return { lead: clip(firstSentence(flat), 220), link, time, judging };
}

export interface GlanceAnswer {
  id: string;
  label: string;
  recommended: boolean;
  /** What picking it does; null when it is the plain "records your pick" line, which is said once for the whole list. */
  effect: string | null;
}

const SMALL = "m-0 mt-2 text-xs font-semibold uppercase text-muted-foreground";

/** The block itself: link and time on the first line, then the lead, what is judged and what each answer does. */
export function GlanceBlock({ g, answers, recordsOnly }: { g: Glance; answers: GlanceAnswer[]; recordsOnly: boolean }) {
  return (
    <section className="mt-3 min-w-0 rounded-lg border border-border bg-muted/40 p-3" aria-label="At a glance" data-glance>
      <div className="flex flex-wrap items-center gap-2">
        {g.link?.kind === "url" ? (
          <a href={g.link.href} target="_blank" rel="noopener noreferrer" className={`${buttonClass("recommended")} min-w-0 max-w-full no-underline sm:!min-h-11 [overflow-wrap:anywhere]`} data-glance-link>
            {`Open ${g.link.text} ↗`}
          </a>
        ) : null}
        {g.link?.kind === "path" ? (
          <span className="min-w-0 text-xs text-muted-foreground">
            {"File: "}
            <code className="select-all rounded bg-muted px-1.5 py-1 font-mono text-xs [overflow-wrap:anywhere]" data-glance-path>{g.link.text}</code>
          </span>
        ) : null}
        <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground" data-glance-time>{g.time ?? "Time not stated"}</span>
      </div>
      <p className="m-0 mt-2 text-sm [overflow-wrap:anywhere]" data-glance-lead>{g.lead}</p>
      {g.judging.length > 0 ? (
        <>
          <p className={SMALL}>{g.judging.length > 1 ? "You are judging" : "You are checking"}</p>
          <ol className="m-0 mt-1 list-decimal space-y-1 pl-5 text-sm" data-glance-judging>
            {g.judging.map((j, i) => <li key={i} className="[overflow-wrap:anywhere]" data-glance-item>{j}</li>)}
          </ol>
        </>
      ) : null}
      {answers.length > 0 ? (
        <>
          <p className={SMALL}>Your answers</p>
          <ul className="m-0 mt-1 list-none space-y-1 p-0 text-sm" data-glance-answers>
            {answers.map((a) => (
              <li key={a.id} className="[overflow-wrap:anywhere]" data-glance-answer={a.id}>
                <span className={a.recommended ? "font-medium" : ""}>{a.label}</span>
                {a.recommended ? <span className="ml-1 text-xs text-primary">(recommended)</span> : null}
                {a.effect ? <span className="block text-xs text-muted-foreground">{a.effect}</span> : null}
              </li>
            ))}
          </ul>
          {recordsOnly ? <p className="m-0 mt-1 text-xs text-muted-foreground" data-glance-records>Each answer records your pick only; nothing is sent.</p> : null}
        </>
      ) : null}
    </section>
  );
}
