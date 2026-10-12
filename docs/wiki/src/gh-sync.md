# The gh-sync card filer

`integrations/bb-plugin-autarch/ghsync.ts` classifies open pull requests by who they wait on and plans one decision card per PR waiting on mk. `scripts/gh-sync-cards.ts` files the plan with `autarch needs-mk file`; a dry run is the default and `--apply` files.

## Repo to project

<!-- figure:repo-mappings=14 -->14 GitHub repositories map to Tasks projects in `REPO_PROJECTS`. A repository with no entry is filed under the catch-all project `MSQ`.

## Card kinds

- A single PR: key `gh-ask:<owner>:<repo>:<number>`, asking Merge it, Close it or Leave, with the full PR URL leading the question and in the title. The older `gh-pr:` key is held while a card under it is open.
- A batch of bot-opened PRs that change only CI workflows: key `gh-bots:...`, asking Merge all, Close all or Leave.
- An unmapped repository: key `gh-unmapped:...`, filed under `MSQ`.

An open card under the same request key counts as filed whatever its wording, because the filer's identity includes the title and question.
