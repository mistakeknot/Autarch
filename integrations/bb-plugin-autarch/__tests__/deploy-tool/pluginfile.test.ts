import { afterEach, describe, expect, it } from "vitest";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { PLACEHOLDERS, loadPluginFile, template, validatePluginObject } from "../../../../tools/bb-plugin-deploy/lib/pluginfile.mjs";
import { TOOL_SRC, defaultPlugin, type Json } from "./rig";

const good = (): Json => defaultPlugin("/nonexistent/upstream");
const problems = (patch: Json): string[] => validatePluginObject({ ...good(), ...patch }) as string[];
const dirs: string[] = [];
afterEach(() => {
  while (dirs.length) rmSync(dirs.pop() ?? "", { recursive: true, force: true });
});
const writePlugin = (obj: unknown): string => {
  const d = mkdtempSync(join(tmpdir(), "bpd-pf-"));
  dirs.push(d);
  const p = join(d, "deploy.json");
  writeFileSync(p, typeof obj === "string" ? obj : JSON.stringify(obj));
  return p;
};

describe("plugin file validation", () => {
  it("the fixture plugin file is valid", () => {
    expect(validatePluginObject(good())).toEqual([]);
  });

  it("the plugin's own deploy.json is accepted as-is by the loader and the CLI", () => {
    const file = join(__dirname, "..", "..", "deploy.json");
    expect(validatePluginObject(JSON.parse(readFileSync(file, "utf8")))).toEqual([]);
    expect(() => loadPluginFile(file)).not.toThrow();
    const r = spawnSync(process.execPath, [join(TOOL_SRC, "cli.mjs"), "validate-plugin-file", "--plugin-file", file, "--json"], { encoding: "utf8" });
    expect(r.status, r.stdout + r.stderr).toBe(0);
  });

  it("an unknown top-level key fails closed", () => {
    expect(problems({ surprise: 1 })).toContain("surprise: unknown key");
  });

  it("a wrong or missing format is rejected", () => {
    expect(problems({ format: "bb-plugin-deploy/plugin@2" }).join("\n")).toMatch(/format/);
    const o = good();
    delete o.format;
    expect((validatePluginObject(o) as string[]).join("\n")).toMatch(/format/);
  });

  it("an unknown placeholder is rejected wherever a template is allowed", () => {
    expect(problems({ build: [["node", "{nope}"]] }).join("\n")).toMatch(/unknown placeholder \{nope\}/);
    expect(problems({ data: [{ name: "d", kind: "file", path: "{home}/x" }] }).join("\n")).toMatch(/unknown placeholder \{home\}/);
    expect(problems({ preflight: [{ name: "p", run: ["x", "{bogus}"] }] }).join("\n")).toMatch(/unknown placeholder \{bogus\}/);
  });

  it("every placeholder in the closed set is accepted", () => {
    const run = PLACEHOLDERS.map((n: string) => `{${n}}`);
    expect(problems({ preflight: [{ name: "all", run: ["x", ...run] }] })).toEqual([]);
  });

  it("a command that is not an argv array is rejected (no shell strings)", () => {
    expect(problems({ preflight: [{ name: "p", run: "echo hi" }] }).join("\n")).toMatch(/argv array/);
    expect(problems({ build: ["npm run build"] }).join("\n")).toMatch(/argv array/);
    expect(problems({ export: [{ name: "e", run: [], required: true }] }).join("\n")).toMatch(/argv array/);
  });

  it("an unknown health kind is rejected, and each kind rejects unknown keys", () => {
    expect(problems({ health: [{ kind: "vibes" }] }).join("\n")).toMatch(/health\[0\]\.kind/);
    expect(problems({ health: [{ kind: "running", extra: 1 }] }).join("\n")).toMatch(/extra: unknown key/);
    expect(problems({ health: [{ kind: "process-count", name: "p", pattern: "x", count: -1 }] }).join("\n")).toMatch(/count/);
    expect(problems({ health: [{ kind: "log-line", name: "l", file: "{stateDir}/l", pattern: "(", since: "install" }] }).join("\n")).toMatch(/regular expression/);
    expect(problems({ health: [{ kind: "log-line", name: "l", file: "{stateDir}/l", pattern: "ok", since: "yesterday" }] }).join("\n")).toMatch(/since/);
    expect(problems({ health: [{ kind: "setting-equals", key: "k", value: "v", timeoutSec: 99999 }] }).join("\n")).toMatch(/timeoutSec/);
  });

  it("every valid health kind is accepted", () => {
    expect(
      problems({
        health: [
          { kind: "running", timeoutSec: 5 },
          { kind: "log-line", name: "l", file: "{stateDir}/l.log", pattern: "ready", fail: ["boom"], since: "enable", timeoutSec: 5 },
          { kind: "process-count", name: "p", pattern: "worker", count: 2 },
          { kind: "check", name: "c", run: ["true"] },
          { kind: "setting-equals", key: "feature", value: "on" },
        ],
      }),
    ).toEqual([]);
  });

  it("retention.builds must be 1..100", () => {
    for (const builds of [0, 101, 2.5, "5"]) expect(problems({ retention: { builds } }).join("\n")).toMatch(/retention\.builds/);
    expect(problems({ retention: { builds: 1 } })).toEqual([]);
    expect(problems({ retention: { builds: 100 } })).toEqual([]);
    expect(problems({ retention: { builds: 5, extra: 1 } }).join("\n")).toMatch(/unknown key/);
  });

  it("duplicate names inside a phase or data list are rejected", () => {
    const e = { name: "same", run: ["true"] };
    expect(problems({ preflight: [e, e] }).join("\n")).toMatch(/duplicate name same/);
    const d = { name: "d", kind: "file", path: "{bbData}/x" };
    expect(problems({ data: [d, d] }).join("\n")).toMatch(/duplicate name d/);
  });

  it("data entries: kind, backup mode and sqlite-only options are checked", () => {
    expect(problems({ data: [{ name: "d", kind: "blob", path: "{bbData}/x" }] }).join("\n")).toMatch(/kind/);
    expect(problems({ data: [{ name: "d", kind: "file", path: "{bbData}/x", backup: "sometimes" }] }).join("\n")).toMatch(/backup/);
    expect(problems({ data: [{ name: "d", kind: "file", path: "{bbData}/x", integrity: true }] }).join("\n")).toMatch(/integrity/);
    expect(problems({ data: [{ name: "d", kind: "dir", path: "{bbData}/x", schemaQuery: "select 1" }] }).join("\n")).toMatch(/schemaQuery/);
    expect(problems({ data: [{ name: "d", kind: "sqlite", path: "{bbData}/x.db", integrity: true, backup: "once-per-commit" }] })).toEqual([]);
  });

  it("configAfterEnable may only require a marker some check declares", () => {
    expect(problems({ configAfterEnable: [{ key: "k", value: "v", requiresMarker: "ghost" }] }).join("\n")).toMatch(/no check declares marker ghost/);
    expect(problems({ configAfterEnable: [{ key: "k", value: "v", requiresMarker: "schema" }] })).toEqual([]);
  });

  it("dataRestore, rollbackLeaves, controlPaths, classes and updateRequest are constrained", () => {
    expect(problems({ dataRestore: "auto" }).join("\n")).toMatch(/dataRestore/);
    expect(problems({ dataRestore: "manual" })).toEqual([]);
    expect(problems({ rollbackLeaves: "maybe" }).join("\n")).toMatch(/rollbackLeaves/);
    expect(problems({ rollbackLeaves: "disabled" })).toEqual([]);
    expect(problems({ controlPaths: [] }).join("\n")).toMatch(/controlPaths/);
    expect(problems({ controlPaths: ["/abs"] }).join("\n")).toMatch(/controlPaths/);
    expect(problems({ controlPaths: ["../up"] }).join("\n")).toMatch(/controlPaths/);
    expect(problems({ classes: "plugin-deploy" }).join("\n")).toMatch(/classes/);
    expect(problems({ classes: [] })).toEqual([]);
    expect(problems({ updateRequest: { kind: "file", db: "x", key: "k" } }).join("\n")).toMatch(/updateRequest\.kind/);
    expect(problems({ pluginDir: "/abs" }).join("\n")).toMatch(/pluginDir/);
    expect(problems({ pluginDir: "a/../b" }).join("\n")).toMatch(/pluginDir/);
  });
});

describe("loadPluginFile and the validate-plugin-file command", () => {
  it("loads a valid file and fills defaults", () => {
    const p = loadPluginFile(writePlugin(good()));
    expect(p.rollbackLeaves).toBe("enabled");
    expect(p.previewGate).toEqual([]);
  });

  it("an invalid file throws one error listing every problem", () => {
    const path = writePlugin({ ...good(), surprise: 1, retention: { builds: 0 } });
    expect(() => loadPluginFile(path)).toThrow(/is invalid:\n {2}surprise: unknown key\n {2}retention\.builds/);
  });

  it("a file that is not JSON or does not exist is a plain error", () => {
    expect(() => loadPluginFile(writePlugin("{nope"))).toThrow(/not valid JSON/);
    expect(() => loadPluginFile(join(tmpdir(), "bpd-no-such-file.json"))).toThrow(/cannot read plugin file/);
  });

  const run = (path: string) => spawnSync(process.execPath, [join(TOOL_SRC, "cli.mjs"), "validate-plugin-file", "--plugin-file", path, "--json"], { encoding: "utf8" });

  it("validate-plugin-file exits 0 for a valid file and 1 for an invalid one", () => {
    expect(run(writePlugin(good())).status).toBe(0);
    const bad = run(writePlugin({ ...good(), surprise: 1 }));
    expect(bad.status).toBe(1);
    expect(bad.stdout + bad.stderr).toMatch(/surprise: unknown key/);
  });
});

describe("templating", () => {
  const vars = { bbData: "/d", stateDir: "/s", targetCommit: "abc" };

  it("substitutes every occurrence", () => {
    expect(template("{bbData}/x/{targetCommit}/{bbData}", vars)).toBe("/d/x/abc//d");
  });

  it("an unknown placeholder throws", () => {
    expect(() => template("{nope}", vars)).toThrow(/unknown placeholder \{nope\}/);
  });

  it("a known placeholder with no value in this context throws instead of becoming empty", () => {
    expect(() => template("{installedCommit}", vars)).toThrow(/has no value in this context/);
  });

  it("text without placeholders is unchanged, including braces that are not names", () => {
    expect(template("a{1}b {} c", vars)).toBe("a{1}b {} c");
  });
});
