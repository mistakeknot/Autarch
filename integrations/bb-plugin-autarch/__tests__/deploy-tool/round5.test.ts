import { afterEach, describe, expect, it } from "vitest";
import { existsSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { Rig, gateEntry, type CliResult } from "./rig";

let rig: Rig;
afterEach(() => rig?.cleanup());

const deployArgs = (sha: string, mode = "agent") => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", mode];
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);
const setEnabled = (enabled: boolean): void => {
  const f = join(rig.instances[0].stateDir, "state.json");
  const st = JSON.parse(readFileSync(f, "utf8")) as Record<string, unknown>;
  st.enabled = enabled;
  writeFileSync(f, JSON.stringify(st));
};
const enabled = (): boolean => rig.state().enabled === true;

describe("a once-per-commit backup is cached only when it is complete", () => {
  const plugin = { data: [{ name: "data.db", kind: "sqlite", path: "{bbData}/plugins/fixture/data.db", backup: "once-per-commit" }] };

  it("an export failure, then a retry: the retry takes a fresh copy and restore-data --check accepts it", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ fail: { moves: true } });
    const first = rig.cli(deployArgs(Y));
    expect(first.status, msg(first)).toBe(1);
    expect(rig.mutations()).toEqual([]);
    rig.inject({ fail: {} });
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 1100);
    const second = rig.cli(deployArgs(Y));
    expect(second.status, msg(second)).toBe(0);
    const backup = (rig.record() as { installed: { backup: string } }).installed.backup;
    const m = JSON.parse(readFileSync(join(backup, "manifest.json"), "utf8")) as { data: { skipped?: string; ref?: string }[] };
    expect(m.data[0].skipped).toBeUndefined();
    const r = rig.cli(["restore-data", "--backup", backup, "--check"]);
    expect(r.status, msg(r)).toBe(0);
  });

  it("a pointer to a backup whose manifest is gone is not reused", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ failInstallMatch: Y.slice(0, 12) });
    const first = rig.cli(deployArgs(Y));
    expect(first.status, msg(first)).toBe(30);
    const cached = String(first.json?.backup);
    expect(existsSync(join(cached, "manifest.json"))).toBe(true);
    rmSync(join(cached, "manifest.json"));
    rig.inject({ failInstallMatch: "" });
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 1100);
    const second = rig.cli(deployArgs(Y));
    expect(second.status, msg(second)).toBe(0);
    const backup = (rig.record() as { installed: { backup: string } }).installed.backup;
    const m = JSON.parse(readFileSync(join(backup, "manifest.json"), "utf8")) as { data: { skipped?: string }[] };
    expect(m.data[0].skipped).toBeUndefined();
    expect(rig.cli(["restore-data", "--backup", backup, "--check"]).status).toBe(0);
  });

  it("a pointer to a backup whose manifest parses but holds no entry for the data is not reused", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ failInstallMatch: Y.slice(0, 12) });
    const first = rig.cli(deployArgs(Y));
    expect(first.status, msg(first)).toBe(30);
    const cached = join(String(first.json?.backup), "manifest.json");
    const m0 = JSON.parse(readFileSync(cached, "utf8")) as { data: unknown[] };
    expect(m0.data).toHaveLength(1);
    writeFileSync(cached, JSON.stringify({ ...m0, data: [] }));
    rig.inject({ failInstallMatch: "" });
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 1100);
    const second = rig.cli(deployArgs(Y));
    expect(second.status, msg(second)).toBe(0);
    const backup = (rig.record() as { installed: { backup: string } }).installed.backup;
    const m = JSON.parse(readFileSync(join(backup, "manifest.json"), "utf8")) as { data: { skipped?: string }[] };
    expect(m.data[0].skipped).toBeUndefined();
    expect(rig.cli(["restore-data", "--backup", backup, "--check"]).status).toBe(0);
  });
});

describe("recovery restores the plugin's enabled state instead of forcing it on", () => {
  it("a plugin that was disabled before a failed deploy is disabled after the rollback", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    setEnabled(false);
    rig.inject({ failInstallMatch: Y.slice(0, 12) });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(30);
    expect(enabled()).toBe(false);
    expect(rig.state().source).toContain(rig.W.slice(0, 12));
    expect(String((r.json as { recoveryBlock?: unknown }).recoveryBlock ?? "")).not.toMatch(/plugin enable/);
  });

  it("a plugin that was enabled stays enabled after the rollback", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ failInstallMatch: Y.slice(0, 12) });
    expect(rig.cli(deployArgs(Y)).status).toBe(30);
    expect(enabled()).toBe(true);
  });

  it("an explicit rollbackLeaves 'disabled' still leaves it disabled", () => {
    rig = new Rig({ installedKind: "stamped", plugin: { rollbackLeaves: "disabled" } });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ failInstallMatch: Y.slice(0, 12) });
    expect(rig.cli(deployArgs(Y)).status).toBe(30);
    expect(enabled()).toBe(false);
  });

  it("a preview of a disabled plugin ends with it disabled", () => {
    rig = new Rig({ installedKind: "stamped", plugin: { previewGate: [gateEntry("preview-gate", "preview")] } });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    setEnabled(false);
    const r = rig.cli(["preview", "--commit", Y]);
    expect(r.status, msg(r)).toBe(0);
    expect(enabled()).toBe(false);
    expect(rig.state().source).toContain(rig.W.slice(0, 12));
  });
});
