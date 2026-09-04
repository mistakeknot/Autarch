# Daily catch-up validation — 2026-09-04

Scope: standalone build, recent changes, question evidence, original-session
handoff. User recognition of the one-minute daily-walk criterion is pending;
the checks below are implementation evidence, not a substitute for that ruling.

## Delivered behavior

- Published dependency revisions replace sibling checkout requirements.
- The opening screen separates recent commits and dated reports from undated
  working-tree changes. Report timestamps cannot advance with later user text.
- Claude and Codex conversation evidence survives a stopped agent. Runtime
  and transcript provider remain separate, including reused terminal seats.
- Questions show full text, choices, supporting context, the original user
  request, later replies, and transcript path/time/offset.
- Explicit structured answers clear requests. Unrelated tool results,
  injected metadata and assistant progress do not. An ordinary later reply
  carries uncertainty and remains accessible through thread evidence.
- Enter first opens evidence, then the selected seat. A separate `s` action
  resumes saved history with its original ID and no new prompt.

## Automated and terminal evidence

The isolated tmux replay builds the actual CLI and drives keys through the
opening screen, Questions, full evidence, explicit resume, project rows and
exact-session handoff. The resume executable is a fixture that records argv;
opening evidence alone must not call it. An answer appended to the transcript
must disappear from Questions after refresh. Narrow terminal width, scrolling,
stable selection, independent refresh, missing git and stale-report regressions
are covered alongside provider-shaped transcript fixtures.

The full race-enabled suite passes with process-wide offline agent stubs.
Ordinary sprint, view and interview tests now isolate installed agents, and
configuration tests isolate the platform's actual config directory. Theme
tests explicitly control color mode. Earlier full runs exposed real-agent
launches and a preferences write in legacy tests; those runs were stopped.
Local preference recovery still awaits the user's choice.

## Live estate read

Before the temporary validation seat was opened: 45 tmux seats, 31 readable
conversation contexts, 30 with project context, and 14 without conversation
IDs. The prior runtime-gated reader recovered no contexts in the earlier
43-seat snapshot. Session counts changed during the work, so these are dated
snapshots, not a fixed denominator.

The new reader found eight questions with no later reply, all saved Claude
history: seven stopped agents and one seat running another provider. The
screen correctly showed zero questions confirmed on a current agent screen.
No private transcript text is included in this record.

The live catch-up displayed 20 projects with recent commits or conversation
activity, source-linked reports, working-tree counts separately, and readable
coverage. The temporary validation seat adds one unnamed seat to that view.

## Limits of this slice

Only named tmux seats with transcript IDs can supply conversation evidence.
Reads are bounded to transcript tails; only the latest question batch is
retained per conversation. Generic prompts do not establish waiting. Text
visible in a pane is evidence of visibility, not proof that execution is
blocked. Reports are agent claims and commits cover local refs, not a release
or deployment verdict. Refresh is explicit (`r`).
