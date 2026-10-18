import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { appendRuling, ledgerId, ledgerPath, type LedgerPick } from "../ledger.js";
import { ask, makeEnv, OPTIONS, type Env } from "./service-helpers.js";

const pick = (over: Partial<LedgerPick> = {}): LedgerPick => ({
  pick_id: "pk-1",
  picked_at: "2026-10-10T12:00:00.000Z",
  by: "mk",
  surface: "home",
  card: "AUTA-1",
  subject: "Collapse?",
  option_id: "day",
  option_label: "Collapse per day",
  reason: "fewer\nfiles",
  ...over,
});

describe("ledger file", () => {
  let dir: string;
  let path: string;
  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), "ledger-"));
    path = join(dir, "nested", "rulings.jsonl");
  });
  afterEach(() => rmSync(dir, { recursive: true, force: true }));
  const lines = () => readFileSync(path, "utf8").split("\n").filter(Boolean).map((l) => JSON.parse(l));

  it("creates the file and writes the documented fields", () => {
    expect(appendRuling(path, pick())).toBe(true);
    expect(lines()).toEqual([
      {
        id: "home-pk-1",
        ts: "2026-10-10T12:00:00.000Z",
        by: "mk",
        surface: "home",
        card: "AUTA-1",
        subject: "Collapse?",
        option: "day",
        words: "Collapse per day: fewer files",
      },
    ]);
  });

  it("is idempotent on the pick id", () => {
    expect(appendRuling(path, pick())).toBe(true);
    expect(appendRuling(path, pick())).toBe(false);
    expect(appendRuling(path, pick({ pick_id: "pk-2" }))).toBe(true);
    expect(lines().map((l) => l.id)).toEqual(["home-pk-1", "home-pk-2"]);
  });

  it("repairs a missing final newline and leaves hand-written lines alone", () => {
    mkdirSync(join(dir, "nested"));
    const hand = '{"id":"q7","by":"mk","words":"a"}';
    writeFileSync(path, hand);
    appendRuling(path, pick());
    const text = readFileSync(path, "utf8");
    expect(text.startsWith(`${hand}\n{`)).toBe(true);
    expect(lines().map((l) => l.id)).toEqual(["q7", "home-pk-1"]);
  });

  it("never mints an id that looks like a q-number", () => {
    for (const id of ["1", "q1", "1065", "q1065"]) expect(ledgerId(id)).not.toMatch(/^q\d+$/);
  });

  it("omits the reason separator when there is no reason", () => {
    appendRuling(path, pick({ reason: null }));
    expect(lines()[0].words).toBe("Collapse per day");
  });

  it("path resolution: env value, off, default", () => {
    expect(ledgerPath({ AUTARCH_RULINGS_LEDGER: "/x/y.jsonl" })).toBe("/x/y.jsonl");
    expect(ledgerPath({ AUTARCH_RULINGS_LEDGER: "off" })).toBeNull();
    expect(ledgerPath({})).toMatch(/\.local\/state\/vizier\/rulings\.jsonl$/);
  });
});

describe("reconcile writes the ledger", () => {
  let env: Env;
  let dir: string;
  let path: string;
  beforeEach(() => {
    env = makeEnv(["Autarch", "Other"]);
    dir = mkdtempSync(join(tmpdir(), "ledger-svc-"));
    path = join(dir, "rulings.jsonl");
  });
  afterEach(() => {
    env.cleanup();
    rmSync(dir, { recursive: true, force: true });
  });
  const lines = () => (existsSync(path) ? readFileSync(path, "utf8").split("\n").filter(Boolean).map((l) => JSON.parse(l)) : []);

  it("appends once per pick, even when the ruling file fails and is retried", async () => {
    const svc = env.open({ rulingsLedger: path });
    const f = await svc.file(ask(env, {}), {});
    if (!f.ok) throw new Error(f.error);
    const rev = (svc.store.decision(f.decision_id) as { revision: string }).revision;
    chmodSync(env.roots.Autarch!, 0o555);
    const p = svc.pick(f.decision_id, "day", rev, "pk-a", "mk", "home", "because");
    expect(p.ok).toBe(true);
    expect(lines()).toHaveLength(1);
    chmodSync(env.roots.Autarch!, 0o755);
    svc.reconcile(f.decision_id);
    const out = lines();
    expect(out).toHaveLength(1);
    expect(out[0]).toMatchObject({ id: "home-pk-a", by: "mk", surface: "home", option: "day" });
    expect(out[0].words).toBe(`${OPTIONS[1]!.label}: because`);
  });

  it("writes nothing when the ledger is disabled", async () => {
    const svc = env.open({ rulingsLedger: null });
    const f = await svc.file(ask(env, {}), {});
    if (!f.ok) throw new Error(f.error);
    const rev = (svc.store.decision(f.decision_id) as { revision: string }).revision;
    svc.pick(f.decision_id, "day", rev, "pk-b", "mk", "home");
    expect(lines()).toHaveLength(0);
  });
});
