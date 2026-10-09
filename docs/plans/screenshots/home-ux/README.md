# Home UX pass: screenshots (bead mk-yjp7)

Real Chromium renders (Playwright) of the actual plugin components (`ui/waiting.tsx`, `ui/notices.tsx`, `ui/catchup.tsx`, `ui/asks.tsx`, `ui/tabs.tsx`) over invented fixture data (`harness/fixtures.ts`: one free-text card with a dry run and a final command, a vizier-adoption notice, three catch-up rows). They are not the live bb app. "Before" is a worktree of a6621fc; "after" is this branch.
Rebuild: `git worktree add /tmp/hu-before a6621fc && BB_BUILD=<plugin-build dir> node harness/build.mjs && node harness/shoot.mjs ..` (full-page shots at 390x844 and 430x932).

Measured in the browser: no page-level horizontal overflow at either size; smallest button height 44px after (16px before: the thread link); the final command wraps so no flag is hidden off-screen.
