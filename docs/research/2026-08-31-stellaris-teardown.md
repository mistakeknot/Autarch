# Stellaris teardown — the lineage of the gardening loop

```yaml
artifact_type: teardown
evidence_class: secondary (published-analysis)   # desktop game, no live surface;
                                                 # Paradox wiki + patch notes + community threads,
                                                 # dev-diary content via search synthesis only
sources_current_as_of: 2026-08-31                # Stellaris 4.4 "Nomads" stable, 4.5 beta
discover_output: >
  Stellaris (RTwP 4X) is the sole named lineage. Gets right: the composite
  gardening loop — "context switching and deep diving and then watching/waiting
  for intervention points"; "one gardens different planets and systems";
  "closest to what I think I should be doing." Resents (all load-bearing,
  undiscriminated): late-game slog, interrupt fatigue, untrustable autopilot,
  losing the thread.
journey_steps_status: draft — skeleton proposed to mk, not yet corrected
```

## The one structural divergence everything else hangs on

Stellaris time is **game-owned**: the simulation runs only while you watch, at a
pace you set, and never moves while you're away. mk's estate is the opposite —
agents run on wall-clock whether or not the door is open. So the literal
mechanism (pause) does not transfer; what transfers is what pause *does for
attention*: every Stellaris session begins and acts from stillness (the game
loads paused; commands can be issued while paused). For Autarch the equivalent
is not stopping the world — it is stopping *you*: entry begins with a catch-up
reading of what moved, and nothing asks for a decision until orientation is
done. This is the direct answer to "losing the thread."

## Choice → tradeoff pairs, mapped to the journey steps

Journey steps (draft): 1 load the save · 2 survey the empire · 3 choose where
attention lands · 4 dive · 5 delegate and set running · 6 get interrupted well ·
7 tend between interrupts · 8 park the session.

| # | Stellaris choice (shipped mechanism) | Tradeoff | Maps to |
|---|---|---|---|
| A | **Real-time-with-pause**: 5 speeds, game starts paused, commands issue while paused | Total pacing control — but only possible because the world halts; mk's world never halts, so the steal is *orientation-before-obligation*, not pause itself | 1, 4, 8 |
| B | **Per-event-type delivery classes** (4.0 rework): each message type set to Off / Toast / Popup / Popup+Pause, with a settings button *inside the popup* to retune the class the moment it annoys | Granular attention routing and in-context tuning — at the cost of a large config surface (setup itself becomes a chore) | 6 |
| C | **The outliner**: persistent right rail, four tabs, collapsible, gear-customized membership and order, double-click jumps camera | Whole empire always scannable — but at scale the rail itself clutters; Paradox's 4.4 fix was `show_in_outliner`, i.e. **visibility itself became opt-in** | 2, 3 |
| D | **Situations vs events**: long-running things get a progress bar + a re-selectable *approach* (policy lever), living in the situation log, not the popup stream | Long work stops generating interrupt noise and gains a mid-flight steering wheel — at the cost of a second place to look | 6, 7 |
| E | **Delegation by intent**: sector Focus + planet Designation + pre-colonization automation presets + hard scope exclusions (telepath automation removed by hand) | Declares *what outcome* rather than scripting actions — but community verdict is damning: sector automation "absolutely useless," players keep the governor bonus and take the work back. Delegation without trust is theater | 5 |
| F | **Sprawl penalties** (empire size → compounding costs) as the wideness governor | Makes scale cost something — but the community's real late-game complaint is *operational* (firefighting crowds out strategy), which the formula cannot fix; only trustworthy delegation + classed interrupts do | 3, cross-cutting |
| G | **Empire Timeline** (4.0): a running record of playthrough milestones inside the situation log | Cheap narrative re-orientation — underused for active play, but the park/load bridge Autarch lacks entirely | 1, 8 |
| H | **Pacing as a tuned number**: Paradox adjusts "how often the game stops you" as a design metric (message reclassification in 4.0; leader-trait cadence halved) | Interruption budget is a first-class parameter, not an emergent accident | 6 |

## What the resented failures teach (their fixes, in Stellaris's own history)

- **Interrupt fatigue** → answered by B: reclassify, don't mute. 4.0 moved
  anomalies/trader events *down* a class (popup→toast) rather than off. The
  jewel is the in-context retune button — fatigue is fixed at the moment of
  annoyance, not in a settings screen you never visit.
- **Untrustable autopilot** → E is a cautionary tale, not a pattern to copy:
  intent-declaration is the right interface, but Stellaris shipped delegation
  the community routes around. Autarch's version must earn trust or stay
  honest about scope (hand-pruned exclusions are respectable).
- **Late-game slog** → F says ranking math (our ruling 11 is a sprawl formula)
  cannot fix it alone; C's `show_in_outliner` says at 98 projects the door's
  default must become "quiet unless attention-worthy."
- **Losing the thread** → A + G: begin from stillness, with a timeline of what
  moved. Stellaris gets this free by freezing the world; Autarch must build it.

## Community metis worth keeping verbatim

- Automate the low-value single-purpose holdings; hand-manage the capital and
  the special worlds. (Maps to: fund/pin a few projects, delegate the tail.)
- Automation's *destroy* permission can raze a planet to essentials — scope
  delegation permissions narrowly. (Maps directly to agent permission tiers.)
- Speed controls address simulation pacing, not attention routing — players at
  scale slow the game down because interrupts are unclassed, not because time
  moves too fast.

## Open gaps (flagged by the research pass)

- Delivery-class taxonomy (Off/Toast/Popup/Popup+Pause) is reconstructed from
  patch notes + forum fragments, not one wiki table.
- No strong community verdict specifically on the outliner was located.
- Dev-diary philosophy content is search-synthesis only (forum bot-walled).

## Handoff

To `/cujgel:provoke`: pairs A–H are the alternative-bank. The sharpest
provocations available: (1) "the door opens with a catch-up briefing and *no*
list" vs today's ranked rows (A vs C); (2) "agents are situations with
progress bars and approaches, never popups" (D) vs the notification stream
every other agent tool ships; (3) "delegation ships with hand-pruned
exclusions and per-permission scopes, Stellaris-honest about what it can't do"
(E). Journey-step skeleton still awaits mk's correction before derive.
