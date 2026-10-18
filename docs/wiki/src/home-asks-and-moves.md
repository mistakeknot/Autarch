# Home asks and moves

A Home card is a BB Tasks card whose description carries a `Request: <key> sha256:<identity>` line and a fenced `home-ask` block. `autarch needs-mk file` files it and is idempotent on the request key: the same key with the same content replays, and the same key with changed content is refused.

## Options

An ask option has one of <!-- figure:option-kinds=3 -->3 kinds: `instruction`, `needs-context` or `ruling-only`. A `ruling-only` option records the answer and nothing else.

## Moves

A card may carry one move, a `home-move/v1` object. There are <!-- figure:move-kinds=4 -->4 move kinds: `script`, `pr`, `read` and `context`. Home opens no moves: what mk owes is the card's decision. A card filed with only a move asks with Done and Not now, and its question names the command to run, the URL to open or the context to give.

Moves opened before this change were closed when Home started. The rows stay in the database, and no tasks card was closed because of them.
