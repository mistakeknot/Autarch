# Home asks and moves

A Home card is a BB Tasks card whose description carries a `Request: <key> sha256:<identity>` line and a fenced `home-ask` block. `autarch needs-mk file` files it and is idempotent on the request key: the same key with the same content replays, and the same key with changed content is refused.

## Evidence

The optional `evidence` array in a `home-ask/v2` block (also accepted by the Go and TypeScript ask parsers)
holds up to six items. Each has a plain-text `caption` of 1–200 characters and exactly one source:
`url`, `image`, or a `before` and `after` pair. Each side of a pair has one `url` or `image`.
URLs must be absolute HTTPS, at most 2048 characters, without credentials or control characters.
Citadel displays remote URLs as links with their host; it never loads remote images.

An image reference is `{"sha256":"<64 lowercase hex>","mime":"image/png"}`. PNG, JPEG, GIF and WebP
are accepted by their magic bytes, up to 2 MiB per image and 8 MiB per card. In the JSON passed to
`autarch needs-mk file --ask-file`, use `image_path` instead of `image` to import a local file.
The CLI replaces the path with a reference before filing. The separate evidence store defaults to
`~/.autarch/home-evidence`; `AUTARCH_HOME_EVIDENCE` overrides it for both CLI and plugin.
Directories have mode 0700 and files 0600; imports use a temporary file and rename.

For example: `{"caption":"Comparison","before":{"image_path":"./before.png"},"after":{"url":"https://example.test/run"}}`.
Citadel shows bounded local thumbnails, opens the full size on click, and displays “image unavailable”
if bytes cannot be read. Pairs fit side by side or stack with the card width.

Evidence is delivery content excluded from identity, revision, semantic key, request identity, card fingerprint
and normalized JSON. Existing hashes remain unchanged. An evidence-only filing or update is a no-op:
include evidence in the initial filing, or alongside a substantive change to the question or options.

## Options

An ask option has one of <!-- figure:option-kinds=3 -->3 kinds: `instruction`, `needs-context` or `ruling-only`. A `ruling-only` option records the answer and nothing else.

## Moves

A card may carry one move, a `home-move/v1` object. There are <!-- figure:move-kinds=4 -->4 move kinds: `script`, `pr`, `read` and `context`. Home opens no moves: what mk owes is the card's decision. A card filed with only a move asks with Done and Not now, and its question names the command to run, the URL to open or the context to give.

Moves opened before this change were closed when Home started. The rows stay in the database, and no tasks card was closed because of them.
