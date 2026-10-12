# Home asks and moves

A Home card is a BB Tasks card whose description carries a `Request: <key> sha256:<identity>` line and a fenced `home-ask` block. `autarch needs-mk file` files it and is idempotent on the request key: the same key with the same content replays, and the same key with changed content is refused.

## Options

An ask option has one of <!-- figure:option-kinds=3 -->3 kinds: `instruction`, `needs-context` or `ruling-only`. A `ruling-only` option records the answer and nothing else.

## Moves

A card may carry one move, a `home-move/v1` object. There are <!-- figure:move-kinds=4 -->4 move kinds: `script`, `pr`, `read` and `context`. A `pr` move names a pull request by `url`; when the PR is merged or closed on GitHub the poller closes the move and the card.

A pull request that waits on mk is shown as a plain decision card in Decide. Its `pr` move stays in the database as a tracker and is not drawn as a separate card.
