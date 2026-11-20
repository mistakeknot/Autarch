# Citadel header preview

Source: bead mk-utw91.3. The permanent merged Queue header puts every other destination under More and uses a Projects chip. There are no alternate header layouts, query parameters or layout switch. It uses one row on desktop and two 44-pixel rows at 390 and 360 pixels. Phones show only the selected panel below the shared header.

The fixture preview bundles actual UI components with invented cards. It never connects to a live plugin. Chat, evidence RPC and the host icon are explicit fixture adapters. Learn marking, Ideas actions, Archive search/reopening and Settings saving use in-page state. The configured host chat attachment is rendered but no live message is sent.

From the repository root, set OUT to an output directory and BB_BUILD to an existing toolchain package that resolves @tailwindcss/node and @tailwindcss/oxide, then run `node docs/plans/screenshots/header-layouts/build.mjs`. No dependency installation is needed. Serve OUT with an isolated loopback server. Set PREVIEW_URL and DEST and run `node docs/plans/screenshots/header-layouts/shoot.mjs`. Chrome defaults to /usr/bin/google-chrome; CHROME can override it.

The browser script captures final-1440x900.png and final-390x844.png, plus a 360x800 check. Captures use those viewports with full-page output. It exercises all More destinations, return to Queue, project filtering/reset, viewport bounds and local actions, without any URL options.

Map was removed because it plotted nothing. Learn retains eight shipped reading cards and persisted Got it/Later marks. Ideas retains project-bound filing and Pursue/Park/Drop actions. Vizier retains the configured host thread chat. Queue, Archive and Settings remain. Answers appear only as buttons with their full instruction effects underneath; the glance block retains the link, optional time and judging criteria. Empty time chips, duplicate opening prose and redundant count headings remain removed.
