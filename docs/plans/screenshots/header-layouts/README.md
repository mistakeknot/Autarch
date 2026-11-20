# Citadel header options

Source: bead mk-utw91.3. The fixture preview bundles the actual UI components and existing fake cards. It never connects to a live plugin. The host chat and evidence RPC are explicit fixture adapters; Learn marking and Ideas filing/actions use in-page state.

From the repository root, set OUT to an output directory and BB_BUILD to an existing toolchain package that can resolve @tailwindcss/node and @tailwindcss/oxide, then run `node docs/plans/screenshots/header-layouts/build.mjs`. No dependency installation is needed. Serve OUT with an isolated loopback server. Set PREVIEW_URL and DEST and run `node docs/plans/screenshots/header-layouts/shoot.mjs` for desktop and phone screenshots and interaction checks. Chrome defaults to /usr/bin/google-chrome; CHROME can override it.

A uses compact outlined controls (`?headerLayout=compact`). C uses wrapping underlined tabs (`?headerLayout=wrap`). Both keep the filter alongside the controls, remove the duplicate summary, and use the same queue counts and content. The visible preview buttons switch the URL. Explicit layout queries also select the responsive queue on phones in the plugin; the ordinary phone inbox remains available without the parameter. The default header uses the wrapped baseline.

Map was removed: its original render only changed a lens label and plotted nothing. Learn retains its shipped reading deck and persisted Got it/Later marks. Ideas retains project-bound idea filing and Pursue/Park/Drop actions. Vizier retains the configured host thread chat. Queue, Archive and Settings remain.

Independent fixes remove empty time chips and the duplicate glance lead (the opening text remains in the full body), wrap top-level navigation, remove duplicate top-level badges and the summary line, and omit the redundant Decide heading inside the selected Queue section.
