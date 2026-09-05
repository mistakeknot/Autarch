#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
go build -o build/autarch ./cmd/autarch
swift build --package-path native/AutarchCapture -c release
capture_bin_dir=$(swift build --package-path native/AutarchCapture -c release --show-bin-path)
mkdir -p build/AutarchCapture.app/Contents/MacOS
cp -f "$capture_bin_dir/AutarchCapture" build/AutarchCapture.app/Contents/MacOS/AutarchCapture
cp -f native/AutarchCapture/Info.plist build/AutarchCapture.app/Contents/Info.plist
git rev-parse HEAD > build/AutarchCapture.app/Contents/build-revision.txt
codesign --force --sign - build/AutarchCapture.app
