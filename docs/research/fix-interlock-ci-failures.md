# Fix Interlock CI Failures

**Date:** 2026-02-26
**Status:** Completed
**Commit:** `00a92e2` pushed to `main`

## Problem

The Interlock CI build on GitHub Actions was failing with:

```
cmd/interlock-mcp/main.go:10:2: github.com/mistakeknot/interbase@v0.0.0: replacement directory ../../sdk/interbase/go does not exist
internal/tools/tools.go:20:2: github.com/mistakeknot/interbase@v0.0.0: replacement directory ../../sdk/interbase/go does not exist
```

### Root Cause

Interlock's `go.mod` has a `replace` directive:

```
replace github.com/mistakeknot/interbase => ../../sdk/interbase/go
```

This relative path resolves within the Demarch monorepo (where interbase lives at `sdk/interbase/go/`), but GitHub Actions checks out only the `mistakeknot/interlock` repository. The `../../sdk/interbase/go` directory does not exist in CI.

## Analysis

### Dependencies

Interlock has exactly one local replace directive:

| Module | Replace Path | GitHub Repo |
|--------|-------------|-------------|
| `github.com/mistakeknot/interbase` | `../../sdk/interbase/go` | `mistakeknot/interbase` |

The interbase Go module is at `go/` within the interbase repo (the repo root contains both the Go SDK and the Bash SDK). The Go module path is `github.com/mistakeknot/interbase` defined in `sdk/interbase/go/go.mod`.

### Existing Pattern

Autarch's CI (`apps/autarch/.github/workflows/ci.yml`) solves the same problem for its `intermute` dependency:

```yaml
- name: Checkout intermute dependency
  uses: actions/checkout@v4
  with:
    repository: mistakeknot/intermute
    path: _deps/intermute

- name: Override replace directive for CI
  run: go mod edit -replace github.com/mistakeknot/intermute=./_deps/intermute
```

## Fix Applied

Added two steps to `.github/workflows/ci.yml`:

1. **Checkout interbase dependency** -- uses `actions/checkout@v4` to clone `mistakeknot/interbase` into `_deps/interbase/`
2. **Override replace directive** -- uses `go mod edit -replace` to point at `_deps/interbase/go` (the `go/` subdir is where the Go module lives)

### Before

```yaml
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-go@v5
    with:
      go-version: "1.24"
  - run: go build ./...
  - run: go vet ./...
  - run: go test -race ./...
```

### After

```yaml
steps:
  - uses: actions/checkout@v4

  - name: Checkout interbase dependency
    uses: actions/checkout@v4
    with:
      repository: mistakeknot/interbase
      path: _deps/interbase

  - uses: actions/setup-go@v5
    with:
      go-version: "1.24"

  - name: Override replace directive for CI
    run: go mod edit -replace github.com/mistakeknot/interbase=./_deps/interbase/go

  - run: go build ./...
  - run: go vet ./...
  - run: go test -race ./...
```

### Key Detail: `/go` Subdirectory

Unlike Autarch's intermute dependency (where the Go module is at the repo root), interbase's Go module lives in the `go/` subdirectory. The replace path must be `_deps/interbase/go`, not `_deps/interbase`.

## Additional Fix: Unpushed interbase Commit

The first CI run after the workflow fix still failed with:

```
no required module provides package github.com/mistakeknot/interbase/mcputil
```

The `mcputil` package existed locally but the commit (`a7cc97b feat(mcputil): add MCP tool handler middleware`) had not been pushed to `mistakeknot/interbase` on GitHub. CI checks out from GitHub, so it got the older version without `mcputil`.

**Fix:** Pushed the interbase commit to `origin/main`, then re-ran the CI.

## Verification

- Local build (`go build ./...`) passes with existing replace directive
- CI run `22427586366` passes all steps: build, vet, test (-race) -- all green
- Previous CI runs were failing consistently (last two runs both `failure`)
- Two fixes applied: (1) workflow checkout + replace override, (2) push unpushed interbase dependency

## Files Changed

- `/home/mk/projects/Demarch/interverse/interlock/.github/workflows/ci.yml` -- added interbase checkout and replace override steps
- `/home/mk/projects/Demarch/sdk/interbase/` -- pushed existing commit `a7cc97b` (mcputil package) to GitHub
