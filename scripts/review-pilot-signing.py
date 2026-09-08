#!/usr/bin/env python3
"""Require Apple Development signing and retain verifiable local pilot evidence."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import subprocess
import sys


def identity():
    result = subprocess.run(["security", "find-identity", "-v", "-p", "codesigning"],
                            check=True, capture_output=True, text=True)
    identities = dict(re.findall(r'\)\s+([A-Fa-f0-9]{40})\s+"(Apple Development: [^"\n]+)"',
                                 result.stdout))
    configured = os.environ.get("AUTARCH_SIGNING_IDENTITY", "")
    matches = [(key, name) for key, name in identities.items()
               if not configured or configured.upper() == key.upper() or configured == name]
    if len(matches) != 1:
        raise ValueError("Exactly one available Apple Development identity is required. "
                         "Set AUTARCH_SIGNING_IDENTITY to its SHA-1 or exact name; "
                         "ad-hoc signing is never a fallback. Check keychain access if none are visible.")
    return matches[0]


def sign(app, evidence, key, name):
    if evidence.exists() or evidence.is_symlink():
        raise ValueError("Refusing to replace existing signing evidence; use a fresh build directory.")
    with (app / "Contents/Info.plist").open("rb") as stream:
        bundle_id = plistlib.load(stream)["CFBundleIdentifier"]
    if bundle_id != "org.autarch.review.capture":
        raise ValueError("Unexpected companion bundle identifier; refusing to sign.")
    # Do not preserve the old ad-hoc requirement or supply -r. Apple generates
    # the default designated requirement for this identity and bundle identifier.
    subprocess.run(["codesign", "--force", "--sign", key, str(app)], check=True)
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    requirement = subprocess.run(["codesign", "--display", "-r-", str(app)],
                                 check=True, capture_output=True, text=True)
    designated = next((line for line in (requirement.stdout + requirement.stderr).splitlines()
                       if line.startswith("designated => ")), "")
    if not designated or "anchor apple" not in designated:
        raise ValueError("Apple designated requirement missing; refusing success evidence.")
    files = {str(path.relative_to(app)): hashlib.sha256(path.read_bytes()).hexdigest()
             for path in sorted(app.rglob("*")) if path.is_file()}
    with evidence.open("x") as stream:
        json.dump({"signed_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                   "identity_sha1": key, "identity_name": name, "bundle_identifier": bundle_id,
                   "designated_requirement": designated, "files": files}, stream, indent=2)
        stream.write("\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("preflight")
    signing = commands.add_parser("sign")
    signing.add_argument("app", type=Path)
    signing.add_argument("evidence", type=Path)
    args = parser.parse_args()
    key, name = identity()
    if args.command == "preflight":
        print(key)
    else:
        sign(args.app, args.evidence, key, name)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        sys.exit(f"Pilot signing failed: {error}")
