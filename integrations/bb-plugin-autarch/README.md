# bb-plugin-autarch

A BB plugin that puts Autarch's Home in BB: the asks waiting for mk, a catch-up feed,
the vizier thread, delegation settings, and a small example todo page.

- `server.ts` — the backend: the Home store in `data.db`, RPC methods for the page,
  the `bb home` CLI command (`cli.ts`), settings, and a realtime signal that keeps every
  open page current. It also keeps the example todo list in `bb.storage.kv`.
- `app.tsx` — the frontend: the **Home** page (asks, catch-up, vizier, map, settings)
  and the **Example todos** page, both in the left sidebar.
- `skills/home/SKILL.md` — a skill that tells agents how to file asks and report blocker
  progress with `bb home`. BB imports it into agent threads automatically.
- `PLUGIN_OVERVIEW.md` — the store listing text: a longer version of
  `bb.description` that the plugin detail page shows under it. See
  [Store listing](#store-listing).

Try it: install the plugin, open **Home** in the sidebar, then run
`bb home list` in a terminal. The example todos are managed on their page only.

## UI components

`components/ui/` is vendored source you own (the shadcn model): edit the
files freely — they never update out from under you. Add more from the BB
component registry (the full shadcn set, version-matched to your BB install
via the pinned ref in `components.json`):

```
npx shadcn add @bb/select @bb/table
```

Run `npm install` once before `bb plugin build` — the vendored components'
npm deps bundle into your dist. React, and BB-shimmed packages like the
radix portal primitives and `sonner` (`import { toast } from "sonner"`
reaches BB's own toaster), are provided by the BB app at runtime and never
bundled. Every shimmed package is declared in `devDependencies` at the
host's version so those imports typecheck; keep them there (never in
`dependencies`, which would bundle a second copy), and `bb plugin types`
repins declared packages alongside the SDK; unused packages may be removed. Ship `dist/` (npm tarball or committed for
git installs) so people installing your plugin never need npm.

## Manifest

`package.json` is the plugin manifest. Notable fields:

- `bb.server` — backend entry (required).
- `bb.app` — frontend entry. Delete it, `app.tsx`, `components/`,
  `hooks/`, and `lib/` for a headless plugin.
- `bb.skills` — skill roots; omitted here, so BB reads `skills/`. Each
  directory with a `SKILL.md` is one skill, named after the directory.
- `bb.name` and `bb.description` — required human-facing identity.
- `bb.branding` — required; declare `icon` as a BB icon name or a
  plugin-relative compact SVG, or declare `logo.light` (with optional
  `logo.dark`). Logo assets must be relative `.svg`, `.png`, or
  `.webp` files.
- `engines.bb` — supported bb app version range.
- `engines.bbPluginSdk` — the lowest plugin SDK you need (scaffold:
  `>=0.5.29`). BB reads this as a floor, not a ceiling: a later
  SDK in the same major still loads your plugin.
- `dependencies` — every package your source imports that BB does not provide.
  `bb plugin build` inlines them into `dist/`, and git installs resolve this
  list alone, so a build-required package here rather than in
  `devDependencies` is what keeps your plugin installable. `devDependencies`
  is for types and tooling only (BB shims React, the portal primitives, and
  `@get-bb/plugin-sdk` at runtime — never bundle them).

Run `bb plugin build` before publishing git/npm installs. It writes
`dist/server.js` + `server.meta.json` and `app.js` / `app.css` /
`app.meta.json`. Each `*.meta.json` stamps SDK major/version,
`artifactFormatVersion`, `pluginId`, `pluginVersion`, and
`builtWith` so managed installs can verify the artifacts.

## Store listing

Two texts describe the plugin in the store. `bb.description` in package.json
is the one-sentence hook on every browse card and the lead paragraph on the
detail page; keep it under about 140 characters. `PLUGIN_OVERVIEW.md` is the
same claim at length, shown in an Overview section under that paragraph.
Rewrite the scaffold's copy for your plugin, and update it whenever
`bb.description` changes, so the two never disagree.

The submission to the public BB Community marketplace requires the file. Keep
it under 4000 characters (aim for 700 to 1800) and use headings, paragraphs,
emphasis, code, blockquotes, lists, thematic breaks, and absolute https links
only — raw HTML, images, tables, footnotes, and task lists are rejected. Do
not open with a `#` title or repeat `bb.description` verbatim; the page
shows both directly above.

## Install

From this directory (`bb plugin new` already ran the install; a fresh clone
needs it):

```
npm install
bb plugin install .
```

After editing sources, reload:

```
bb plugin reload autarch
```

Or let `bb plugin dev` rebuild and reload on every save.

## Configure

```
bb plugin config autarch
bb plugin config autarch set showDone false
bb plugin reload autarch
```

## Types & API reference

The plugin API ships as the npm package `@get-bb/plugin-sdk`, pinned to an
exact version in `devDependencies` (`0.5.29` — the SDK of the BB
that scaffolded this plugin). After `npm install`, the full surface is on disk
at:

```
node_modules/@get-bb/plugin-sdk/bundled-types/bb-plugin-sdk.d.ts      # backend
node_modules/@get-bb/plugin-sdk/bundled-types/bb-plugin-sdk-app.d.ts  # frontend
```

Your editor and `tsc` resolve `@get-bb/plugin-sdk` there through ordinary node
resolution — no path mapping. These are readable declarations: open them for an
exact signature.

The SDK surface grows with every BB release, so the pin has to track the BB you
actually run:

```
bb plugin types          # sync this plugin's SDK surface to the running BB
bb plugin types --check  # CI: fail when it does not match
```

Ask BB to write plugins for you: the `bb-plugin-authoring` skill documents
the whole surface with examples.

Confused by the API, or need something the types don't explain? Clone the BB
repo and read the source: <https://github.com/get-bb/bb>.

## Storage and removal

The Home store lives in this plugin's `data.db` (SQLite, WAL), opened through
`bb.storage.database()`. Schema changes are staged and expand-only: an older instance
keeps working on a newer database until `min_reader_version` is raised in a later
release.

`bb plugin remove` leaves `data.db` behind. Reinstalling picks it up again with the
same `store_id`. To reset, stop the plugin and delete the plugin's `data.db` (plus its
`-wal` and `-shm` files); the next start creates a fresh database with a new `store_id`.

The nightly event export writes immutable segments to
`~/.autarch/home-export/<store_id>/events-<first_seq>-<last_seq>.jsonl`, outside the
plugin folder, so it outlives removal. A reset database exports into a new directory.

## Ruling files and the same-user residual

Rulings are written under the project root that `serve` resolved at filing, through a
root-pinned writer (`ruling.ts`): it checks the saved root's `dev` and `ino`, walks
`docs/decisions` refusing symlinks, writes a temp file with `O_NOFOLLOW`, fsyncs and
renames it, then re-checks the root. Node has no `openat`, so on Linux the writer holds
each directory open (`O_DIRECTORY|O_NOFOLLOW`, identity checked with `fstat`) and reaches
children through `/proc/self/fd/<n>/`, which resolves inside that held directory; a path
swapped for a symlink between steps cannot redirect the write. Where `/proc/self/fd` is
absent (macOS), the writer falls back to path-based steps and a same-user process can still
swap a path between them. That is the same-user risk accepted in the plan's `[D21]`; the
writer defends against a different-origin symlink or a replaced root, and on Linux against
a same-user race, but on other platforms not against a hostile process running as mk.
