#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
go build -o build/autarch ./cmd/autarch
clavain_source="${CLAVAIN_SOURCE_DIR:-../../os/Clavain}"
if [[ -z "${CLAVAIN_SOURCE_DIR:-}" && -f ../../os/Clavain-feedback-pilot/cmd/clavain-cli/review.go ]]; then
  clavain_source=../../os/Clavain-feedback-pilot
fi
pilot_build_dir="$PWD/build"
(cd "$clavain_source/cmd/clavain-cli" && go build -o "$pilot_build_dir/clavain-cli" .)
mkdir -p build/clavain
cp -rf "$clavain_source/scripts" build/clavain/
cp -rf "$clavain_source/config" build/clavain/
lattice_source="${LATTICE_SOURCE_DIR:-../../interverse/lattice}"
if [[ ! -x build/lattice/.venv/bin/python ]]; then uv venv build/lattice/.venv; fi
uv pip install --python build/lattice/.venv/bin/python "$lattice_source"
swift build --package-path native/AutarchCapture -c release
capture_bin_dir=$(swift build --package-path native/AutarchCapture -c release --show-bin-path)
mkdir -p build/AutarchCapture.app/Contents/MacOS build/AutarchCapture.app/Contents/Resources
cp -f "$capture_bin_dir/AutarchCapture" build/AutarchCapture.app/Contents/MacOS/AutarchCapture
cp -f native/AutarchCapture/Info.plist build/AutarchCapture.app/Contents/Info.plist
rm -f build/AutarchCapture.app/Contents/build-revision.txt
git rev-parse HEAD > build/AutarchCapture.app/Contents/Resources/build-revision.txt
codesign --force --sign - build/AutarchCapture.app
codesign --verify --deep --strict build/AutarchCapture.app
