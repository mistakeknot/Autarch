#!/usr/bin/env node
// bb-plugin-deploy: one deploy tool for bb plugins (see README.md). Node built-ins only; external programs are
// git, bb, sqlite3, flock and npm, always spawned with argv arrays and no shell.
import { realpathSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { check, deploy, preview, prepare, publishAll, restoreCommand, rollback, tryPublish } from "./lib/deploy.mjs";
import { gcBuilds } from "./lib/gc.mjs";
import { gitTry, verifyClosure, verifyClosureAgainstHead } from "./lib/gitops.mjs";
import { loadPluginFile } from "./lib/pluginfile.mjs";
import { readResultFile } from "./lib/publish.mjs";
import { closeSession, openSession } from "./lib/session.mjs";
import { updateTick } from "./lib/tick.mjs";
import { EXIT, PROTOCOL, ToolError } from "./lib/util.mjs";

process.umask(0o077);

const cliPath = realpathSync(fileURLToPath(import.meta.url));
const engineRoot = resolve(dirname(cliPath), "..", "..");

const VALUE_FLAGS = new Set(["plugin-file", "bb-data", "state-dir", "host-file", "commit", "class", "mode", "expect-tree", "confirmed-first-run", "to", "build", "backup", "path"]);
const BOOL_FLAGS = new Set(["json", "check", "apply", "write", "reconcile", "verify-closure"]);
const COMMANDS = ["version", "validate-plugin-file", "record", "status", "check", "prepare", "deploy", "rollback", "restore-data", "preview", "update-tick", "gc"];

export function parseArgs(argv) {
  const [command, ...rest] = argv;
  const f = {};
  for (let i = 0; i < rest.length; i++) {
    const a = rest[i];
    if (!a.startsWith("--")) throw new ToolError(`unexpected argument ${a}`, EXIT.USAGE);
    const k = a.slice(2);
    if (BOOL_FLAGS.has(k)) f[k] = true;
    else if (VALUE_FLAGS.has(k)) {
      const v = rest[++i];
      if (v === undefined || v.startsWith("--")) throw new ToolError(`--${k} needs a value`, EXIT.USAGE);
      f[k] = v;
    } else throw new ToolError(`unknown flag --${k}`, EXIT.USAGE);
  }
  return { command, f };
}

function emit(f, report) {
  if (f.json) {
    process.stdout.write(`${JSON.stringify({ protocol: PROTOCOL, ...report }, null, 2)}\n`);
    return;
  }
  const out = [];
  if (report.message) out.push(report.message);
  for (const g of report.gates ?? []) out.push(`  ${g.ok ? "ok  " : "FAIL"} ${g.check}/${g.gate}${g.reason ? `: ${g.reason}` : ""}`);
  for (const w of report.warnings ?? []) out.push(`warning: ${w}`);
  if (report.ok && report.rollbackCommand) out.push("", "To roll back:", report.rollbackCommand, "", "To restore data from this deploy's backup (mk only):", report.restoreDataCommand);
  const block = report.recoveryBlock ?? (report.recovery && report.recovery.lastResort ? report.recovery : null);
  if (block) out.push("", "Last-resort recovery (if this tool cannot run):", block.lastResort);
  process.stdout.write(`${out.join("\n")}\n`);
}

const gitHead = (root) => gitTry(root, ["rev-parse", "HEAD"]);

async function main() {
  const { command, f } = parseArgs(process.argv.slice(2));
  if (!command || !COMMANDS.includes(command)) throw new ToolError(`usage: cli.mjs <${COMMANDS.join("|")}> --plugin-file PATH [options]`, EXIT.USAGE);
  if (command === "version") {
    emit(f, { ok: true, exit: 0, message: `${PROTOCOL} node ${process.version} engine ${gitHead(engineRoot) ?? "unknown"}`, protocol: PROTOCOL, node: process.version, toolCommit: gitHead(engineRoot) });
    return 0;
  }
  if (!f["plugin-file"]) throw new ToolError("--plugin-file is required", EXIT.USAGE);
  let pluginFile;
  try {
    pluginFile = realpathSync(resolve(f["plugin-file"]));
  } catch {
    throw new ToolError(`cannot read the plugin file ${f["plugin-file"]}`, EXIT.ERROR);
  }
  if (command === "validate-plugin-file") {
    const p = loadPluginFile(pluginFile);
    emit(f, { ok: true, exit: 0, message: `plugin file for ${p.pluginId} is valid` });
    return 0;
  }
  const log = f.json ? () => {} : (m) => process.stderr.write(`${m}\n`);
  const flags = { bbData: f["bb-data"], stateDir: f["state-dir"], hostFile: f["host-file"] };
  const s = openSession({ pluginFile, flags, cliPath, engineRoot, log, write: command !== "record" || !!f.reconcile });
  try {
    const o = { commit: f.commit, cls: f.class, mode: f.mode ?? "agent", expectTree: f["expect-tree"], confirmedFirstRun: f["confirmed-first-run"], build: f.build, backup: f.backup, check: !!f.check, path: f.path };
    let report;
    switch (command) {
      case "record": {
        report = { command, ok: true, exit: 0, message: `installed ${s.live.state === "unknown" ? "unknown" : s.live.commit}`, live: s.live, record: s.rec };
        if (f["verify-closure"]) {
          if (s.live.state === "stamped") verifyClosure({ root: s.live.buildRoot, commit: s.live.commit, controlPaths: s.plugin.controlPaths });
          else if (s.live.state === "git") verifyClosureAgainstHead({ root: s.live.buildRoot, controlPaths: s.plugin.controlPaths });
          else throw new ToolError(`cannot verify the closure: ${s.live.reason}`, EXIT.REFUSED);
          report.closureVerified = true;
        }
        if (f.reconcile) {
          // Reconcile also brings the published status in line: an older result must not keep a stale update button alive.
          const pub = tryPublish(s, null);
          report.status = pub;
          report.warnings = [pub.warning, s.shaRepairError ? `installed.sha could not be repaired: ${s.shaRepairError}` : null].filter(Boolean);
          if (s.shaRepairError) {
            report.ok = false;
            report.exit = 1;
            report.message = `reconcile could not repair installed.sha: ${s.shaRepairError}`;
          }
        }
        break;
      }
      case "status": {
        const pub = f.write ? publishAll(s, null) : null;
        report = { command, ok: true, exit: 0, message: `installed ${s.live.state === "unknown" ? "unknown" : s.live.commit}`, live: s.live, status: pub, result: readResultFile(s.stateDir) };
        break;
      }
      case "check":
        report = check(s, o);
        break;
      case "prepare":
        report = prepare(s, o);
        break;
      case "deploy":
        report = deploy(s, o);
        break;
      case "rollback":
        report = rollback(s, o);
        break;
      case "restore-data":
        report = restoreCommand(s, o);
        break;
      case "preview":
        report = await preview(s, o);
        break;
      case "update-tick":
        report = updateTick(s, o, readResultFile);
        break;
      case "gc": {
        const g = gcBuilds({ buildsRoot: s.dirs.buildsRoot, pluginId: s.plugin.pluginId, keepN: s.plugin.retention.builds, installedRoot: s.live.buildRoot, previousRoot: s.rec.previous?.buildRoot, apply: !!f.apply });
        report = { command, ok: true, exit: 0, message: `${g.remove.length} build(s) ${f.apply ? "removed" : "would be removed"}`, ...g };
        break;
      }
      default:
        throw new ToolError(`unhandled command ${command}`, EXIT.USAGE);
    }
    emit(f, report);
    return report.exit;
  } finally {
    closeSession(s);
  }
}

try {
  process.exitCode = await main();
} catch (e) {
  const err = e instanceof ToolError ? e : new ToolError(String(e?.stack ?? e));
  const wantJson = process.argv.includes("--json");
  if (wantJson) process.stdout.write(`${JSON.stringify({ protocol: PROTOCOL, ok: false, exit: err.code, message: err.message })}\n`);
  else process.stderr.write(`bb-plugin-deploy: ${err.message}\n`);
  process.exitCode = err.code;
}
