# Home mobile pass: screenshots (bead mk-sfr1)

Real Chromium renders (Playwright) of the actual `ui/asks.tsx` and `ui/tabs.tsx` over invented fixture data (26 asks, one project, long titles), styled with Tokyo Night tokens and a monospace body font like the host. They are not the live bb app. "Before" is a worktree of main (7a2e031 plus later merges); "after" is this branch. The host's own "Home" header bar is bb chrome and is not part of this plugin.
Rebuild: `git worktree add /tmp/hm-before origin/main && node harness/build.mjs && node harness/shoot.mjs ..` (needs the bb plugin-build toolchain; see build.mjs).

Measured in the browser (390 and 430 wide): no page-level horizontal overflow; tab row is one line and scrolls sideways at 390; smallest tap target 44px after (40px and 20px before); project line count 26 before, 0 after (every ask is from one project).
