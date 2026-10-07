# Home "Your move" Phase 1, chunk B: screenshots

Real Chromium renders (Playwright, chromium-1208) of the actual plugin components, bundled with esbuild and styled with Tailwind v4
using Tokyo Night theme tokens. They are NOT the live bb app: the repo has no UI harness that renders inside bb, so `harness/` mounts
`ui/yourmove.tsx`, `ui/asks.tsx` and `ui/blocks.tsx` over invented fixture data (`harness/fixtures.ts`), with RPC handlers stubbed.
"Before" is the same harness pointed at a worktree of 687ca9d; "after" is this branch.
Rebuild: `node harness/build.mjs && node harness/shoot.mjs .` (needs /tmp/ym-before checked out at 687ca9d and the bb plugin-build toolchain; see build.mjs).

| File | What it shows |
|---|---|
| before-overview-1100.png | Asks only (Decide queue, Stalled row): underlined-link options, no Your move, no Other box, no Dismiss. |
| after-overview-1100.png | Your move above Asks: script card with four copy blocks in run order (sha256sum with expected sha, --check, real run, recovery), PR/context cards, Reported done not verified (waiting, failed, succeeded, no report), Later, collapsed Closed list, bordered option buttons with a recommended mark and a red irreversible one, Other box on every card, Dismiss on the Stalled row. |
| after-overview-390.png | Same at phone width: no page-level horizontal overflow; long commands scroll inside their box. |
| before-unbound-1100.png / after-unbound-1100.png | Blocks panel: before shows pick links on an unbound card; after moves it to "Unbound projects" with an explanation, no pick buttons, Answer with this disabled. |
| after-copied-state.png | The Copy button in its "Copied" state. |
| after-button-focus-and-hover.png | Keyboard focus ring on one button and hover on another. |
| after-other-box-filled.png | Other box with text, counter, both buttons enabled. |
| browser-checks.json | Real-browser results: clipboard API text equals the block text (no newline, backtick or trailing space); fallback path (navigator.clipboard removed) copies the same string; triple-click selects exactly the command; command boxes are overflow-x:auto / white-space:pre. |
