---
artifact_type: research
bead: mk-schu
---

# Vizier window spike: Open Questions 1 and 2

Bead mk-schu. Checked 2026-09-29 against the installed bb build
(0.44.0+aleph.0.5.0, plugin-authoring reference) and the local bb source at
`~/bb-picker-switch` (checkout dated 2026-09-22, so the desktop findings are
from source that may lag the installed build).

## OQ1: can a plugin panel show a thread's chat? Yes.

`ThreadChat` is a host component in the plugin SDK. It is documented as bb's
complete chat surface for an existing thread, rendered wherever plugin React
runs: nav panels, thread-panel tabs, homepage and settings sections. Props:
`threadId`, `variant` (`full`, `compact`, `timeline`), `layout`,
`focusRequest`, `permissionPolicy`, `leadingContent`, `messageActions`.
The host owns loading, streaming, drafts, send/queue/steer/stop,
attachments, pending interactions and read tracking. The docs say not to
proxy thread data or rebuild the composer.

So approach A stands: the Home panel can embed the vizier's thread with its
composer. No Aleph core change and no `bb thread tell` fallback are needed.
Limits: there is no scroll-to-message, and `permissionPolicy: "inherit"`
keeps sends at the thread's own permission mode.

Not yet tested: an actual render in the installed build. This is a
documentation and source finding. A one-panel smoke test in the Home plugin
would confirm it.

## OQ2: can the overlay be a frameless always-on-top window? Not from a plugin.

- Aleph desktop is Electron, and it already opens several `BrowserWindow`s
  (main, log viewer, dialog, browser-view popups).
- Frameless is already supported on Linux (`desktop-window-factory.ts`,
  `frame: false`).
- The source has no `globalShortcut` and no `alwaysOnTop`, so a summon
  hotkey and an on-top window do not exist today.
- The plugin API reference has no hotkey or window-opening surface.
  Plugins run in the renderer and server, and cannot reach Electron's main
  process.

So the overlay needs an Aleph core change, in the desktop main process:
register a global shortcut, and open a frameless always-on-top window that
loads a plugin panel route. That is small, but it is core, and it belongs to
the Aleph coordinator (thr_39wwcmwi84). The web build cannot have a global
hotkey; it is limited to an in-page overlay.

Consequence for the order in decision 5: the Home vizier column (A) has no
core dependency. The overlay is separate and gated on that core change, so
it should not block the plan revision.

## What was not checked

- The installed build's desktop binary, which is not on this host.
- Whether the Aleph coordinator already has a window or hotkey change in
  flight. Ask before proposing one.

## Added 2026-09-29: approvals that authorize one exact action

The vizier asked (mk: "this should be a core functionality in Aleph") that a
pick can be an authenticated approval for one action: kind (merge, deploy,
release), target (repo#PR, site, unit) and pinned identity (head SHA, image
id), single-use or expiring, with an audit trail and a local check API for
hooks and the classifier. To be folded into plan revision 4.

Position for the plan review (mk decides): the record and its check API
belong in **Aleph core**, and the Home plugin is only the surface that files
picks.

- Whoever enforces (hooks, the classifier) cannot depend on a plugin. Plugin
  code runs in the server process but can be removed, and `bb plugin remove`
  leaves `data.db` behind (decision 24), so a plugin-owned table is not a
  trust root.
- "Per account, not per device" needs the account identity, which core owns.
- Single use needs an atomic consume. Home's database can do that (the pick
  is one transaction), but only for callers that go through the plugin.
- Delegated rulings (`by: vizier`) never mint an approval; only mk's own
  authenticated pick does. The plan should state that as an invariant.

Interim if core is not ready: keep the approval schema in Home's database
with a narrow local API, mark it advisory, and leave the SylvesteOps text
hook as the enforcing check until core owns it.

Open: whether core already has an account-scoped pick or approval record.
Ask the Aleph coordinator (thr_39wwcmwi84) before writing it into the plan.

## Answered 2026-09-29 by the Aleph coordinator (thr_39wwcmwi84)

1. **No general approval store in core.** Nothing on origin/main or
   integrate/aleph-0.5.0 keeps account-scoped picks with a local check API.
   Two narrower designs exist and neither is built:
   - **mk-qap9** (epic, P1, open; plan rev 6 on branch
     `plan/qap9-biometric-root-approval`, APPROVE-WITH-CHANGES; blocked on mk:
     Apple Developer team, P0 key rotation). A hash-bound broker
     (`aleph-rootd` on zklw) verifies Secure Enclave signatures over one exact
     root script. Scope is one action pinned by hash, root scripts only.
   - **Release plan v8 section 5.1** (mk-7l2o): an "mk approval record", JSON
     signed with `ssh-keygen -Y sign -n aleph-approval` using mk's
     touch-required key, listing exact tags, SHAs and digests with
     `batch_valid_until`. Release-only, lives in the release lane.
   The coordinator agrees core should own approvals, built as a
   generalization of the qap9 hash-bound signed-request format and the v8
   record, not a third scheme, and suggests a plan bead naming both as prior
   art. Revision 4 should cite both.
2. **No hotkey or overlay work anywhere.** `apps/desktop` has no
   `globalShortcut` or `alwaysOnTop`; the only `frame: false` is the Linux
   frameless main window. The overlay needs a new main-process window and a
   preload bridge, which is a core desktop change.
