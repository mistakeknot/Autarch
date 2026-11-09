// Backups, exports and restore-data (plan 2.3, 2.7). The backup format is the legacy one: a 0700 directory holding
// each data file, previous-source.txt (the previous PLUGIN directory) and a manifest.
import { chmodSync, copyFileSync, cpSync, existsSync, mkdirSync, readFileSync, readdirSync, renameSync, rmSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { childEnv } from "./bb.mjs";
import { sleepSync, pollUntil } from "./health.mjs";
import { template, templateArgv } from "./pluginfile.mjs";
import { ToolError, nowIso, refuse, run, sha256, utcStamp, writeFileAtomic } from "./util.mjs";

export const BACKUP_FORMAT = "bb-plugin-deploy/backup@1";

const safePath = (p) => {
  if (p.includes("'") || p.includes("\n") || p.includes("\0")) throw refuse(`path ${JSON.stringify(p)} contains a character the sqlite shell cannot take safely`);
  return p;
};
const sqlite = (db, sql, { readonly = false } = {}) => {
  const r = run(["sqlite3", ...(readonly ? ["-readonly"] : []), db, sql], { timeoutMs: 120_000 });
  if (r.error && r.error.code === "ENOENT") throw new ToolError("sqlite3 is required and was not found");
  return r;
};

/** The schema version of a sqlite file: schemaQuery if given, else schema_meta.schema_version if the table exists, else user_version. */
export function schemaVersion(db, schemaQuery) {
  let q = schemaQuery;
  if (!q) {
    const has = sqlite(db, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_meta'", { readonly: true });
    q = has.code === 0 && has.stdout.trim() === "1" ? "SELECT value FROM schema_meta WHERE key='schema_version'" : "PRAGMA user_version";
  }
  const r = sqlite(db, q, { readonly: true });
  if (r.code !== 0) throw new ToolError(`cannot read the schema version of ${db}: ${r.stderr.trim().slice(0, 200)}`);
  return r.stdout.trim();
}

export function integrityOk(db) {
  const r = sqlite(db, "PRAGMA integrity_check", { readonly: true });
  return r.code === 0 && r.stdout.trim() === "ok";
}

/**
 * Walks a data directory and calls `onFile(rel)` for each regular file. A filename is bytes, not text: decoding it as UTF-8 maps
 * every invalid byte to U+FFFD, so the raw name "x\xff" and the valid name "x\uFFFD" would both read as "x\uFFFD", be opened through
 * the same path and be hashed as one file. Any entry whose decoded name holds U+FFFD is refused before it is used (the same rule
 * the closure walk applies).
 */
function walkDataDir(root, onFile) {
  const walk = (rel) => {
    for (const d of readdirSync(join(root, rel), { withFileTypes: true })) {
      const r = rel ? `${rel}/${d.name}` : d.name;
      if (d.name.includes("\uFFFD")) throw refuse(`data path ${JSON.stringify(r)} under ${root} is not valid UTF-8 (or holds U+FFFD); a name that cannot be read back unambiguously is not backed up`);
      if (d.isDirectory()) walk(r);
      else if (d.isFile()) onFile(r);
    }
  };
  walk("");
}

function manifestOfDir(root) {
  const files = [];
  walkDataDir(root, (r) => files.push({ path: r, sha256: sha256(readFileSync(join(root, r))) }));
  return files.sort((a, b) => (a.path < b.path ? -1 : 1));
}

const onceFile = (stateDir, name, commit) => join(stateDir, "once-per-commit", `${name}-${commit}.json`);

/**
 * Back up every data entry, run the exports, record previous-source.txt, write the manifest.
 * Any failure throws before anything was installed.
 */
export function makeBackup({ plugin, vars, backupsRoot, stateDir, commit, previousPluginDir, serverUrl, lockFd, env = process.env, log = () => {} }) {
  mkdirSync(backupsRoot, { recursive: true, mode: 0o700 });
  const base = env.BPD_UTC_STAMP && /^\d{8}T\d{6}Z$/.test(env.BPD_UTC_STAMP) ? env.BPD_UTC_STAMP : utcStamp();
  let dir = join(backupsRoot, base);
  for (let i = 1; existsSync(dir); i++) dir = join(backupsRoot, `${base}-${i}`);
  mkdirSync(dir, { mode: 0o700 });
  chmodSync(dir, 0o700);
  const manifest = { format: BACKUP_FORMAT, pluginId: plugin.pluginId, createdAt: nowIso(), commit, previousSource: previousPluginDir, data: [], exports: [], warnings: [] };
  const pointers = [];
  for (const e of plugin.data) {
    const src = template(e.path, vars);
    if (e.backup === "once-per-commit") {
      const f = onceFile(stateDir, e.name, commit);
      if (existsSync(f)) {
        try {
          const ref = JSON.parse(readFileSync(f, "utf8"));
          // Reuse only a backup that finished and holds this entry: a valid manifest with the matching non-skipped entry and its digest.
          if (ref.dir && backupHolds(ref.dir, plugin.pluginId, e)) {
            manifest.data.push({ name: e.name, kind: e.kind, src, skipped: "once-per-commit", ref: ref.dir });
            continue;
          }
        } catch {
          /* take a new one */
        }
      }
    }
    if (!existsSync(src)) throw new ToolError(`data ${e.name}: ${src} does not exist; nothing was changed`);
    const dst = join(dir, e.name);
    const rec = { name: e.name, kind: e.kind, src, dst };
    if (e.kind === "sqlite") {
      const r = sqlite(safePath(src), `.backup '${safePath(dst)}'`, { readonly: true });
      if (r.code !== 0) throw new ToolError(`data ${e.name}: backup failed; nothing was changed: ${r.stderr.trim().slice(0, 200)}`);
      if (e.integrity !== false && !integrityOk(dst)) throw new ToolError(`data ${e.name}: backup failed its integrity check; nothing was changed`);
      const live = schemaVersion(src, e.schemaQuery);
      const copy = schemaVersion(dst, e.schemaQuery);
      if (live !== copy) throw new ToolError(`data ${e.name}: backup schema ${copy} differs from live ${live}; nothing was changed`);
      rec.schema = live;
      rec.sha256 = sha256(readFileSync(dst));
    } else if (e.kind === "file") {
      copyFileSync(src, dst);
      rec.sha256 = sha256(readFileSync(dst));
      if (rec.sha256 !== sha256(readFileSync(src))) throw new ToolError(`data ${e.name}: copy differs from source; nothing was changed`);
    } else {
      walkDataDir(src, () => {});
      cpSync(src, dst, { recursive: true, errorOnExist: true, verbatimSymlinks: true });
      rec.files = manifestOfDir(dst);
    }
    manifest.data.push(rec);
    if (e.backup === "once-per-commit") pointers.push(onceFile(stateDir, e.name, commit));
  }
  for (const x of plugin.export) {
    const argv = templateArgv(x.run, vars);
    log(`export ${x.name}: ${argv.join(" ")}`);
    const r = run(argv, { cwd: vars.installedRoot ?? stateDir, env: childEnv({ serverUrl, lockFd }), fds: lockFd ? [lockFd] : [], timeoutMs: 5 * 60_000 });
    if (r.code !== 0) {
      if (x.required) throw new ToolError(`export ${x.name} failed (exit ${r.code}); nothing was changed: ${(r.stderr || r.stdout).trim().slice(0, 200)}`);
      manifest.warnings.push(`export ${x.name} failed (exit ${r.code})`);
      continue;
    }
    const out = join(dir, `export-${x.name}.json`);
    writeFileAtomic(out, r.stdout);
    manifest.exports.push({ name: x.name, file: out, sha256: sha256(Buffer.from(r.stdout)) });
  }
  writeFileAtomic(join(dir, "previous-source.txt"), `${previousPluginDir}\n`);
  writeFileAtomic(join(dir, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
  // A once-per-commit pointer is published only now that the exports and manifest.json are complete.
  for (const f of pointers) writeFileAtomic(f, JSON.stringify({ dir, at: nowIso() }) + "\n");
  return { dir, manifest };
}

/**
 * True when `dir` is a finished backup of this plugin whose manifest holds the matching, non-skipped entry for `e`
 * and the copy on disk still matches the recorded digest (a directory entry is checked by its recorded file list).
 */
function backupHolds(dir, pluginId, e) {
  try {
    const m = loadManifest(dir, pluginId);
    const rec = m.data.find((d) => d && d.name === e.name && !d.skipped);
    if (!rec || rec.kind !== e.kind) return false;
    const copy = join(dir, e.name);
    if (!existsSync(copy)) return false;
    if (e.kind === "sqlite" || e.kind === "file") return typeof rec.sha256 === "string" && sha256(readFileSync(copy)) === rec.sha256;
    if (!Array.isArray(rec.files)) return false;
    const now = manifestOfDir(copy);
    return now.length === rec.files.length && rec.files.every((f) => now.some((n) => n.path === f.path && n.sha256 === f.sha256));
  } catch {
    return false;
  }
}

/** Reads and validates a backup directory's manifest (format, plugin id). */
function loadManifest(dir, pluginId) {
  const f = join(dir, "manifest.json");
  if (!existsSync(f)) throw refuse(`${f} is missing: this is not a deploy backup`);
  let m;
  try {
    m = JSON.parse(readFileSync(f, "utf8"));
  } catch {
    throw refuse(`${f} is not valid JSON`);
  }
  if (!m || typeof m !== "object" || m.format !== BACKUP_FORMAT) throw refuse(`${f} is not a ${BACKUP_FORMAT} manifest`);
  if (m.pluginId !== pluginId) throw refuse(`backup belongs to plugin ${String(m.pluginId)}, not ${pluginId}`);
  if (!Array.isArray(m.data) || typeof m.previousSource !== "string") throw refuse(`${f} is malformed`);
  return m;
}

/**
 * Port of home-ui.sh `rollback --restore-data`: backup integrity, require fuser, disable, wait, no holder on the
 * database or its -wal, restore from the backup, drop -wal/-shm, reinstall the previous source. `check` verifies only.
 */
export function restoreData({ plugin, vars, backupDir, check, bb, log = () => {} }) {
  if (!existsSync(backupDir) || !statSync(backupDir).isDirectory()) throw refuse(`backup directory ${backupDir} does not exist`);
  const sqliteEntries = plugin.data.filter((e) => e.kind === "sqlite");
  if (sqliteEntries.length === 0) throw refuse("the plugin file declares no sqlite data to restore");
  const manifest = loadManifest(backupDir, plugin.pluginId);
  const prevFile = join(backupDir, "previous-source.txt");
  if (!existsSync(prevFile)) throw refuse(`${prevFile} is missing: cannot reinstall the previous version`);
  const prev = readFileSync(prevFile, "utf8").trim();
  if (prev !== manifest.previousSource) throw refuse(`previous-source.txt disagrees with the backup manifest; refusing`);
  if (!prev.startsWith("/") || !existsSync(prev)) throw refuse(`previous source ${prev} does not exist`);
  const plan = [];
  for (const e of sqliteEntries) {
    const m = manifest.data.find((d) => d && d.name === e.name);
    if (!m) throw refuse(`backup manifest has no entry for ${e.name}`);
    // A once-per-commit skip points at the earlier backup that holds the file; that backup must itself be valid.
    let holderDir = backupDir;
    let holder = m;
    if (m.skipped) {
      if (typeof m.ref !== "string" || dirname(m.ref) !== dirname(backupDir)) throw refuse(`${e.name}: the referenced earlier backup is not beside this one`);
      const refManifest = loadManifest(m.ref, plugin.pluginId);
      holder = refManifest.data.find((d) => d && d.name === e.name && !d.skipped);
      if (!holder) throw refuse(`${e.name}: the referenced earlier backup has no copy`);
      holderDir = m.ref;
    }
    const bk = join(holderDir, e.name);
    if (!existsSync(bk)) throw refuse(`backup has no ${e.name}`);
    if (typeof holder.sha256 !== "string" || sha256(readFileSync(bk)) !== holder.sha256) throw refuse(`backup ${bk} does not match its manifest digest; nothing was changed`);
    if (!integrityOk(bk)) throw refuse(`backup ${bk} fails its integrity check; nothing was changed`);
    plan.push({ name: e.name, backup: bk, db: template(e.path, vars) });
  }
  const fuser = run(["fuser", "-V"]);
  if (fuser.error && fuser.error.code === "ENOENT") throw refuse("fuser (psmisc) is required to confirm nothing holds the database");
  if (check) return { ok: true, check: true, plan, previous: prev };
  log("restore-data: disabling the plugin");
  bb.disable();
  sleepSync(1000);
  for (const p of plan) {
    for (const f of [p.db, `${p.db}-wal`]) {
      if (!existsSync(f)) continue;
      const h = run(["fuser", f]);
      if (h.code === 0) throw new ToolError(`something still holds ${f} (pids: ${h.stdout.trim() || h.stderr.trim()}); the plugin stays disabled`);
    }
    const r = sqlite(safePath(p.backup), `.backup '${safePath(`${p.db}.restore`)}'`);
    if (r.code !== 0) throw new ToolError(`data restore failed; the plugin stays disabled: ${r.stderr.trim().slice(0, 200)}`);
    rmSync(`${p.db}-wal`, { force: true });
    rmSync(`${p.db}-shm`, { force: true });
    renameSync(`${p.db}.restore`, p.db);
  }
  bb.install(prev);
  bb.enable();
  const up = pollUntil(() => bb.isRunning(), 60);
  if (!up) throw new ToolError("data restored and the previous version installed, but the plugin is not running");
  return { ok: true, check: false, plan, previous: prev };
}
