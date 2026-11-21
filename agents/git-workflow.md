# Git Workflow

## Commit Messages
```
type(scope): description

Types: feat, fix, chore, docs, test, refactor
Scopes: bigend, gurgeh, coldwine, pollard, tui, build, home, citadel
```

## Session Completion

Work lands through a pull request. Never push to `main` and never force-push.

1. Run quality gates (if code changed)
2. Commit on a work branch, not on `main`, then push the branch: `git push -u origin <branch>`
3. Open a pull request against `main` (`gh pr create`); a maintainer reviews and merges it
4. Verify `git status` shows the branch "up to date with origin/<branch>"
