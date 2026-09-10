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
fetch https://archive.ubuntu.com/ubuntu/pool/main/t/tmux/tmux_3.4-1ubuntu0.1_amd64.deb "$visit_tools/tmux.deb" 2913e17aa61879d1f905aab6f374f153f1ece63aefba1b996e387325d1d5338d
fetch https://archive.ubuntu.com/ubuntu/pool/main/libe/libevent/libevent-core-2.1-7t64_2.1.12-stable-9ubuntu2.1_amd64.deb "$visit_tools/event.deb" f8af913312f8de9cab6efcd2259918b3df92f5b7d501aba7ce9a55699fa452b8
fetch https://archive.ubuntu.com/ubuntu/pool/main/libu/libutempter/libutempter0_1.2.1-3build1_amd64.deb "$visit_tools/utempter.deb" 46c0dfbfeca19ba0773398d654b7e7225b178f26b414f0e2f8250519551bfb32
for package in tmux event utempter; do dpkg-deb -x "$visit_tools/$package.deb" "$visit_tools/root"; done
export PATH="$visit_tools/go/bin:$visit_tools/root/usr/bin:$PATH"
export LD_LIBRARY_PATH="$visit_tools/root/usr/lib/x86_64-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export GOWORK=off GOTOOLCHAIN=local GOFLAGS=-modcacherw
export GOCACHE="$visit_tools/go-cache" GOPATH="$visit_tools/gopath" TMUX_TMPDIR="$visit_tools"
unset TMUX TMUX_PANE
go version
go env CGO_ENABLED CC
tmux -V
go build -mod=readonly ./... 2>&1 | tee "$visit_tools/build.log"
go test -mod=readonly -race ./... 2>&1 | tee "$visit_tools/race.log"
git diff --exit-code
[[ -z $(git status --porcelain) ]]
sha256sum go.mod go.sum "$visit_tools/build.log" "$visit_tools/race.log"
printf 'PASS Autarch read-only build and full race suite source=%s\n' "$CI_SOURCE_SHA"
