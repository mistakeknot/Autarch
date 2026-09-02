# Thread registry probe — mk's session note against tmux and the transcripts

```yaml
artifact_type: probe
evidence_class: primary (mk's Apple Note verbatim, 2026-09-02) + measured (tmux, ~/.claude/projects, ps, same day)
serves: autarch-01 steps 2, 3, 5; autarch-02 step 1; the 01 ledger question on garden-vs-thread rows
probe_scripts: kept in the session's job directory; the method is reproducible from the commands below
```

mk shared the Apple Note they keep of their projects and sessions "along with the terminals and window positions, if that helps design autarch". This document records the note as given and what the machine says about the same threads.

## The note, verbatim

```
—— wezterm
wezterm[finorcs - d256fbdd-98c1-48f2-80fa-f58bfc618394
wezterm[monetization - 2d73ec8c-e1cd-4830-b569-986faf76ce3b

—— iterm
iterm[autarch - 5920c9b1-6a3f-4a7d-8566-e6067aaeaf01
iterm[concordance - fca9cfa0-60ee-460d-ad2b-10688853d70c
iterm[shadewright - 45f5fb3c-9968-4ace-b883-b4202e9dc732
iterm[ushas/bridger - 21434d6f-592b-40c0-9ae3-9663094454af
iterm[jeddnet -  f44fd423-1cc0-4f6d-8bd1-a793d69facf4
iterm[jeddnet@codex - 019f805d-303f-7c43-a79e-7e1893411b25
iterm[estate - 5920c9b1-6a3f-4a7d-8566-e6067aaeaf01
iterm[zahro - f1f7d2ea-6df9-4b6a-8d4c-b998825feeff
iterm[jawnbase - d4ada246-2a56-434f-afe1-e84901791781

[cujgel - 5920c9b1-6a3f-4a7d-8566-e6067aaeaf01
[concordance/interlacer - fca9cfa0-60ee-460d-ad2b-10688853d70c
[agmodb@codex - 019f97b4-65c5-7093-be2f-745372ed5da2

iterm[]rakes-of-the-new-sun - 5c7a44a3-4bff-461e-9ad2-06f0f7c7e18a
iterm[]elf-revel - 82ab9c48-b94d-4d73-be2d-b8af2a69ea52
iterm[]stakeholders - e509df11-511d-4900-b0c4-90426c2a5cbb
iterm[]undersketch - 21404b26-7aa4-44ca-9e7a-9ee553e4ed3f
iterm[]kimilibrary - session_144d176a-592b-459d-a382-409aec933ae2
iterm[]ideagui - f99f1bcc-e641-4501-ab27-4ebca0d6b664

iterm]unc-rancher - 818281f9-9a63-460a-86f3-f2fb124648cb
iterm]jawnomicon - d58d5e63-b647-4d68-82f8-64d787310d15
iterm]shadow-work-ui - 2aee0821-62fc-4917-837f-e16e55dc64f8
iterm]shadow-work-policy - b3de853f-5744-4967-bd05-77ccda16a9b4
iterm]shadowbench - dd7cad33-3fed-4a39-af59-8df5bb5a4d1c
iterm]after-them - 4eb13c63-f7cd-492e-83c4-0b24172bac99
iterm]fluxrig - a8912a85-ef85-4d05-9298-7624464bb33d
iterm]ravenous - f8f5d301-b169-47b0-8a4a-116a5a5b7ec9
iterm]mind-state -  277e9e5c-c622-4e73-96b9-7a60be162cbd
iterm]linsekasten - 74e5950e-c47e-44c8-b811-84e360fdcda0
iterm]grey-area - 456d04f7-549d-453a-bcce-26af85b5e8da

—— rio
rio[jetty/fissionchips - 228255df-83a7-4fa2-8e19-334a7951d0cd
rio[jetty-ar-runbooks - e14147e4-596a-4c62-b626-7c711ee7e142
rio[clavain - aa2bb078-ee16-4c32-9f97-01ef7dbdec61
rio[taxes - 5a729c1d-03bf-43e1-8d4b-44ae1e7187f8

rio]auraken-inkling - 3b67def9-c705-40c0-a758-0fb311d785ee
rio]solwend - 21cc6bd2-139f-4127-9c2d-7ea821858209
rio]jawnfund - d0d8e10f-f094-4634-94ee-9b4ce24360f8
rio]garden-salon

[tldrs
[remontoire
[jeddbench
[canongraph
[agmodb
[oodacademy

]interstate
]ifs
]jawnfit
]jawnscope
]phosphene
]gsvdotcom
]athenverse
```

Shape of the note: 52 topic lines. 38 carry a session id (35 distinct: 33 Claude Code, 2 Codex, 1 of another runtime for kimilibrary). 14 carry no id. Three terminal emulators. Three position marks: `[` on 24 lines, `]` on 22, `[]` on 6. The meaning of the marks is mk's to state; the working reading is left half, right half, and full width. One session id appears under three topics (autarch, estate, cujgel) and another under two (concordance, concordance/interlacer): topics map many-to-one onto threads.

Topics are not repos. `estate`, `monetization`, `stakeholders`, `taxes`, `mind-state`, `cujgel` name themes, non-code gardens, or tools. This is the estate ontology's lattice showing up in mk's own registry, and the vision capture's open question about non-code gardens made concrete.

## What tmux says

`tmux list-panes -a -F '#{pane_current_command}|#{pane_pid}|#{session_name}'`, 2026-09-02 16:11 local.

39 tmux sessions, one window each, every pane's working directory `~/projects`. 34 of them are named in the note's exact line format, terminal mark and resume id included. The note is a transcription of the tmux session list, kept by hand in a second place.

The two copies have drifted:

| topic | note | tmux |
|---|---|---|
| ushas/bridger | id 21434d6f (7.3 MB transcript) | id ef9ad21a (45 MB transcript, the live one) |
| rakes-of-the-new-sun | that name | `rakes-of-the-new-book`, same id |
| grey-area | `iterm]grey-area - 456d04f7…` | bare `grey-area` |
| shadewright | id 45f5fb3c, transcript touched today | no tmux session |
| garden-salon, 13 no-id topics | listed with a position | no tmux session |
| ryan, spellswords@hermes, 28, 30, kimifork, mobile | not in the note | tmux sessions |

What the panes run:

| pane command | count | meaning |
|---|---|---|
| a Claude Code version binary (`2.1.221` … `2.1.258`, 13 distinct versions) | 32 | a running Claude Code thread |
| `codex` | 1 | jeddnet@codex |
| `kimi` | 1 | kimilibrary |
| `node` | 1 | kimifork |
| `python3.11` | 1 | spellswords@hermes |
| `zsh` | 3 | idle shells: 28, 30, mobile |

The threads are running processes with a named seat, not parked transcripts. Because Claude Code's process is named by its version and auto-update is not pinned on this machine, the version is a free proxy for how long a thread has stood: `2.1.221` on fluxrig and jetty/fissionchips, `2.1.233` on taxes, `2.1.258` on the 17 most recently started. A plain `ps | grep claude` finds none of them.

## What the transcripts say

Every one of the 33 Claude Code ids resolves to a transcript. 31 sit under the estate root directory (`~/.claude/projects/-Users-sma-projects`), 1 under Sylveste (grey-area), 1 under the jetty/fissionchips worktree. The per-garden directory attribution the briefing shipped with (plan WI-1, `IndexSessions`) therefore sees 2 of 33 threads.

Every transcript's mtime read as today, but the tails are bookkeeping entries with no timestamp (`bridge-session`, `mode`, `permission-mode`, `last-prompt`, `atis-latch`), not conversation. The last real turn tells a different story:

| thread | last conversational turn | pane |
|---|---|---|
| taxes | 2026-08-14 (an informational entry 08-20) | 2.1.233, running |
| fluxrig | 2026-08-25 | 2.1.221, running |
| jawnbase | 2026-08-26 | 2.1.232, running |
| jetty/fissionchips | 2026-08-27 | 2.1.221, running |
| the other 28 | 2026-09-01 or 09-02 | running |
| both Codex threads | 2026-08-19 | one running |

mtime is not a liveness signal for Claude Code transcripts. The last `user` or `assistant` entry's timestamp is. The briefing's sessions column counts files by mtime and will overstate.

Attribution from transcript content works where the directory does not. Counting mentions of `/Users/sma/projects/<garden>` across each transcript (1,010 MB scanned in about a minute) gives a dominant garden that matches mk's topic label for 31 of the 32 threads that mention any path:

| thread | gardens mentioned | top gardens |
|---|---|---|
| zahro | 9 | zahro 13698 |
| jawnomicon | 41 | jawnomicon 58091, gsvdotcom 4555, intervox 1980 |
| monetization | 15 | solwend 1041, jawncite 221, palettice 71 |
| after-them | 111 | After-Them 21854, jawnsmith 3440, uncrancher 2604 |
| clavain | 100 | tool-time 1691, Clavain 1641, dotfiles 1303 |
| linsekasten | 14 | interlens 100, interflux 66 |
| mind-state | 41 | palettice 2494, Mind-State 536 |
| taxes | 0 | none: a non-code garden, invisible to every substrate probed |

The wide threads (after-them 111, clavain 100, jawnbase 57, auraken-inkling 50) are the many-garden standing threads the session archaeology described, now countable.

## Findings

1. The registry Autarch is meant to be already exists as data: tmux session names carry terminal, window position, topic and resume id. mk keeps a second copy by hand and the copies have drifted in at least three places.
2. Liveness has three observable states today without any pane scraping: a pane running an agent binary (with its version as age), a pane at an idle shell, and a topic with no seat. The waiting-on-a-human state is the one that still needs the pane's content.
3. Garden attribution for a thread launched at the estate root comes from the paths in its transcript, not from the transcript's directory or the pane's cwd. Both substrates the briefing plan probed were the wrong field.
4. Transcript mtime lies; the last conversational turn does not. This is a defect in the shipped briefing's sessions count.
5. Non-code gardens (taxes) and themes (monetization, estate, stakeholders) are rows in mk's registry and have no path evidence at all. The card and the row model need to admit them.
6. Window position is a first-class axis for mk. Entry by `switch-client` moves a thread into whichever window Autarch runs in, which cuts across the layout mk remembers.

## Questions for mk (not ruled here)

- Is the walk's row the garden, the thread, or the topic as the note has it, with gardens attributed underneath?
- What do `[`, `]` and `[]` mean, and are the three emulators a deliberate axis or history?
- Should entry focus the thread's existing window rather than switching the client Autarch runs in?
- Which of the 14 no-id topics are dormant gardens holding a seat, and which are stale lines?

## Method

- Note parsed with `^(wezterm|iterm|rio)?(\[\]|\[|\])\s*(.+?)(?:\s+-\s+(\S+))?$`.
- Transcript lookup: `~/.claude/projects/*/<id>.jsonl`; Codex: `~/.codex/sessions/**/rollout-*<id>.jsonl`.
- Attribution: regex over `/Users/sma/projects/([^/]+)(/[^/]+)?(/[^/]+)?`, Sylveste expanded one level, counted per line.
- Liveness: `tmux list-panes -a -F '#{pane_current_command}|#{pane_pid}|#{session_name}'`; last real turn from the trailing 200 KB of each transcript.
