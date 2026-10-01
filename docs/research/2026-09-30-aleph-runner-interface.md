# Aleph root-runner interface (mk-b87j.5), as reported by the Aleph coordinator 2026-09-30

Source: Aleph coordinator thr_39wwcmwi84, reviewed branch slice5a (4fb84f0) and the decision-panel README "Run items".

## Slice A: run by paste, "not authenticated" (built; install pending as mk-todo #420)
- No RPC or HTTP execution path, deliberately. Callers only DISPLAY a command; mk pasting it is the launch.
- Hand-off: write a run item into a decision-panel set with `todo-add --set <SET_ID> --file item.json` (exit 2 on refusal).
  Fields: `run_as: "mk"`, exactly one `script` (absolute path), `script_sha256` (hex), `owner_thread`, optional
  `run_timeout_s` (1-3600, default 900). No args. Label unique per set. A card's {script path, sha256, timeout} maps 1:1.
- `todo-add` pins the bytes (<=256 KiB, regular file, absolute path with no symlink or `..`, shebang with <=1 arg,
  interpreter in {/bin/sh, /bin/bash, /usr/bin/bash, /usr/bin/python3, /usr/bin/python3.12}, root-owned with root-owned
  parents) and prints `runuser -u mk -- <ROOT>/bin/todo-run <set> <item> <script_sha256>`. The runner re-verifies the sha at launch.
- Time limit: systemd RuntimeMaxSec. Status: `todo-run --status <set> <item>` (exit 0 handed off, 2 usage, 3 refused, 4 launch_failed,
  5 start not yet reported); phases launching/running/finalizing; terminal exited/killed/timeout/launch_failed/interrupted.
  Max 3 concurrent; once per item (a retry is a new item). Label text: "run by paste, not authenticated" everywhere; nothing says "approved by mk".
- `run_as: "mk"` runs in mk's user manager scope; the zklw-root part is mk pasting from a root-capable shell.

## Slice B: passkey page (zklw:444): NOT built, gated on mk leaving the docker group (ALPH-3 / mk-todo #419) and v5 section 5 open items.
Requester input will be the pinned tuple (set, item, script path, sha256, timeout, owner_thread). Return shape unspecified. Do not design Home against it.

## Aleph's asks of Home
(a) Thin card->item adapter owned in decision-panel (`todo-add --from-card`); Home tells Aleph the card fields.
(b) Stable id link both ways (card key <-> set/item).
(c) Card state is untrusted (same-uid agents write cards): Home may show a run command, never mark anything approved; root approval = slice B only.
(d) Read-only consumption of `todo-run --status` JSON (Aleph can add `--json`) to show phase on the card.
Home must NOT exec `todo-run` and must not write status records.
