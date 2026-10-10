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

/** A title that asks mk to go and do something. A link alone does not make a card a check card. */
const CHECK = /\b(play|playtest|walk|walkthrough|check|try)\b/i;
const TIME = /(?:(about|around|roughly|~)\s*)?\b(\d{1,3})(?:\s*(?:-|–|to)\s*(\d{1,3}))?\s*(minutes?|mins?|hours?|hrs?)\b/i;
const URL_RE = /https?:\/\/[^\s<>"')\]]+/;
const PATH_RE = /(?<![\w:/.~-])\/(?:[\w.@+-]+\/)+[\w.@+-]*\.[A-Za-z0-9]{1,8}\b/;
const MARK = /(?:^|[\s(;:])\(?(\d{1,2})\)\s*/g;
const MAX_ITEMS = 6;
const clip = (s: string, n: number) => (s.length > n ? `${s.slice(0, n - 1).trimEnd()}…` : s);

function timeOf(...texts: string[]): string | null {
  for (const t of texts) {
    const m = TIME.exec(t);
    if (!m) continue;
    const unit = m[4]!.toLowerCase().startsWith("h") ? "hr" : "min";
    const span = m[3] ? `${m[2]}–${m[3]}` : m[2]!;
    return `${m[1] ? "About " : ""}${span} ${unit}`;
  }
  return null;
}

function linkOf(prose: string): GlanceLink | null {
  const u = URL_RE.exec(prose);
  if (u) {
    const href = u[0].replace(/[.,;:!?]+$/, "");
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
    else if (marks.length > 0) return []; // a skip, repeat or swap: not a list we can read
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
  if (!CHECK.test(subject)) return null;
  // Commands are the card's own copyable blocks; a path inside one is not the thing to open.
  const prose = splitCommands(ask.ask.question).flatMap((p) => (p.type === "prose" ? [p.text] : [])).join("\n");
  const flat = prose.replace(/\s+/g, " ").trim();
  const link = linkOf(prose);
  let judging = numbered(flat);
  if (judging.length === 0) {
    const w = /\bwhether\b[^.!?]*/.exec(flat);
    if (w) judging = [clip(w[0].trim(), 200)];
  }
  const time = timeInTitle ?? timeOf(flat);
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
