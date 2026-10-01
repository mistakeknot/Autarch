// The a9853e2 (schema 2) store and migrations, extracted from git into a dot-directory next to
// the tests so it resolves node_modules and is never typechecked or collected. Test-only.
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

export const V2_COMMIT = "a9853e2";
export const V2_DIR = join(import.meta.dirname, ".v2-a9853e2");
const FILES = ["store.ts", "migrations.ts"];

export function ensureV2Build(): string {
  if (FILES.every((f) => existsSync(join(V2_DIR, f)))) return V2_DIR;
  mkdirSync(V2_DIR, { recursive: true });
  for (const f of FILES) {
    const src = execFileSync("git", ["show", `${V2_COMMIT}:integrations/bb-plugin-autarch/${f}`], {
      cwd: import.meta.dirname,
      encoding: "utf8",
      maxBuffer: 16 << 20,
    });
    writeFileSync(join(V2_DIR, f), src);
  }
  return V2_DIR;
}

export async function loadV2() {
  const dir = ensureV2Build();
  const store = (await import(/* @vite-ignore */ join(dir, "store.ts"))) as typeof import("../store.js");
  const migrations = (await import(/* @vite-ignore */ join(dir, "migrations.ts"))) as typeof import("../migrations.js");
  return { Store: store.Store, MIGRATIONS: migrations.MIGRATIONS, CODE_VERSION: migrations.CODE_VERSION };
}

// The whole a9853e2 (schema 2) plugin backend, for driving its real wiring (wireHome, the `home` CLI, the
// Service) in the fence and cutover acceptance tests. Same dot-directory, so it is gitignored, never
// collected and never typechecked. Test-only. UI sources are not extracted; the backend does not import them.
const FULL = [
  "asks.ts", "catchup.ts", "cli.ts", "contract.ts", "delegation.ts", "export.ts", "feed.ts", "migrations.ts",
  "model.ts", "ruling.ts", "serve.ts", "server.ts", "service.ts", "store.ts", "wakes.ts", "scripts/source-hash.mjs",
];

export function ensureV2Full(): string {
  if (FULL.every((f) => existsSync(join(V2_DIR, f)))) return V2_DIR;
  mkdirSync(join(V2_DIR, "scripts"), { recursive: true });
  for (const f of FULL) {
    const src = execFileSync("git", ["show", `${V2_COMMIT}:integrations/bb-plugin-autarch/${f}`], {
      cwd: import.meta.dirname,
      encoding: "utf8",
      maxBuffer: 16 << 20,
    });
    writeFileSync(join(V2_DIR, f), src);
  }
  return V2_DIR;
}

export async function loadV2Home() {
  const dir = ensureV2Full();
  const server = (await import(/* @vite-ignore */ join(dir, "server.ts"))) as typeof import("../server.js");
  const store = (await import(/* @vite-ignore */ join(dir, "store.ts"))) as typeof import("../store.js");
  const migrations = (await import(/* @vite-ignore */ join(dir, "migrations.ts"))) as typeof import("../migrations.js");
  const sourceHash = (await import(/* @vite-ignore */ join(dir, "scripts/source-hash.mjs"))) as unknown;
  void sourceHash;
  return { wireHome: server.wireHome, createStoreHandle: store.createStoreHandle, Store: store.Store, SchemaTooNewError: migrations.SchemaTooNewError, CODE_VERSION: migrations.CODE_VERSION };
}
