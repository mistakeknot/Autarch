# Home asks and moves

A Home card is a BB Tasks card whose description carries a `Request: <key> sha256:<identity>` line and a fenced `home-ask` block. `autarch needs-mk file` files it and is idempotent on the request key: the same key with the same content replays, and the same key with changed content is refused.

## Blocker urgency

`autarch needs-mk file --urgency blocker --stopped "Release waits for this answer"`
requests one phone push when the card is filed. The ask JSON equivalent is
`"urgency": "blocker", "stopped": "Release waits for this answer"`; flags override JSON.
`stopped` must be a trimmed, non-blank single line of at most 200 characters describing
what work is stopped until the answer. It is allowed only with blocker urgency.
A missing or invalid field exits 2 before filing. Add the field or omit blocker urgency
(no phone push). The filer checks the field, without interpreting the question.
Existing cards may omit `stopped`; readers still validate its shape and blocker urgency
when the field is present. The required-field rule applies to new filing input.
Like urgency, `stopped` participates in the filer request identity (H1). It stays
out of normalized JSON, revision, semantic key, and the Citadel card fingerprint.

`bb home pushes --by-thread --days 7` returns JSON rows `{thread, blocker_cards}`,
ordered by descending count and then thread. The default window is seven days
(1–365 allowed). This read-only audit counts each distinct card carrying blocker
urgency once, including legacy cards without `stopped`, whether or not its phone
push succeeded. The window uses the first generation’s filing time, so an old card
edited this week stays outside this week’s report. It uses the recorded filing thread
and excludes older asks that have no task/card ID.

### Deployment and rollback

Deploy the Citadel plugin with this reader support before (or together with) the filer
that writes `stopped`. Rolling the plugin back below this change hides cards carrying
`stopped`, because the older plugin rejects that field. Keep the compatible plugin
while rolling back the filer; do not roll the plugin back without accounting for
cards and stored bodies that already carry the field.

## Evidence

The optional `evidence` array in a `home-ask/v2` block (also accepted by the Go and TypeScript ask parsers)
holds up to six items. Each has a plain-text `caption` of 1–200 characters and exactly one source:
`url`, `image`, or a `before` and `after` pair. Each side of a pair has one `url` or `image`.
URLs must be absolute HTTPS, at most 2048 characters, without credentials or control characters.
Citadel displays remote URLs as links with their host; it never loads remote images.

Both validators use the same conservative grammar: literal `https://`, followed by a lowercase
ASCII host of at most 253 bytes. Host labels are 1–63 letters, digits or hyphens, without edge
hyphens or an `xn--` prefix. Non-IP final labels contain a letter and do not start with `0x`,
avoiding browser numeric-host interpretation. IPv4 uses exactly four decimal octets, each 0–255,
without leading zeros. An optional port is decimal 1–65535 without leading zeros. IPv6 and
userinfo are refused. An optional `/path`, `?query` and `#fragment` follow in that order.
Their allowlist is ASCII letters, digits, `._~-!$&'()*+,;=:@/` and percent escapes with exactly
two hexadecimal digits. Query and fragment also allow `?`. No raw whitespace or non-ASCII is accepted.

Caption emptiness uses this explicit trim set in both languages: U+0009–000D, 0020, 00A0,
1680, 2000–200A, 2028, 2029, 202F, 205F, 3000 and FEFF. Stored captions are unchanged;
C0 and C1 control characters (U+0000–001F and 007F–009F) are refused separately.
The shared acceptance vectors live in `testdata/evidence-vectors.json`.
Evidence member names are case-sensitive at every level. Duplicate JSON members use
last-value-wins semantics, including replacement of the whole previous object, matching `JSON.parse`.
The filer writes evidence from the validated struct with canonical field names.

An image reference is `{"sha256":"<64 lowercase hex>","mime":"image/png"}`. PNG, JPEG, GIF and WebP
are accepted by their magic bytes, up to 2 MiB per image and 8 MiB per card. In the JSON passed to
`autarch needs-mk file --ask-file`, use `image_path` instead of `image` to import a local file.
The CLI replaces the path with a reference before filing. The separate evidence store defaults to
`~/.autarch/home-evidence`; `AUTARCH_HOME_EVIDENCE` overrides it for both CLI and plugin.
An empty or relative override selects the default in both languages. Local imports must be regular files.
Directories have mode 0700 and files 0600; imports use a temporary file and rename.

For example: `{"caption":"Comparison","before":{"image_path":"./before.png"},"after":{"url":"https://example.test/run"}}`.
Citadel shows bounded local thumbnails, opens the full size on click, and displays “image unavailable”
if bytes cannot be read. Pairs fit side by side or stack with the card width.

Evidence is delivery content excluded from identity, revision, semantic key, request identity, card fingerprint
and normalized JSON. Existing hashes remain unchanged. An evidence-only filing or update is a no-op:
include evidence in the initial filing, or alongside a substantive change to the question or options.

## Pick export

`autarch needs-mk export --db ./data.db --days 7` reads Home's stored card picks
as JSON Lines. The default window is seven days (1–365 allowed), inclusive through
the start of the command. It includes every recorded card pick by the owner in that
window, including earlier generations, ordered by pick time then pick ID. It opens
SQLite read-only and moves no cursor. Database discovery matches `needs-mk footer`.

Each line starts with `id`, `time`, `state`, `pick`, `title`, matching the five
columns of the earlier research export. `id` is the card key, falling back to task ID;
state and title describe the current stored card. Added `decision_id` and `pick_id`
distinguish repeated picks. Recommendation data comes from that pick's decision
snapshot, rather than the card's latest generation.

| Added field | Meaning |
| --- | --- |
| `recommendation_id` | Stored recommended option ID; null if absent. |
| `matched_recommendation` | Exact option-ID comparison, or null without a recommendation. An own-words pick differs from a recommended option. |
| `question_recommendations` | Per-question IDs and recommendations for multi-pick cards. Their `pick` and `matched_recommendation` are null: storage retains readable answer labels rather than structured option IDs. Null for cards without questions. |
| `kind` | Stored ask kind (`decide`, `steps`, or `machine`); cards use `decide`. This is not an inferred merge/question classification. |
| `pr_head` | The selected option's structured merge-approval identity when it is a full lowercase 40-character commit SHA; otherwise null. Prose is never mined for a head. |
| `round` | Null: no structured round is retained. |
| `auto_close` | Null: conversion to the stored v1 ask drops these terms; original v2 ask JSON is not retained. Conditions in prose are not interpreted. |
| `revisions` | Null: generation includes overrides and bring-backs, so it is not a revision count. An edit history between observations is unavailable. |

Missing historical values are never reconstructed. This export records data only;
it applies no rules and takes no action on picks.

## Options

An ask option has one of <!-- figure:option-kinds=3 -->3 kinds: `instruction`, `needs-context` or `ruling-only`. A `ruling-only` option records the answer and nothing else.

## Moves

A card may carry one move, a `home-move/v1` object. There are <!-- figure:move-kinds=4 -->4 move kinds: `script`, `pr`, `read` and `context`. Home opens no moves: what mk owes is the card's decision. A card filed with only a move asks with Done and Not now, and its question names the command to run, the URL to open or the context to give.

Moves opened before this change were closed when Home started. The rows stay in the database, and no tasks card was closed because of them.
