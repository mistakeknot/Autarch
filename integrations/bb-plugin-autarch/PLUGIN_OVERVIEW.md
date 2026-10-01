Keep what is waiting for you in one place, in the sidebar and in your agent threads.

## What you get

- A **Home** page in the left sidebar with the asks waiting for a ruling, a catch-up
  feed of what happened while you were away, the vizier thread, and delegation settings.
- A `bb home` command that lets agents look up cards and asks. Agents file new asks as `needs-mk` cards with `autarch needs-mk file`.
- An **Example todos** page that adds, completes, and removes todos.
- Live updates, so a change made in one place reaches every open page at once.

## How it works

Home data lives in this plugin's own storage on the BB server. The plugin reads a local
Autarch service on this machine, which it does not start, and needs no account, API key, or external service.

## For agents

The bundled skill tells an agent to file an ask with `autarch needs-mk file` and look it up
with `bb home get`. `bb home progress`, `resolve` and `withdraw` are kept for asks filed before
cards.
