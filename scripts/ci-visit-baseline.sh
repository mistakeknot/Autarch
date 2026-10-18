#!/usr/bin/env bash
# Manual-only independent guest baseline; no installation or publication.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]]
[[ -n ${CI_SOURCE_SHA:-} && $(git rev-parse HEAD) == "$CI_SOURCE_SHA" ]]
[[ -z $(git status --porcelain) ]]
visit_tools=$(mktemp -d)
trap 'rm -rf "$visit_tools"' EXIT
fetch() {
  curl --fail --location --retry 3 --silent --show-error "$1" -o "$2"
  printf '%s  %s\n' "$3" "$2" | sha256sum --check -
}
fetch https://go.dev/dl/go1.26.4.linux-amd64.tar.gz "$visit_tools/go.tgz" 1153d3d50e0ac764b447adfe05c2bcf08e889d42a02e0fe0259bd47f6733ad7f
tar -xzf "$visit_tools/go.tgz" -C "$visit_tools"
# Extract exact Ubuntu 24.04 packages privately; the guest image is unchanged.
# Fetched from an immutable snapshot: archive.ubuntu.com drops superseded versions (libevent 9ubuntu2.1 went 404).
ubuntu_snapshot=https://snapshot.ubuntu.com/ubuntu/20261001T000000Z/pool/main
fetch "$ubuntu_snapshot/t/tmux/tmux_3.4-1ubuntu0.1_amd64.deb" "$visit_tools/tmux.deb" 2913e17aa61879d1f905aab6f374f153f1ece63aefba1b996e387325d1d5338d
fetch "$ubuntu_snapshot/libe/libevent/libevent-core-2.1-7t64_2.1.12-stable-9ubuntu2.2_amd64.deb" "$visit_tools/event.deb" 2a5264b572f5fe50fc9d63e115d308c79b9fb746d13cc55b5ea1f0f01a78daa7
fetch "$ubuntu_snapshot/libu/libutempter/libutempter0_1.2.1-3build1_amd64.deb" "$visit_tools/utempter.deb" 46c0dfbfeca19ba0773398d654b7e7225b178f26b414f0e2f8250519551bfb32
fetch https://nodejs.org/dist/v22.22.2/node-v22.22.2-linux-x64.tar.xz "$visit_tools/node.txz" 88fd1ce767091fd8d4a99fdb2356e98c819f93f3b1f8663853a2dee9b438068a
tar -xJf "$visit_tools/node.txz" -C "$visit_tools"
for package in tmux event utempter; do dpkg-deb -x "$visit_tools/$package.deb" "$visit_tools/root"; done
export PATH="$visit_tools/node-v22.22.2-linux-x64/bin:$visit_tools/go/bin:$visit_tools/root/usr/bin:$PATH"
export LD_LIBRARY_PATH="$visit_tools/root/usr/lib/x86_64-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export GOWORK=off GOTOOLCHAIN=local GOFLAGS=-modcacherw
export GOCACHE="$visit_tools/go-cache" GOPATH="$visit_tools/gopath" TMUX_TMPDIR="$visit_tools"
unset TMUX TMUX_PANE
go version
go env CGO_ENABLED CC
tmux -V
go build -mod=readonly ./... 2>&1 | tee "$visit_tools/build.log"
go test -mod=readonly -race ./... 2>&1 | tee "$visit_tools/race.log"
# Home plugin: no merge-conflict markers in tracked source, then typecheck, lint (oxlint plus the escape ratchet) and unit tests.
# git grep exits 0 on a match, 1 on none; only 1 is clean, any other status (an error) fails.
marker_rc=0
git grep -nE '^(<<<<<<< |>>>>>>> |=======$)' -- integrations/bb-plugin-autarch ':!*/node_modules/*' ':!*.md' || marker_rc=$?
[[ $marker_rc -eq 1 ]] || { echo "conflict markers in the Home plugin, or the scan failed (git grep exit $marker_rc)" >&2; exit 1; }
node --version
# npm reads no user or global config and keeps its cache under visit_tools; dev dependencies are installed explicitly.
# The tests run from the installed binary with the guest's environment, which some of them assert on.
mkdir -p "$visit_tools/npm"
npm_cfg=(npm_config_cache="$visit_tools/npm/cache" npm_config_userconfig="$visit_tools/npm/user.npmrc" npm_config_globalconfig="$visit_tools/npm/global.npmrc" npm_config_include=dev npm_config_update_notifier=false)
(cd integrations/bb-plugin-autarch && env "${npm_cfg[@]}" npm ci --no-audit --no-fund && env "${npm_cfg[@]}" npm run typecheck && env "${npm_cfg[@]}" npm run lint && ./node_modules/.bin/vitest run --testTimeout=60000) 2>&1 | tee "$visit_tools/plugin.log"
git diff --exit-code
[[ -z $(git status --porcelain) ]]
sha256sum go.mod go.sum integrations/bb-plugin-autarch/package-lock.json "$visit_tools/build.log" "$visit_tools/race.log" "$visit_tools/plugin.log"
printf 'PASS Autarch read-only build, full race suite and Home plugin typecheck and tests source=%s\n' "$CI_SOURCE_SHA"
