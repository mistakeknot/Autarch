# Git Workflow

## Commit Messages
```
type(scope): description

Types: feat, fix, chore, docs, test, refactor
Scopes: bigend, gurgeh, coldwine, pollard, tui, build
```

## Session Completion

1. Run quality gates (if code changed)
2. **Push to remote** (mandatory): `git pull --rebase && bd sync && git push`
3. Verify `git status` shows "up to date with origin"
