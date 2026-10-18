import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { readRealEnv } from "../e2e/real.js";

const dir = () => {
  const d = mkdtempSync(join(tmpdir(), "real-env-"));
  writeFileSync(join(d, "bb.db"), "");
  return d;
};
const ok = (d: string) => ({ HOME_E2E_BB: "http://127.0.0.1:33143", HOME_E2E_BB_HOST_PORT: "37761", HOME_E2E_BB_DATA: d });

describe("readRealEnv refuses anything but a separate loopback server", () => {
  it("accepts an isolated server", () => expect(readRealEnv(ok(dir())).url).toBe("http://127.0.0.1:33143"));
  it("needs all three variables", () => expect(() => readRealEnv({})).toThrow(/isolated/));
  it("refuses a non-loopback host", () => expect(() => readRealEnv({ ...ok(dir()), HOME_E2E_BB: "https://bb.example.com" })).toThrow(/loopback/));
  it("refuses the ambient server", () => expect(() => readRealEnv({ ...ok(dir()), BB_SERVER_URL: "http://127.0.0.1:33143" })).toThrow(/ambient/));
  it("refuses the ambient data dir", () => {
    const d = dir();
    expect(() => readRealEnv({ ...ok(d), BB_DATA_DIR: d })).toThrow(/ambient/);
  });
  it("refuses a live machine's data", () => {
    const d = join(mkdtempSync(join(tmpdir(), "x-")), ".bb-machines");
    mkdirSync(d);
    writeFileSync(join(d, "bb.db"), "");
    expect(() => readRealEnv(ok(d))).toThrow(/live machine/);
  });
});
