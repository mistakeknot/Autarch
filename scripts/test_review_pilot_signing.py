"""Signing contract tests; fake platform tools never access a real keychain."""
import json
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("review-pilot-signing.py")
IDENTITY = "A" * 40
NAME = "Apple Development: Pilot Test (TEST123456)"


class SigningTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.env = dict(os.environ, PATH=str(self.root) + os.pathsep + os.environ["PATH"],
                        IDENTITIES=f'  1) {IDENTITY} "{NAME}"\n',
                        CALLS=str(self.root / "calls"))
        self.env.pop("AUTARCH_SIGNING_IDENTITY", None)
        self.tool("security", '#!/bin/sh\nprintf "%s" "$IDENTITIES"\n')
        self.tool("codesign", '''#!/bin/sh
echo "$*" >> "$CALLS"
case "$1" in
  --display) echo 'designated => identifier "org.autarch.review.capture" and anchor apple generic' >&2;;
esac
exit "${SIGN_EXIT:-0}"
''')

    def tool(self, name, text):
        p = self.root / name
        p.write_text(text)
        p.chmod(0o755)

    def run_signer(self, *args):
        return subprocess.run(["python3", str(SCRIPT), *args], env=self.env,
                              capture_output=True, text=True)

    def test_selects_single_apple_development_identity(self):
        p = self.run_signer("preflight")
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertEqual(p.stdout.strip(), IDENTITY)

    def test_missing_identity_fails_without_signing(self):
        self.env["IDENTITIES"] = "0 valid identities found"
        p = self.run_signer("preflight")
        self.assertNotEqual(p.returncode, 0)
        self.assertIn("Apple Development", p.stderr)
        self.assertFalse((self.root / "calls").exists())

    def test_ambiguous_identity_requires_configuration(self):
        self.env["IDENTITIES"] += f'  2) {"B" * 40} "Apple Development: Other (OTHER)"\n'
        p = self.run_signer("preflight")
        self.assertNotEqual(p.returncode, 0)
        self.assertIn("AUTARCH_SIGNING_IDENTITY", p.stderr)
        self.env["AUTARCH_SIGNING_IDENTITY"] = IDENTITY
        self.assertEqual(self.run_signer("preflight").stdout.strip(), IDENTITY)

    def test_rejects_adhoc_or_unavailable_configuration(self):
        for identity in ("-", "missing", "Developer ID Application: Other"):
            self.env["AUTARCH_SIGNING_IDENTITY"] = identity
            p = self.run_signer("preflight")
            self.assertNotEqual(p.returncode, 0)

    def app(self):
        app = self.root / "AutarchCapture.app"
        (app / "Contents/MacOS").mkdir(parents=True)
        (app / "Contents/Info.plist").write_bytes(plistlib.dumps({
            "CFBundleIdentifier": "org.autarch.review.capture"}))
        (app / "Contents/MacOS/AutarchCapture").write_bytes(b"test binary")
        return app

    def test_signing_uses_default_requirement_and_records_hashes(self):
        app = self.app()
        evidence = self.root / "signing.json"
        p = self.run_signer("sign", str(app), str(evidence))
        self.assertEqual(p.returncode, 0, p.stderr)
        data = json.loads(evidence.read_text())
        self.assertEqual(data["identity_sha1"], IDENTITY)
        self.assertEqual(data["identity_name"], NAME)
        self.assertIn("anchor apple", data["designated_requirement"])
        self.assertEqual(len(data["files"]["Contents/MacOS/AutarchCapture"]), 64)
        calls = (self.root / "calls").read_text()
        self.assertIn("--force --sign " + IDENTITY, calls)
        self.assertIn("--verify --deep --strict", calls)
        self.assertNotIn("--requirements", calls)
        self.assertNotIn("--preserve-metadata", calls)

    def test_signing_failure_emits_no_success_evidence(self):
        self.env["SIGN_EXIT"] = "1"
        p = self.run_signer("sign", str(self.app()), str(self.root / "signing.json"))
        self.assertNotEqual(p.returncode, 0)
        self.assertFalse((self.root / "signing.json").exists())

    def test_existing_evidence_refuses_resigning(self):
        evidence = self.root / "signing.json"
        evidence.write_text("sealed")
        p = self.run_signer("sign", str(self.app()), str(evidence))
        self.assertNotEqual(p.returncode, 0)
        self.assertEqual(evidence.read_text(), "sealed")
        self.assertFalse((self.root / "calls").exists())

    def test_repeated_build_preserves_receipts_and_failed_rebuild_clears_current(self):
        repo = self.root / "repo"
        (repo / "scripts").mkdir(parents=True)
        for name in ("build-review-pilot.sh", "review-pilot-signing.py"):
            shutil.copyfile(SCRIPT.with_name(name), repo / "scripts" / name)
        (repo / "native/AutarchCapture").mkdir(parents=True)
        (repo / "native/AutarchCapture/Info.plist").write_bytes(plistlib.dumps({
            "CFBundleIdentifier": "org.autarch.review.capture"}))
        clavain = self.root / "clavain"
        for directory in ("cmd/clavain-cli", "scripts", "config", ".claude-plugin", "docs/canon"):
            (clavain / directory).mkdir(parents=True)
        (clavain / ".claude-plugin/plugin.json").write_text('{"version":"test"}')
        (clavain / "docs/canon/reasoning-routing.md").write_text("Fixture routing canon")
        (clavain / "scripts/sync-agent-instructions.py").write_text('''
import json, pathlib, sys
source = pathlib.Path(sys.argv[sys.argv.index('--source') + 1])
assert json.loads((source / '.claude-plugin/plugin.json').read_text())['version']
assert (source / 'docs/canon/reasoning-routing.md').read_text()
print('Fixture reasoning contract')
''')
        self.env["CLAVAIN_SOURCE_DIR"] = str(clavain)
        self.env["LATTICE_SOURCE_DIR"] = str(self.root / "lattice")
        (self.root / "lattice").mkdir()
        self.env["CAPTURE_BIN"] = str(self.root / "capture-bin")
        self.env["BUILD_REVISION"] = "first"
        self.tool("go", '''#!/bin/sh
test "$1" = build && test "$2" = -o || exit 1
printf '%s' "$BUILD_REVISION" > "$3"
''')
        self.tool("uv", '''#!/bin/sh
if test "$1" = venv; then
  mkdir -p "$2/bin"
  touch "$2/bin/python"
  chmod +x "$2/bin/python"
fi
''')
        self.tool("swift", '''#!/bin/sh
mkdir -p "$CAPTURE_BIN"
printf '%s' "$BUILD_REVISION" > "$CAPTURE_BIN/AutarchCapture"
case "$*" in *--show-bin-path*) printf '%s\\n' "$CAPTURE_BIN";; esac
''')
        self.tool("git", '''#!/bin/sh
case "$*" in
  *symbolic-ref*) echo main;;
  *status*) ;;
  *archive*) tar -cf - -C "$2" scripts config .claude-plugin/plugin.json docs/canon/reasoning-routing.md;;
  *) printf '%s\\n' "$BUILD_REVISION";;
esac
''')

        def build():
            return subprocess.run(["bash", str(repo / "scripts/build-review-pilot.sh")],
                                  env=self.env, capture_output=True, text=True)

        first = build()
        self.assertEqual(first.returncode, 0, first.stderr)
        sources = json.loads((repo / "build/review-pilot-sources.json").read_text())
        self.assertEqual(sources["sources"]["clavain"]["path"], str(clavain.resolve()))
        self.assertEqual(sources["sources"]["lattice"]["commit"], "first")
        self.assertEqual(set(sources["binaries"]), {"build/autarch", "build/clavain-cli"})
        packaged = repo / "build/clavain"
        self.assertEqual((packaged / ".claude-plugin/plugin.json").read_bytes(),
                         (clavain / ".claude-plugin/plugin.json").read_bytes())
        self.assertIn("build/clavain/.claude-plugin/plugin.json", sources["runtime_files"])
        self.assertEqual(sources["signed_bundle_receipt"], "review-pilot-signing.json")
        receipts = list((repo / "build/signing").glob("*/signing.json"))
        self.assertEqual(len(receipts), 1)
        first_receipt, first_bytes = receipts[0], receipts[0].read_bytes()
        current = repo / "build/review-pilot-signing.json"
        self.assertEqual(current.read_bytes(), first_bytes)
        (packaged / "scripts/deleted-script.sh").write_text("stale build output")
        self.env["BUILD_REVISION"] = "second"
        second = build()
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertFalse((packaged / "scripts/deleted-script.sh").exists())
        self.assertEqual(first_receipt.read_bytes(), first_bytes)
        receipts = list((repo / "build/signing").glob("*/signing.json"))
        self.assertEqual(len(receipts), 2)
        second_receipt = next(path for path in receipts if path != first_receipt)
        self.assertEqual(current.read_bytes(), second_receipt.read_bytes())
        self.assertNotEqual(json.loads(first_bytes)["files"],
                            json.loads(current.read_bytes())["files"])
        second_bytes = second_receipt.read_bytes()
        self.env["SIGN_EXIT"] = "1"
        failed = build()
        self.assertNotEqual(failed.returncode, 0)
        self.assertFalse(current.exists())
        self.assertEqual(first_receipt.read_bytes(), first_bytes)
        self.assertEqual(second_receipt.read_bytes(), second_bytes)


if __name__ == "__main__":
    unittest.main()
