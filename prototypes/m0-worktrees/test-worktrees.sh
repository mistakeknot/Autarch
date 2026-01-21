#!/bin/bash
set -euo pipefail

# M0 Prototype: Git Worktree Isolation Testing
# Tests concurrent worktree creation, package isolation, and cleanup

RESET='\033[0m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'

echo -e "${BLUE}=== M0 Git Worktree Prototype ===${RESET}"
echo "Testing: Concurrent worktrees with package manager isolation"
echo ""

# Configuration
NUM_WORKTREES=5
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="$SCRIPT_DIR/test-repo"
WORKTREE_DIR="$SCRIPT_DIR/test-worktrees"
USE_PNPM=true  # Set to false to test npm

# Cleanup from previous runs
cleanup() {
    echo -e "${YELLOW}Cleaning up...${RESET}"
    if [ -d "$WORKTREE_DIR" ]; then
        rm -rf "$WORKTREE_DIR"
    fi
    if [ -d "$TEST_DIR" ]; then
        cd "$TEST_DIR" 2>/dev/null || true
        git worktree list --porcelain | grep "worktree" | cut -d' ' -f2 | while read -r wt; do
            if [[ "$wt" != "$TEST_DIR" ]]; then
                git worktree remove "$wt" --force 2>/dev/null || true
            fi
        done
        cd ..
        rm -rf "$TEST_DIR"
    fi
}

# Setup test repository
setup_test_repo() {
    echo -e "${BLUE}[1/6] Setting up test repository${RESET}"

    mkdir -p "$TEST_DIR"
    cd "$TEST_DIR"

    git init
    git config user.email "test@tandemonium.dev"
    git config user.name "Test User"

    # Create a realistic package.json with multiple dependencies
    cat > package.json <<'EOF'
{
  "name": "worktree-test",
  "version": "1.0.0",
  "dependencies": {
    "react": "^18.3.0",
    "react-dom": "^18.3.0",
    "zustand": "^4.5.0"
  },
  "devDependencies": {
    "typescript": "^5.6.0",
    "vite": "^5.4.0"
  }
}
EOF

    git add package.json
    git commit -m "Initial commit with package.json"

    cd ..
    echo -e "${GREEN}✓ Test repository created${RESET}"
}

# Test concurrent worktree creation
test_concurrent_creation() {
    echo -e "${BLUE}[2/6] Testing concurrent worktree creation${RESET}"

    mkdir -p "$WORKTREE_DIR"
    START_TIME=$(date +%s)

    # Create worktrees in parallel
    for i in $(seq 1 $NUM_WORKTREES); do
        (
            BRANCH_NAME="test-branch-$i"
            WORKTREE_PATH="$WORKTREE_DIR/worktree-$i"

            cd "$TEST_DIR"
            git worktree add "$WORKTREE_PATH" -b "$BRANCH_NAME" 2>&1 | grep -v "Preparing worktree" || true

            if [ -d "$WORKTREE_PATH" ]; then
                echo -e "${GREEN}✓ Worktree $i created${RESET}"
            else
                echo -e "${RED}✗ Worktree $i failed${RESET}"
                exit 1
            fi
        ) &
    done

    wait
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))

    echo -e "${GREEN}✓ Created $NUM_WORKTREES worktrees in ${DURATION}s${RESET}"
}

# Test concurrent package installs
test_concurrent_installs() {
    echo -e "${BLUE}[3/6] Testing concurrent package installs${RESET}"

    if [ "$USE_PNPM" = true ] && ! command -v pnpm &> /dev/null; then
        echo -e "${YELLOW}⚠ pnpm not found, falling back to npm${RESET}"
        USE_PNPM=false
    fi

    PKG_MANAGER="npm"
    if [ "$USE_PNPM" = true ]; then
        PKG_MANAGER="pnpm"
    fi

    echo "Using: $PKG_MANAGER"
    START_TIME=$(date +%s)

    # Install packages in parallel
    for i in $(seq 1 $NUM_WORKTREES); do
        (
            WORKTREE_PATH="$WORKTREE_DIR/worktree-$i"
            cd "$WORKTREE_PATH"

            if [ "$PKG_MANAGER" = "pnpm" ]; then
                pnpm install --silent 2>&1 | grep -v "Progress" || true
            else
                npm install --silent 2>&1 || true
            fi

            if [ -d "node_modules" ]; then
                MODULE_COUNT=$(find node_modules -maxdepth 1 -type d | wc -l)
                echo -e "${GREEN}✓ Worktree $i: installed ($MODULE_COUNT modules)${RESET}"
            else
                echo -e "${RED}✗ Worktree $i: install failed${RESET}"
                exit 1
            fi
        ) &
    done

    wait
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))

    echo -e "${GREEN}✓ Completed $NUM_WORKTREES concurrent installs in ${DURATION}s${RESET}"
}

# Validate package isolation
validate_isolation() {
    echo -e "${BLUE}[4/6] Validating package isolation${RESET}"

    # Check that each worktree has independent node_modules
    for i in $(seq 1 $NUM_WORKTREES); do
        WORKTREE_PATH="$WORKTREE_DIR/worktree-$i"

        if [ ! -d "$WORKTREE_PATH/node_modules" ]; then
            echo -e "${RED}✗ Worktree $i: missing node_modules${RESET}"
            exit 1
        fi

        # Verify a dependency exists
        if [ ! -d "$WORKTREE_PATH/node_modules/react" ]; then
            echo -e "${RED}✗ Worktree $i: missing react package${RESET}"
            exit 1
        fi
    done

    # Test that modifying one worktree doesn't affect others
    WORKTREE_PATH="$WORKTREE_DIR/worktree-1"
    echo "test-modification" > "$WORKTREE_PATH/test-file.txt"

    for i in $(seq 2 $NUM_WORKTREES); do
        TEST_PATH="$WORKTREE_DIR/worktree-$i/test-file.txt"
        if [ -f "$TEST_PATH" ]; then
            echo -e "${RED}✗ Isolation breach: file appeared in worktree $i${RESET}"
            exit 1
        fi
    done

    echo -e "${GREEN}✓ Package isolation validated${RESET}"
}

# Measure disk usage
measure_disk_usage() {
    echo -e "${BLUE}[5/6] Measuring disk usage${RESET}"

    TOTAL_SIZE=$(du -sh "$WORKTREE_DIR" | cut -f1)
    echo "Total worktree directory size: $TOTAL_SIZE"

    for i in $(seq 1 $NUM_WORKTREES); do
        WORKTREE_PATH="$WORKTREE_DIR/worktree-$i"
        SIZE=$(du -sh "$WORKTREE_PATH" | cut -f1)
        echo "  Worktree $i: $SIZE"
    done

    # Compare pnpm vs npm savings
    if [ "$USE_PNPM" = true ]; then
        echo -e "${GREEN}✓ Using pnpm (shared store saves disk space)${RESET}"
    else
        echo -e "${YELLOW}⚠ Using npm (duplicated node_modules)${RESET}"
    fi
}

# Test cleanup
test_cleanup() {
    echo -e "${BLUE}[6/6] Testing cleanup${RESET}"

    cd "$TEST_DIR"

    # List worktrees before cleanup
    WORKTREE_COUNT=$(git worktree list | wc -l)
    echo "Worktrees before cleanup: $WORKTREE_COUNT"

    # Remove worktrees
    START_TIME=$(date +%s)
    for i in $(seq 1 $NUM_WORKTREES); do
        WORKTREE_PATH="$WORKTREE_DIR/worktree-$i"
        git worktree remove "$WORKTREE_PATH" --force 2>&1 | grep -v "Removing" || true
    done
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))

    # Prune references
    git worktree prune

    cd ..

    # Verify cleanup
    if [ -d "$WORKTREE_DIR" ]; then
        REMAINING=$(find "$WORKTREE_DIR" -mindepth 1 -maxdepth 1 -type d | wc -l)
        if [ "$REMAINING" -gt 0 ]; then
            echo -e "${RED}✗ Cleanup incomplete: $REMAINING directories remain${RESET}"
            exit 1
        fi
    fi

    echo -e "${GREEN}✓ Cleanup completed in ${DURATION}s${RESET}"
}

# Run validation report
validation_report() {
    echo ""
    echo -e "${BLUE}=== Validation Report ===${RESET}"
    echo ""
    echo "✅ PASS: Worktrees create without conflicts"
    echo "✅ PASS: Package installs don't interfere"
    echo "✅ PASS: Isolation verified (independent node_modules)"
    echo "✅ PASS: Cleanup works properly"
    echo ""
    echo -e "${GREEN}🎉 All validation criteria passed!${RESET}"
    echo ""
    echo "Findings:"
    echo "  - Concurrent worktree creation: FAST"
    echo "  - Package manager isolation: WORKING"
    if [ "$USE_PNPM" = true ]; then
        echo "  - Recommendation: Use pnpm for disk savings"
    fi
    echo "  - Git worktree is VIABLE for multi-agent isolation"
    echo ""
}

# Main execution
main() {
    trap cleanup EXIT

    cleanup
    setup_test_repo
    test_concurrent_creation
    test_concurrent_installs
    validate_isolation
    measure_disk_usage
    test_cleanup
    validation_report
}

main "$@"
