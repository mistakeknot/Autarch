#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
# Fail before building or touching an existing bundle if signing is unavailable.
# Empty configuration selects the sole Apple Development identity. Multiple
# identities require an explicit SHA-1 or exact name; '-' is never accepted.
AUTARCH_SIGNING_IDENTITY=$(python3 scripts/review-pilot-signing.py preflight)
export AUTARCH_SIGNING_IDENTITY
: "${CLAVAIN_SOURCE_DIR:?Explicit Clavain source checkout required}"
: "${LATTICE_SOURCE_DIR:?Explicit Lattice source checkout required}"
pin_source() {
  local source_dir="$1"
  [[ "$(git -C "$source_dir" symbolic-ref --short HEAD)" == main ]] || { echo "Source checkout must be on main: $source_dir" >&2; return 1; }
  [[ -z "$(git -C "$source_dir" status --porcelain)" ]] || { echo "Source checkout must be clean: $source_dir" >&2; return 1; }
  git -C "$source_dir" rev-parse HEAD
}
clavain_source="$(cd "$CLAVAIN_SOURCE_DIR" && pwd -P)"
lattice_source="$(cd "$LATTICE_SOURCE_DIR" && pwd -P)"
autarch_sha="$(pin_source "$PWD")"
clavain_sha="$(pin_source "$clavain_source")"
lattice_sha="$(pin_source "$lattice_source")"
# build/ is mutable build output; installed packages remain immutable. Preserve
# every signing receipt while allowing the accepted revision to rebuild here.
mkdir -p build/signing
signing_attempt=$(mktemp -d "$PWD/build/signing/attempt.XXXXXX")
rm -f build/review-pilot-signing.json
go build -o build/autarch ./cmd/autarch
pilot_build_dir="$PWD/build"
(cd "$clavain_source/cmd/clavain-cli" && go build -o "$pilot_build_dir/clavain-cli" .)
mkdir -p build/clavain
cp -rf "$clavain_source/scripts" build/clavain/
cp -rf "$clavain_source/config" build/clavain/
if [[ ! -x build/lattice/.venv/bin/python ]]; then uv venv build/lattice/.venv; fi
uv pip install --python build/lattice/.venv/bin/python "$lattice_source"
swift build --package-path native/AutarchCapture -c release
capture_bin_dir=$(swift build --package-path native/AutarchCapture -c release --show-bin-path)
mkdir -p build/AutarchCapture.app/Contents/MacOS build/AutarchCapture.app/Contents/Resources
cp -f "$capture_bin_dir/AutarchCapture" build/AutarchCapture.app/Contents/MacOS/AutarchCapture
cp -f native/AutarchCapture/Info.plist build/AutarchCapture.app/Contents/Info.plist
rm -f build/AutarchCapture.app/Contents/build-revision.txt
[[ "$(pin_source "$PWD")" == "$autarch_sha" && "$(pin_source "$clavain_source")" == "$clavain_sha" && "$(pin_source "$lattice_source")" == "$lattice_sha" ]] || { echo 'Source moved during build' >&2; exit 1; }
printf '%s\n' "$autarch_sha" > build/AutarchCapture.app/Contents/Resources/build-revision.txt
python3 - "$PWD" "$autarch_sha" "$clavain_source" "$clavain_sha" "$lattice_source" "$lattice_sha" <<'PY'
import hashlib, json, sys
from pathlib import Path
sources = {name: {"path": sys.argv[i], "commit": sys.argv[i+1]} for name, i in [("autarch", 1), ("clavain", 3), ("lattice", 5)]}
files = {name: hashlib.sha256(Path(name).read_bytes()).hexdigest() for name in ["build/autarch", "build/clavain-cli"]}
# The signature changes the capture executable. Its final bytes are bound by
# the existing post-sign receipt; embedding its own signed hash is circular.
receipt = json.dumps({"version": 1, "sources": sources, "binaries": files, "signed_bundle_receipt": "review-pilot-signing.json"}, indent=2) + "\n"
Path("build/AutarchCapture.app/Contents/Resources/source-bindings.json").write_text(receipt)
Path("build/review-pilot-sources.json").write_text(receipt)
PY
python3 scripts/review-pilot-signing.py sign build/AutarchCapture.app "$signing_attempt/signing.json"
cp -f "$signing_attempt/signing.json" build/review-pilot-signing.json
