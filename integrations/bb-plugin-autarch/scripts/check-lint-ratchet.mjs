#!/usr/bin/env node
// Shrink-only ledger for lint escapes in the Home plugin: `as unknown as`,
// `as any`, lint-disable comments and oxlint warnings, counted per file. A
// change that raises any count, or adds a file that carries one, fails. A change
// that lowers one must regenerate the baseline with --write so the win shows in
// the diff. --write refuses to raise a count.
//
//   node scripts/check-lint-ratchet.mjs           check live counts against lint-lock.json (exact)
//   node scripts/check-lint-ratchet.mjs --write   regenerate the baseline (refuses to raise; creates it when absent)
//   node scripts/check-lint-ratchet.mjs --list    print the live counts
//
// LINT_RATCHET_ROOT=<dir> points the CLI at another tree (tests use it).
import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

export const KINDS = ["as_unknown_as", "as_any", "lint_disable", "oxlint_warning"];
const SKIP = new Set(["node_modules", "dist", ".git"]);
const SOURCE = /\.(ts|tsx|mts|mjs)$/;

export function countEscapes(text) {
  return {
    as_unknown_as: (text.match(/\bas\s+unknown\s+as\b/g) ?? []).length,
    as_any: (text.match(/\bas\s+any\b/g) ?? []).length,
    lint_disable: (text.match(/\b(?:eslint|oxlint)-disable/g) ?? []).length,
  };
}

export function scanTree(root) {
  const out = {};
  const walk = (dir) => {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      if (SKIP.has(e.name) || e.name.startsWith(".")) continue;
      const p = join(dir, e.name);
      if (e.isDirectory()) walk(p);
      else if (e.isFile() && SOURCE.test(e.name) && e.name !== "check-lint-ratchet.mjs") {
        const c = countEscapes(readFileSync(p, "utf8"));
        if (c.as_unknown_as || c.as_any || c.lint_disable) out[relative(root, p).split(sep).join("/")] = c;
      }
    }
  };
  walk(root);
  return out;
}

export function mergeWarnings(counts, perFile) {
  const out = JSON.parse(JSON.stringify(counts));
  for (const [file, n] of Object.entries(perFile)) {
    out[file] ??= { as_unknown_as: 0, as_any: 0, lint_disable: 0 };
    out[file].oxlint_warning = n;
  }
  return out;
}

function n(entry, kind) {
  return entry?.[kind] ?? 0;
}

// diff returns what rose and what fell between the baseline and the live counts.
export function diff(base, live) {
  const rose = [];
  const fell = [];
  for (const file of new Set([...Object.keys(base), ...Object.keys(live)])) {
    for (const kind of KINDS) {
      const b = n(base[file], kind);
      const l = n(live[file], kind);
      if (l > b) rose.push(`${file}: ${kind} ${b} -> ${l}`);
      else if (l < b) fell.push(`${file}: ${kind} ${b} -> ${l}`);
    }
  }
  return { rose, fell };
}

export function normalize(counts) {
  const out = {};
  for (const file of Object.keys(counts).sort()) {
    const e = {};
    for (const kind of KINDS) if (n(counts[file], kind)) e[kind] = n(counts[file], kind);
    if (Object.keys(e).length) out[file] = e;
  }
  return out;
}

function oxlintWarnings(root) {
  const bin = join(root, "node_modules", ".bin", "oxlint");
  let raw;
  try {
    raw = execFileSync(bin, ["-c", ".oxlintrc.json", "-f", "json", "."], { cwd: root, encoding: "utf8", maxBuffer: 64 << 20 });
  } catch (e) {
    if (e.stdout) raw = e.stdout;
    else throw e;
  }
  const d = JSON.parse(raw);
  const errors = d.diagnostics.filter((x) => x.severity === "error");
  const per = {};
  for (const x of d.diagnostics) if (x.severity === "warning") per[x.filename] = (per[x.filename] ?? 0) + 1;
  return { per, errors };
}

export function main(argv = process.argv.slice(2), env = process.env) {
  const root = env.LINT_RATCHET_ROOT ?? join(dirname(fileURLToPath(import.meta.url)), "..");
  const lock = join(root, "lint-lock.json");
  const noOxlint = env.LINT_RATCHET_NO_OXLINT === "1";
  let live = scanTree(root);
  if (!noOxlint) {
    const { per, errors } = oxlintWarnings(root);
    if (errors.length) {
      console.error(`oxlint reports ${errors.length} error(s); run: npx oxlint -c .oxlintrc.json .`);
      return 1;
    }
    live = mergeWarnings(live, per);
  }
  live = normalize(live);
  if (argv.includes("--list")) {
    console.log(JSON.stringify(live, null, 2));
    return 0;
  }
  const base = existsSync(lock) ? JSON.parse(readFileSync(lock, "utf8")) : {};
  const { rose, fell } = diff(base, live);
  if (argv.includes("--write")) {
    if (rose.length && existsSync(lock)) {
      console.error("refusing to raise the ledger:\n  " + rose.join("\n  "));
      return 1;
    }
    writeFileSync(lock, JSON.stringify(live, null, 2) + "\n");
    console.log(`wrote ${lock}`);
    return 0;
  }
  if (rose.length) {
    console.error("lint escapes rose (remove them, do not raise the ledger):\n  " + rose.join("\n  "));
    return 1;
  }
  if (fell.length) {
    console.error("lint escapes fell; regenerate the ledger with `node scripts/check-lint-ratchet.mjs --write`:\n  " + fell.join("\n  "));
    return 1;
  }
  console.log("lint ratchet: ok");
  return 0;
}

if (import.meta.url === `file://${process.argv[1]}`) process.exit(main());
