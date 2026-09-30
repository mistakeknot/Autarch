#!/usr/bin/env node
// Exports HEAD with `git archive` into a scratch directory, stamps the plugin
// with identity.json {source_sha256}, installs its dependencies, builds
// `autarch` from that export, and writes the build file:
//   {commit, plugin_dir, source_sha256, autarch_path, autarch_sha256}
// usage: build-identity.mjs --out <file> [--repo <dir>] [--scratch <dir>] [--skip-npm-ci]
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { sourceSha256 } from "./source-hash.mjs";

const PLUGIN_REL = "integrations/bb-plugin-autarch";

export function buildIdentity({ repo, out, scratch, skipNpmCi = false }) {
  const git = (...a) => execFileSync("git", ["-C", repo, ...a], { encoding: "utf8" }).trim();
  const top = git("rev-parse", "--show-toplevel");
  const commit = git("rev-parse", "HEAD");
  const root = scratch ? resolve(scratch) : mkdtempSync(join(tmpdir(), "autarch-build-"));
  const src = join(root, "src");
  mkdirSync(src, { recursive: true });
  const tar = execFileSync("git", ["-C", top, "archive", "--format=tar", "HEAD"], { maxBuffer: 1 << 30 });
  execFileSync("tar", ["-x", "-C", src], { input: tar });
  const pluginDir = join(src, PLUGIN_REL);
  const source = sourceSha256(pluginDir);
  writeFileSync(join(pluginDir, "identity.json"), JSON.stringify({ commit, source_sha256: source }) + "\n");
  if (!skipNpmCi) execFileSync("npm", ["ci", "--no-audit", "--no-fund"], { cwd: pluginDir, stdio: ["ignore", "ignore", "inherit"] });
  const bin = join(root, "bin");
  mkdirSync(bin, { recursive: true });
  const autarch = join(bin, "autarch");
  execFileSync("go", ["build", "-buildvcs=false", "-o", autarch, "./cmd/autarch"], { cwd: src, stdio: ["ignore", "ignore", "inherit"] });
  const autarchSha = createHash("sha256").update(readFileSync(autarch)).digest("hex");
  const build = { commit, plugin_dir: pluginDir, source_sha256: source, autarch_path: autarch, autarch_sha256: autarchSha };
  mkdirSync(dirname(resolve(out)), { recursive: true });
  writeFileSync(out, JSON.stringify(build, null, 2) + "\n");
  return build;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const a = { out: null, repo: process.cwd(), scratch: null, skipNpmCi: false };
  const argv = process.argv.slice(2);
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--out") a.out = argv[++i];
    else if (argv[i] === "--repo") a.repo = argv[++i];
    else if (argv[i] === "--scratch") a.scratch = argv[++i];
    else if (argv[i] === "--skip-npm-ci") a.skipNpmCi = true;
    else {
      console.error(`build-identity: unexpected argument ${argv[i]}`);
      process.exit(2);
    }
  }
  if (!a.out) {
    console.error("build-identity: --out <file> is required");
    process.exit(2);
  }
  const b = buildIdentity(a);
  console.log(`build-identity: ${b.commit} plugin ${b.source_sha256.slice(0, 12)} autarch ${b.autarch_sha256.slice(0, 12)} -> ${a.out}`);
}
