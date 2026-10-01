// Shared TypeScript-compiler-API scans for the Task 2.7 static checks. These look at the syntax tree,
// not at text, so a regexp `.exec()` or a SQLite `.exec()` is never mistaken for process execution.
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import ts from "typescript";

export const ROOT = join(import.meta.dirname, "..");
const SKIP_DIRS = new Set(["ui", "__tests__", "e2e", "scripts", "node_modules", "dist", ".v2-a9853e2"]);

/** Every server source: all .ts/.tsx outside ui/, __tests__/, e2e/ and scripts/. */
export function serverSources(): string[] {
  const out: string[] = [];
  const walk = (dir: string) => {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      if (e.isDirectory()) {
        if (!SKIP_DIRS.has(e.name) && !e.name.startsWith(".")) walk(join(dir, e.name));
      } else if (/\.(ts|tsx)$/.test(e.name) && !e.name.endsWith(".d.ts") && !/\.config\.ts$/.test(e.name)) out.push(join(dir, e.name));
    }
  };
  walk(ROOT);
  return out.sort();
}

export const rel = (file: string) => relative(ROOT, file);
export const parse = (text: string, name = "fixture.ts") => ts.createSourceFile(name, text, ts.ScriptTarget.ES2022, true, name.endsWith("x") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
export const parseFile = (file: string) => parse(readFileSync(file, "utf8"), file);

function visit(node: ts.Node, fn: (n: ts.Node) => void) {
  fn(node);
  ts.forEachChild(node, (c) => visit(c, fn));
}
const lit = (n: ts.Node | undefined): string | undefined => (n && (ts.isStringLiteral(n) || ts.isNoSubstitutionTemplateLiteral(n)) ? n.text : undefined);
const where = (sf: ts.SourceFile, n: ts.Node) => `${sf.fileName}:${sf.getLineAndCharacterOfPosition(n.getStart()).line + 1}`;

const BANNED_MODULES = new Set(["child_process", "node:child_process", "execa", "worker_threads", "node:worker_threads", "cluster", "node:cluster"]);

/** Imports, requires and dynamic imports of process-spawning modules, plus Bun.spawn and process.binding. */
export function execViolations(sf: ts.SourceFile): string[] {
  const bad: string[] = [];
  visit(sf, (n) => {
    if ((ts.isImportDeclaration(n) || ts.isExportDeclaration(n)) && n.moduleSpecifier) {
      const m = lit(n.moduleSpecifier);
      const typeOnly = ts.isImportDeclaration(n) && n.importClause?.isTypeOnly === true; // erased at compile time
      if (m !== undefined && !typeOnly && BANNED_MODULES.has(m)) bad.push(`${where(sf, n)} import of ${m}`);
    }
    if (ts.isImportEqualsDeclaration(n) && ts.isExternalModuleReference(n.moduleReference)) {
      const m = lit(n.moduleReference.expression);
      if (m !== undefined && BANNED_MODULES.has(m)) bad.push(`${where(sf, n)} require of ${m}`);
    }
    if (ts.isCallExpression(n)) {
      const isRequire = ts.isIdentifier(n.expression) && n.expression.text === "require";
      const isImport = n.expression.kind === ts.SyntaxKind.ImportKeyword;
      if (isRequire || isImport) {
        const m = lit(n.arguments[0]);
        if (m === undefined) bad.push(`${where(sf, n)} ${isImport ? "dynamic import" : "require"} of a non-literal specifier`);
        else if (BANNED_MODULES.has(m)) bad.push(`${where(sf, n)} ${isImport ? "dynamic import" : "require"} of ${m}`);
      }
    }
    // Bun.spawn, Bun.spawnSync, process.binding, by property or element access.
    const access = (obj: ts.Expression, name: string | undefined) => {
      if (!ts.isIdentifier(obj)) return;
      if (obj.text === "Bun" && name !== undefined && /^spawn/.test(name)) bad.push(`${where(sf, n)} Bun.${name}`);
      if (obj.text === "process" && name === "binding") bad.push(`${where(sf, n)} process.binding`);
    };
    if (ts.isPropertyAccessExpression(n)) access(n.expression, n.name.text);
    if (ts.isElementAccessExpression(n)) access(n.expression, lit(n.argumentExpression));
  });
  return bad;
}

const FS_MODULES = new Set(["fs", "node:fs", "fs/promises", "node:fs/promises"]);
export const FS_WRITE_APIS = new Set([
  "writeFile", "writeFileSync", "appendFile", "appendFileSync", "createWriteStream", "write", "writeSync", "writev", "writevSync",
  "rename", "renameSync", "unlink", "unlinkSync", "rm", "rmSync", "rmdir", "rmdirSync", "mkdir", "mkdirSync", "mkdtemp", "mkdtempSync",
  "copyFile", "copyFileSync", "cp", "cpSync", "truncate", "truncateSync", "ftruncate", "ftruncateSync", "symlink", "symlinkSync", "link", "linkSync",
  "chmod", "chmodSync", "fchmod", "fchmodSync", "chown", "chownSync", "fchown", "fchownSync", "utimes", "utimesSync", "futimes", "futimesSync",
  "lchmod", "lchown", "fsync", "fsyncSync", "fdatasync", "fdatasyncSync", "open", "openSync",
]);
const OPENERS = new Set(["open", "openSync"]);
const FD_ONLY = new Set(["fsync", "fsyncSync"]);

export interface FsUse {
  api: string;
  at: string;
  /** For open/openSync: the flags argument as written, or undefined when absent. */
  flags?: string;
}

/** Every use of an fs write-capable API (imported by name, via a namespace, or via require). */
export function fsWriteUses(sf: ts.SourceFile): FsUse[] {
  const names = new Map<string, string>(); // local identifier -> api name
  const namespaces = new Set<string>();
  const uses: FsUse[] = [];
  visit(sf, (n) => {
    if (ts.isImportDeclaration(n) && FS_MODULES.has(lit(n.moduleSpecifier) ?? "")) {
      const b = n.importClause?.namedBindings;
      if (b && ts.isNamedImports(b)) for (const el of b.elements) names.set(el.name.text, (el.propertyName ?? el.name).text);
      if (b && ts.isNamespaceImport(b)) namespaces.add(b.name.text);
      if (n.importClause?.name) namespaces.add(n.importClause.name.text);
    }
  });
  const flagsOf = (call: ts.CallExpression) => (call.arguments[1] ? call.arguments[1].getText(sf) : undefined);
  visit(sf, (n) => {
    if (ts.isCallExpression(n)) {
      let api: string | undefined;
      if (ts.isIdentifier(n.expression) && names.has(n.expression.text)) api = names.get(n.expression.text);
      if (ts.isPropertyAccessExpression(n.expression) && ts.isIdentifier(n.expression.expression) && namespaces.has(n.expression.expression.text)) api = n.expression.name.text;
      if (ts.isPropertyAccessExpression(n.expression) && ts.isPropertyAccessExpression(n.expression.expression) && n.expression.expression.name.text === "promises") api = n.expression.name.text;
      if (api !== undefined && FS_WRITE_APIS.has(api)) uses.push({ api, at: where(sf, n), ...(OPENERS.has(api) ? { flags: flagsOf(n) } : {}) });
    }
  });
  // An imported write API that is only referenced (passed as a value) still counts.
  visit(sf, (n) => {
    if (ts.isIdentifier(n) && names.has(n.text) && FS_WRITE_APIS.has(names.get(n.text)!)) {
      const p = n.parent;
      const isCallee = ts.isCallExpression(p) && p.expression === n;
      const isImport = ts.isImportSpecifier(p);
      if (!isCallee && !isImport) uses.push({ api: names.get(n.text)!, at: where(sf, n) });
    }
  });
  return uses;
}

export const isFdOnly = (api: string) => FD_ONLY.has(api);
export const isOpener = (api: string) => OPENERS.has(api);

/** True when an open() flags expression can only read: "r", or O_RDONLY with no write, create or truncate bit. */
export function readOnlyFlags(flags: string | undefined): boolean {
  if (flags === undefined) return true; // open(path) alone is read-only for fs.open
  if (/^["'`]r["'`]$/.test(flags.trim())) return true;
  return /\bO_RDONLY\b/.test(flags) && !/\bO_(WRONLY|RDWR|CREAT|TRUNC|APPEND|EXCL)\b/.test(flags);
}

/** Names that exist only as test seams (MigrateOptions.test, BackupTestSeams, StoreOptions.hook). */
export const SEAM_NAMES = new Set(["test", "skipHold", "hook"]);

/** A value that is only relayed from the caller's own options (`opts.test`, `opts?.hook ?? ...`) is plumbing, not a seam being passed. */
const relayed = (n: ts.Node | undefined, sf: ts.SourceFile) => n !== undefined && /^\(?\s*opts\??\./.test(n.getText(sf));

/** Object-literal properties with a seam name, and `.test =` style assignments, unless the value is relayed from `opts`. */
export function seamViolations(sf: ts.SourceFile): string[] {
  const bad: string[] = [];
  visit(sf, (n) => {
    if ((ts.isPropertyAssignment(n) || ts.isShorthandPropertyAssignment(n)) && ts.isObjectLiteralExpression(n.parent)) {
      const nm = ts.isIdentifier(n.name) ? n.name.text : lit(n.name);
      if (nm !== undefined && SEAM_NAMES.has(nm) && !(ts.isPropertyAssignment(n) && relayed(n.initializer, sf))) bad.push(`${where(sf, n)} object property "${nm}"`);
    }
    if (ts.isBinaryExpression(n) && n.operatorToken.kind === ts.SyntaxKind.EqualsToken && ts.isPropertyAccessExpression(n.left) && SEAM_NAMES.has(n.left.name.text) && !relayed(n.right, sf)) {
      bad.push(`${where(sf, n)} assignment to .${n.left.name.text}`);
    }
  });
  return bad;
}

/** Every harness source under e2e/ (.ts and .mjs). */
export function e2eSources(): string[] {
  const out: string[] = [];
  const walk = (dir: string) => {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      if (e.isDirectory()) walk(join(dir, e.name));
      else if (/\.(ts|mjs)$/.test(e.name) && !e.name.endsWith(".d.ts")) out.push(join(dir, e.name));
    }
  };
  walk(join(ROOT, "e2e"));
  return out.sort();
}

const HTTP_CLIENT_MODULES = new Set(["undici", "node-fetch", "axios", "got"]);

/** Task 2.11: beyond process execution, an e2e source may not call fetch or callRpc or import an HTTP client. */
export function rigViolations(sf: ts.SourceFile): string[] {
  const bad = execViolations(sf);
  visit(sf, (n) => {
    if ((ts.isImportDeclaration(n) || ts.isExportDeclaration(n)) && n.moduleSpecifier) {
      const m = lit(n.moduleSpecifier);
      if (m !== undefined && HTTP_CLIENT_MODULES.has(m)) bad.push(`${where(sf, n)} import of ${m}`);
    }
    if (ts.isCallExpression(n)) {
      const f = n.expression;
      if (ts.isIdentifier(f) && (f.text === "fetch" || f.text === "callRpc")) bad.push(`${where(sf, n)} call of ${f.text}`);
      if (ts.isPropertyAccessExpression(f) && (f.name.text === "fetch" || f.name.text === "callRpc")) bad.push(`${where(sf, n)} call of .${f.name.text}`);
      if (ts.isElementAccessExpression(f)) {
        const k = lit(f.argumentExpression);
        if (k === "fetch" || k === "callRpc") bad.push(`${where(sf, n)} call of ["${k}"]`);
      }
    }
  });
  return bad;
}
