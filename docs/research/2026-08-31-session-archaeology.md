# Session archaeology — how mk actually works (August 2026)

```yaml
artifact_type: observation
evidence_class: primary (behavioral — ~/.claude/projects transcript corpus, this machine)
window: 2026-08-01 .. 2026-08-31 (retention truncates earlier data; June/July partial)
method: >
  Per-session first/last timestamps + real user-turn counts (type:user, non-sidechain,
  non-tool-result) across 1,287 August transcripts; probe dirs (CodexBar ClaudeProbe,
  /private) excluded; 30 background-job session IDs tagged; active time = span minus
  >3h dormancy gaps. Scripts in the session job dir (session_archaeology*.py,
  dive_roster.py, thread_cadence.py).
requested_by: mk — "hmm check my sessions to determine how I actually work"
```

## The topline split

| Population | August count | What it is |
|---|---|---|
| Total sessions | 1,287 | everything ≥2KB in the transcript store |
| One-shots (≤1 user turn) | 1,193 (~40/day) | the fleet: `claude -p`, hooks, delegates, cron |
| Touches (2–5 turns) | 69 | 2-minute pokes, mostly repo-local (dotfiles 16, uncrancher 10) |
| Working (6–20 turns) | 9 | median 111 active minutes |
| **Dives (>20 turns)** | **16** | **median 58 turns, ~10 active hours — see below** |

Plus ~44 codex sessions/day (2,659 over 60 days) — delegated executors, separate
corpus, not thread-analyzed here.

## Finding 1 — the unit of work is the standing thread, not the session

The 16 "dives" are not sittings. They are **standing conversation threads, alive
for 2–4 weeks each**, returned to again and again: spans of 607h, 565h, 564h,
554h wall-clock; 634, 303, 252 turns. Only **one of sixteen** (clife, 3.3h, one
day) matches the classic "enter a project, dive, leave" shape.

Each thread is **thematic and estate-ranging, not garden-scoped**: single
threads show cwds across Clavain + discourse-vivify + infinite-fun-space +
shadow-work; another across After-Them + Sylveste + bridger + elf-revel +
jawnomicon + kublai + mythref. Fifteen of sixteen live in the `~/projects`
transcript root — mk launches from the estate and reaches into gardens.

## Finding 2 — the threads have pace-layer structure (the cadence spine, confirmed, relocated)

Revisit cadence per thread (mk-touched days, median gap between them):

- **Daily companions** (gap 1d): four threads touched 20, 20, 16, 12 distinct
  days in the month — the fast layer.
- **Every-2–3-days**: five threads (gaps 2–3d) — the mid layer.
- **Weekly-ish**: one thread at 5d gaps across 24 days — the slow layer.
- **Burst threads**: short-lived 1–4 day runs that end (a real "dive and done").

The tending-cadence spine the CUJs assert is **empirically real — but the
tended object is the thread, not the garden.** Gardens are where threads reach;
threads are what mk returns to.

## Finding 3 — threads never park

No landing-the-plane behavior is observable: the long threads idle under
compaction/auto-resume and get picked back up days later. mk keeps threads
alive *because the thread's accumulated context is the orientation* — a living
thread is its own seance. Parking-as-ritual (autarch-02 step 6) is therefore a
**designed correction to a real failure mode** (threads die by compaction and
lose canon), not a description of current habit — its evidence class must say
so.

## Finding 4 — presence rhythm and the fleet's hum

- Active **24 of 31 days**; weekdays slightly heavier (Mon 22, Thu 19 real
  sessions), weekends lighter but never zero.
- Real-session starts: morning ramp 7–10am, a midday block ~1–2pm, and the
  heavy band **7pm–1am** (dive starts cluster at 7–8am, 4–5pm, and 8pm).
- The fleet mirrors mk's hours (one-shot peaks 1pm, 7–8pm) plus a midnight–2am
  autonomous tail; near-silence 3–6am.
- Median 3 real sessions across 2 projects per active day; max 6 projects/day.
- Concurrency: median 3, p90 6 overlapping sessions — the "agents run while
  you're away" premise is visible in the data.

## Finding 5 — the dormant tail is real (walk needs quiet-default)

Since June: ~23 project roots on a fast visit cadence (≤2d median gap), ~5 mid
(3–7d), ~37 touched fewer than 3 days, and ~50 of 112 transcript roots not
touched at all. In any month, **roughly two-thirds of the estate is dormant** —
the door's quiet-unless-attention-worthy default is not an aesthetic, it is the
only honest rendering.

## Blind spots (named, not waved at)

- **zklw**: the canonical fleet runs there; none of it is in this corpus.
- **codex**: counted, not thread-analyzed; "codex-first" repos under-observed.
- **Non-agent work**: Zed, Ghostty, browsing, and the door TUI itself leave no
  transcripts here.
- **Retention**: ~60-day horizon; seasonal (monthly+) rhythms are not
  measurable yet. The claim "no slow-cadence gardens" cannot be made from a
  one-month window.

## Consequences for the CUJs

| CUJ claim | Verdict from the data |
|---|---|
| Tending-cadence spine (walk/dive/reshape/canon) | **Confirmed** — cadence classes are measurably real |
| The dive enters *a garden* | **Corrected** — 15/16 threads are estate-scoped and thematic; the garden is reached from the thread |
| Parking is the session's close | **Contradicted as habit, upheld as need** — threads never land today; that is exactly the losing-the-thread failure |
| Quiet-default at estate scale | **Confirmed** — two-thirds of the estate dormant in any month |
| Walk = scan gardens | **Open** — the objects mk actually triages daily are standing threads + fleet jobs; whether the door's primary row is the garden or the thread is now a ruling for mk |

The open axis question is logged in autarch-01/02's ambiguity ledgers (v1.1)
rather than silently re-derived; the specs' step text still reads garden-first
pending mk's ruling.

## Correction (mk, same day) — the threads are scaffolding, not preference

> "well, the reason i work in sessions/threads is because we haven't built
> autarch/garden salon" — mk, 2026-08-31

This re-frames the headline finding. Adopted behavior outranks stated
preference only when the behavior was chosen among alternatives; a workaround
is evidence of the *need*, not the *design*. The standing thread is the only
container that exists today for cross-garden, cadence-respecting,
context-accumulating work — so mk lives in it. Read as scaffolding, each
thread property names the missing organ it compensates for:

| Observed thread property | What it compensates for | Where the organ lives |
|---|---|---|
| Estate-ranging (15/16 span many gardens) | No cross-garden discourse — mk is the only bus carrying context between gardens | autarch-04 (guests talk to each other) |
| Never parks; living thread = its own seance | No durable estate memory or cheap structured parking — thread context is ersatz canon | autarch-02 step 6 (landing the plane), autarch-04 (canon capture) |
| Daily companions re-entered 20/26 days | No walk, no catch-up briefing — re-entering the thread IS the orientation | autarch-01 (briefing, waiting-on-me) |

**What survives the correction** (tool-independent facts): the pace-layer
cadence classes (attention rhythms, not tool artifacts), the dormant
two-thirds of the estate, the presence rhythm, the fleet's hum.

**What inverts**: the garden-vs-thread axis question. The thread is not a
candidate first-class object to enshrine — it is the thing Autarch/garden
salon exist to dissolve. The live question becomes a *decomposition order*:
which compensated function does the estate absorb first? Usage frequency
ranks them — orientation is exercised daily (4 companion threads), the bus
function constantly (15/16 threads), parking never happens at all (16/16).
Transitional honesty: threads exist today and the HUD transcribes reality,
so rendering live threads during the transition may still be wanted — as
scaffolding on the way down, not as the data model.
