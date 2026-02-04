#!/bin/sh
# Post-commit hook: regenerate architecture visualizer if relevant Go files changed.
# Only runs the generator when source-of-truth files are modified.
set -e

# Check if relevant files changed in the latest commit
if git diff-tree --no-commit-id --name-only -r HEAD | grep -qE '(internal/tui/messages\.go|internal/gurgeh/arbiter/types\.go|pkg/signals/signal\.go)'; then
    echo "archviz: relevant source files changed, regenerating..."
    go run ./cmd/archviz
    if [ -n "$(git diff docs/architecture-visualizer.html)" ]; then
        echo "archviz: visualizer updated — stage and amend if desired"
        echo "  git add docs/architecture-visualizer.html && git commit --amend --no-edit"
    fi
fi
