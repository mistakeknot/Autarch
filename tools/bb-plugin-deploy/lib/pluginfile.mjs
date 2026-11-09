// Plugin file `bb-plugin-deploy/plugin@1` (plan 2.3): strict validation (unknown keys fail closed) and templating.
import { readFileSync } from "node:fs";
import { ToolError, EXIT, isObj } from "./util.mjs";

export const PLUGIN_FORMAT = "bb-plugin-deploy/plugin@1";
export const PLACEHOLDERS = Object.freeze(["bbData", "stateDir", "installedRoot", "installedPluginDir", "targetRoot", "targetPluginDir", "installedCommit", "targetCommit", "class", "buildDir"]);
const PH = new Set(PLACEHOLDERS);
const PH_RE = /\{([A-Za-z][A-Za-z0-9]*)\}/g;

const TOP_KEYS = ["format", "pluginId", "repo", "branch", "pluginDir", "build", "data", "export", "preflight", "preInstall", "preEnable", "postEnablePreConfig", "configAfterEnable", "health", "classes", "dataRestore", "previewGate", "rollbackLeaves", "controlPaths", "retention", "updateRequest"];
const HEALTH_KINDS = ["running", "log-line", "process-count", "check", "setting-equals"];
const SINCE = ["install", "enable", "config"];

/** Returns the list of problems (empty when valid). Pure: takes the parsed JSON. */
export function validatePluginObject(p) {
  const errs = [];
  const bad = (path, msg) => errs.push(`${path}: ${msg}`);
  const allow = (obj, keys, path) => {
    for (const k of Object.keys(obj)) if (!keys.includes(k)) bad(path ? `${path}.${k}` : k, "unknown key");
  };
  const str = (v, path, { re, max = 4096 } = {}) => {
    if (typeof v !== "string" || v.length === 0 || v.length > max) return bad(path, "must be a non-empty string"), false;
    if (re && !re.test(v)) return bad(path, `must match ${re}`), false;
    return true;
  };
  /** A templated string: every {placeholder} must be in the closed set. */
  const tmpl = (v, path) => {
    if (!str(v, path)) return;
    for (const m of v.matchAll(PH_RE)) if (!PH.has(m[1])) bad(path, `unknown placeholder {${m[1]}}`);
  };
  const argv = (v, path) => {
    if (!Array.isArray(v) || v.length === 0) return bad(path, "must be a non-empty argv array");
    v.forEach((a, i) => tmpl(a, `${path}[${i}]`));
  };
  const list = (v, path, fn) => {
    if (!Array.isArray(v)) return bad(path, "must be an array");
    v.forEach((e, i) => (isObj(e) ? fn(e, `${path}[${i}]`) : bad(`${path}[${i}]`, "must be an object")));
  };
  const markerName = (e, path) => {
    if (e.marker !== undefined) str(e.marker, `${path}.marker`, { re: /^[A-Za-z0-9_]{1,64}$/ });
  };
  const checkEntry = (e, path) => {
    allow(e, ["name", "run", "marker"], path);
    str(e.name, `${path}.name`, { re: /^[A-Za-z0-9_.-]{1,64}$/ });
    argv(e.run, `${path}.run`);
    markerName(e, path);
  };

  if (!isObj(p)) return ["plugin file must be a JSON object"];
  allow(p, TOP_KEYS, "");
  if (p.format !== PLUGIN_FORMAT) bad("format", `must be "${PLUGIN_FORMAT}"`);
  str(p.pluginId, "pluginId", { re: /^[a-z0-9][a-z0-9._-]{0,63}$/ });
  str(p.repo, "repo");
  str(p.branch, "branch", { re: /^[A-Za-z0-9._\/-]{1,200}$/ });
  if (str(p.pluginDir, "pluginDir") && (p.pluginDir.startsWith("/") || p.pluginDir.split("/").includes(".."))) bad("pluginDir", "must be a relative path inside the repo");
  if (!Array.isArray(p.build)) bad("build", "must be an array of argv arrays");
  else p.build.forEach((a, i) => argv(a, `build[${i}]`));

  if (!Array.isArray(p.data)) bad("data", "must be an array");
  else
    list(p.data, "data", (e, path) => {
      allow(e, ["name", "kind", "path", "integrity", "schemaQuery", "backup"], path);
      str(e.name, `${path}.name`, { re: /^[A-Za-z0-9_.-]{1,64}$/ });
      // These names would collide with files the backup writes itself (manifest, previous source, exports).
      if (typeof e.name === "string" && (e.name === "." || e.name === ".." || e.name === "manifest.json" || e.name === "previous-source.txt" || /^export-.*\.json$/.test(e.name))) bad(`${path}.name`, `${e.name} is reserved`);
      if (!["sqlite", "file", "dir"].includes(e.kind)) bad(`${path}.kind`, "must be sqlite, file or dir");
      tmpl(e.path, `${path}.path`);
      if (e.integrity !== undefined && typeof e.integrity !== "boolean") bad(`${path}.integrity`, "must be a boolean");
      if (e.schemaQuery !== undefined) {
        str(e.schemaQuery, `${path}.schemaQuery`, { max: 400 });
        if (e.kind !== "sqlite") bad(`${path}.schemaQuery`, "only for sqlite data");
      }
      if (e.backup !== undefined && !["always", "once-per-commit"].includes(e.backup)) bad(`${path}.backup`, "must be always or once-per-commit");
      if (e.integrity && e.kind !== "sqlite") bad(`${path}.integrity`, "only for sqlite data");
    });
  const names = (arr, path) => {
    if (!Array.isArray(arr)) return;
    const seen = new Set();
    for (const e of arr) if (isObj(e) && typeof e.name === "string") (seen.has(e.name) ? bad(path, `duplicate name ${e.name}`) : seen.add(e.name));
  };
  names(p.data, "data");

  if (p.export !== undefined) {
    list(p.export, "export", (e, path) => {
      allow(e, ["name", "run", "required"], path);
      str(e.name, `${path}.name`, { re: /^[A-Za-z0-9_.-]{1,64}$/ });
      if (typeof e.name === "string" && (e.name === "." || e.name === "..")) bad(`${path}.name`, `${e.name} is reserved`);
      argv(e.run, `${path}.run`);
      if (typeof e.required !== "boolean") bad(`${path}.required`, "must be a boolean");
    });
    names(p.export, "export");
  }
  for (const phase of ["preflight", "preInstall", "preEnable", "postEnablePreConfig", "previewGate"]) {
    if (p[phase] === undefined) continue;
    list(p[phase], phase, checkEntry);
    names(p[phase], phase);
  }
  if (p.configAfterEnable !== undefined)
    list(p.configAfterEnable, "configAfterEnable", (e, path) => {
      allow(e, ["key", "value", "requiresMarker"], path);
      str(e.key, `${path}.key`, { re: /^[A-Za-z0-9_.-]{1,100}$/ });
      tmpl(e.value, `${path}.value`);
      if (e.requiresMarker !== undefined) str(e.requiresMarker, `${path}.requiresMarker`, { re: /^[A-Za-z0-9_]{1,64}$/ });
    });
  if (!Array.isArray(p.health)) bad("health", "must be an array");
  else
    list(p.health, "health", (e, path) => {
      if (!HEALTH_KINDS.includes(e.kind)) return bad(`${path}.kind`, `must be one of ${HEALTH_KINDS.join(", ")}`);
      const timeout = () => {
        if (e.timeoutSec !== undefined && !(Number.isInteger(e.timeoutSec) && e.timeoutSec >= 0 && e.timeoutSec <= 3600)) bad(`${path}.timeoutSec`, "must be an integer 0..3600");
      };
      switch (e.kind) {
        case "running":
          allow(e, ["kind", "timeoutSec"], path);
          timeout();
          break;
        case "check":
          allow(e, ["kind", "name", "run", "marker"], path);
          str(e.name, `${path}.name`, { re: /^[A-Za-z0-9_.-]{1,64}$/ });
          argv(e.run, `${path}.run`);
          markerName(e, path);
          break;
        case "log-line":
          allow(e, ["kind", "name", "file", "pattern", "fail", "since", "timeoutSec"], path);
          str(e.name, `${path}.name`, { re: /^[A-Za-z0-9_.-]{1,64}$/ });
          tmpl(e.file, `${path}.file`);
          for (const [k, v] of [["pattern", e.pattern], ...(e.fail ?? []).map((f, i) => [`fail[${i}]`, f])]) {
            if (!str(v, `${path}.${k}`, { max: 500 })) continue;
            try {
              new RegExp(v);
            } catch {
              bad(`${path}.${k}`, "not a valid regular expression");
            }
          }
          if (e.fail !== undefined && !Array.isArray(e.fail)) bad(`${path}.fail`, "must be an array of patterns");
          if (!SINCE.includes(e.since)) bad(`${path}.since`, "must be install, enable or config");
          timeout();
          break;
        case "process-count":
          allow(e, ["kind", "name", "pattern", "count", "timeoutSec"], path);
          str(e.name, `${path}.name`, { re: /^[A-Za-z0-9_.-]{1,64}$/ });
          if (str(e.pattern, `${path}.pattern`, { max: 500 }))
            try {
              new RegExp(e.pattern);
            } catch {
              bad(`${path}.pattern`, "not a valid regular expression");
            }
          if (!(Number.isInteger(e.count) && e.count >= 0 && e.count <= 1000)) bad(`${path}.count`, "must be a non-negative integer");
          timeout();
          break;
        case "setting-equals":
          allow(e, ["kind", "key", "value", "timeoutSec"], path);
          str(e.key, `${path}.key`, { re: /^[A-Za-z0-9_.-]{1,100}$/ });
          tmpl(e.value, `${path}.value`);
          timeout();
          break;
      }
    });
  if (!Array.isArray(p.classes) || !p.classes.every((c) => typeof c === "string" && /^[a-z0-9-]{1,64}$/.test(c))) bad("classes", "must be an array of class names ([] means mk-only)");
  if (p.dataRestore !== undefined && p.dataRestore !== "manual") bad("dataRestore", 'must be "manual"');
  if (p.rollbackLeaves !== undefined && !["enabled", "disabled"].includes(p.rollbackLeaves)) bad("rollbackLeaves", "must be enabled or disabled");
  if (!Array.isArray(p.controlPaths) || p.controlPaths.length === 0 || !p.controlPaths.every((c) => typeof c === "string" && c && !c.startsWith("/") && !c.includes(".."))) bad("controlPaths", "must be a non-empty array of relative globs");
  if (!isObj(p.retention)) bad("retention", "must be an object");
  else {
    allow(p.retention, ["builds"], "retention");
    if (!(Number.isInteger(p.retention.builds) && p.retention.builds >= 1 && p.retention.builds <= 100)) bad("retention.builds", "must be an integer 1..100");
  }
  if (p.updateRequest !== undefined) {
    if (!isObj(p.updateRequest)) bad("updateRequest", "must be an object");
    else {
      allow(p.updateRequest, ["kind", "db", "key"], "updateRequest");
      if (p.updateRequest.kind !== "settings-kv") bad("updateRequest.kind", 'must be "settings-kv"');
      tmpl(p.updateRequest.db, "updateRequest.db");
      str(p.updateRequest.key, "updateRequest.key", { re: /^[A-Za-z0-9_.-]{1,100}$/ });
    }
  }
  // A marker a configAfterEnable entry requires must be declared by some check.
  const declared = new Set();
  for (const phase of ["preflight", "preInstall", "preEnable", "postEnablePreConfig", "previewGate"]) for (const e of p[phase] ?? []) if (isObj(e) && e.marker) declared.add(e.marker);
  for (const e of p.health ?? []) if (isObj(e) && e.marker) declared.add(e.marker);
  (p.configAfterEnable ?? []).forEach((e, i) => {
    if (isObj(e) && e.requiresMarker && !declared.has(e.requiresMarker)) bad(`configAfterEnable[${i}].requiresMarker`, `no check declares marker ${e.requiresMarker}`);
  });
  return errs;
}

/** Load and validate a plugin file. Throws ToolError (exit 1) listing every problem. */
export function loadPluginFile(path) {
  let raw;
  try {
    raw = readFileSync(path, "utf8");
  } catch (e) {
    throw new ToolError(`cannot read plugin file ${path}: ${e.message}`);
  }
  let obj;
  try {
    obj = JSON.parse(raw);
  } catch (e) {
    throw new ToolError(`plugin file ${path} is not valid JSON: ${e.message}`);
  }
  const errs = validatePluginObject(obj);
  if (errs.length) throw new ToolError(`plugin file ${path} is invalid:\n  ${errs.join("\n  ")}`, EXIT.ERROR, { problems: errs });
  return { ...obj, rollbackLeaves: obj.rollbackLeaves ?? "enabled", export: obj.export ?? [], configAfterEnable: obj.configAfterEnable ?? [], preflight: obj.preflight ?? [], preInstall: obj.preInstall ?? [], preEnable: obj.preEnable ?? [], postEnablePreConfig: obj.postEnablePreConfig ?? [], previewGate: obj.previewGate ?? [] };
}

/** Substitute {placeholders} in one string. An unknown or unset placeholder is an error, never silently empty. */
export function template(s, vars) {
  return s.replace(PH_RE, (_m, name) => {
    if (!PH.has(name)) throw new ToolError(`unknown placeholder {${name}}`);
    const v = vars[name];
    if (typeof v !== "string") throw new ToolError(`placeholder {${name}} has no value in this context`);
    return v;
  });
}
export const templateArgv = (argv, vars) => argv.map((a) => template(a, vars));
