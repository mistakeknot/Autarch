# escalation-check

`escalation-check` checks, in code, the evidence an escalation cites. It does not
judge anything that needs a person or a model: whether a ruling covers the action,
whether a review was good, whether a design is right. PASS is evidence, never
authority; the reviewer and rehearsal state are the tools' own output and are not
proven to come from who they claim.

```
escalation-check [--script PATH --sha SHA256] [--pr URL [--post-merge] [--require-check NAME]...]
                 [--review FILE] [--ruling ID [--rulings FILE]] [--offline] [--rehearse-state DIR] [--pretty]
```

Output is one JSON report (`escalation-check/v1`) with a PASS, FAIL or UNKNOWN per
item. The roll-up is FAIL if any item fails, else UNKNOWN if any is unknown, else
PASS. Exit codes: 0 PASS, 1 FAIL, 3 UNKNOWN, 2 usage error. An item you asked for
is never silently skipped.

Items: `script.path`, `script.sha256`, `script.lint`, `script.exec_bit`,
`script.rehearsed`, `script.stable`; `pr.url`, `pr.state`, `pr.checks`, `pr.head`;
`review.verdict`, `review.binds`; `ruling.present`, `ruling.shape`.

## Rules worth knowing

- `--script` needs `--sha`: the digest being vouched for comes from the caller.
  The script is read once, linted from a copy of those bytes, and read again at the
  end; a change voids every script item.
- `script.rehearsed` runs `handoff-rehearse --verify` and also cross-checks the
  receipt it points to (inside the rehearsal state directory, same sha256 and
  script path, `passed`, `check_rc` 0, matching uid).
- The pull request is read twice with `gh --repo github.com/OWNER/REPO`; the second
  read supplies the state, checks and head, and a head that moved is a FAIL. Every
  reported check must be green and every `--require-check` name present and green.
  Unrecognised states are UNKNOWN. `--post-merge` expects MERGED instead of OPEN.
- A review receipt counts only through its trailing block: `REVIEWED-PR:
  OWNER/REPO#N`, `REVIEWED-HEAD: <40 hex>`, `REVIEWED-SCRIPT-SHA256: <64 hex>`
  (within the last 12 non-empty lines), then `VERDICT: PASS|PASS WITH NOTES|FAIL`
  as the last line. Reviews without the block are UNKNOWN. `NOTE:` lines of a
  PASS WITH NOTES review are returned for a person to disposition.
- The ledger is `$XDG_STATE_HOME/vizier/rulings.jsonl` (or `--rulings`). A row's ID
  is `id` or `ruling_id`. Any malformed line, or the same ID with different
  contents, makes the result UNKNOWN; an ID absent from a clean ledger is FAIL.
- Threat model: a concurrent adversarial writer on the same machine is out of scope.
