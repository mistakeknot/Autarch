# Gates: what only mk can merge or deploy

Agents may push a work branch and open a pull request. They never merge to the default branch, tag, release or publish.

Some merges and deploys are delegated by standing approvals, and the vizier makes those only when the approval's conditions hold. Everything else is yours: public-facing deploys, spends, credentials, settings, hooks and instruction files.

When a card asks for one of these, it names the exact thing: a pull request URL, or a script with its full path and sha256.
