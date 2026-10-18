// Property tests for the pure parts of Home: the lint ratchet, migration ordering and the ask
// round-trip. Each draws many cases from a seeded generator (no extra dependency), so a failure
// reproduces from the seed printed in its title.
import Database from "better-sqlite3";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { diff, KINDS } from "../scripts/check-lint-ratchet.mjs";
import { CODE_VERSION, MIGRATIONS, migrate, readSchemaState, SchemaTooNewError, type Migration } from "../migrations.js";
import { canonicalJson, identity, normalizedJson, parseAsk, revision, semanticKey } from "../model.js";
import { tmpDir } from "./helpers.js";

const SEED = 20261014;
// mulberry32: small, fast and good enough to drive generators.
function rng(seed: number) {
  let s = seed >>> 0;
  const next = () => {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = s;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
  const int = (n: number) => Math.floor(next() * n);
  const pick = <T>(xs: readonly T[]): T => xs[int(xs.length)]!;
  const shuffle = <T>(xs: readonly T[]): T[] => {
    const a = [...xs];
    for (let i = a.length - 1; i > 0; i--) {
      const j = int(i + 1);
      [a[i], a[j]] = [a[j]!, a[i]!];
    }
    return a;
  };
  return { next, int, pick, shuffle };
}

describe(`escape ratchet never accepts an increase (seed ${SEED})`, () => {
  type Counts = Record<string, Record<string, number>>;
  const files = ["a.ts", "b.ts", "c/d.tsx", "e.mjs"];
  const gen = (r: ReturnType<typeof rng>): Counts => {
    const out: Counts = {};
    for (const f of files) {
      if (r.int(3) === 0) continue;
      const e: Record<string, number> = {};
      for (const k of KINDS) if (r.int(2)) e[k] = r.int(4);
      out[f] = e;
    }
    return out;
  };
  const get = (c: Counts, f: string, k: string) => c[f]?.[k] ?? 0;

  it("rose is non-empty exactly when some count went up, fell exactly when some went down", () => {
    const r = rng(SEED);
    for (let i = 0; i < 2000; i++) {
      const base = gen(r);
      const live = gen(r);
      let up = 0;
      let down = 0;
      for (const f of files) for (const k of KINDS) {
        const b = get(base, f, k);
        const l = get(live, f, k);
        if (l > b) up++;
        if (l < b) down++;
      }
      const d = diff(base, live);
      expect(d.rose.length, `case ${i}`).toBe(up);
      expect(d.fell.length, `case ${i}`).toBe(down);
    }
  });

  it("raising any single count of any baseline is always reported, including a new file", () => {
    const r = rng(SEED + 1);
    for (let i = 0; i < 500; i++) {
      const base = gen(r);
      const live: Counts = JSON.parse(JSON.stringify(base));
      const f = r.pick(files);
      const k = r.pick(KINDS);
      live[f] ??= {};
      live[f]![k] = get(base, f, k) + 1 + r.int(3);
      expect(diff(base, live).rose, `case ${i}`).toContain(`${f}: ${k} ${get(base, f, k)} -> ${live[f]![k]}`);
    }
  });

  it("identical counts report nothing either way", () => {
    const r = rng(SEED + 2);
    for (let i = 0; i < 200; i++) {
      const c = gen(r);
      expect(diff(c, JSON.parse(JSON.stringify(c)))).toEqual({ rose: [], fell: [] });
    }
  });
});

describe(`migration ordering (seed ${SEED})`, () => {
  let t: ReturnType<typeof tmpDir>;
  beforeEach(() => void (t = tmpDir()));
  afterEach(() => t.cleanup());

  const schemaOf = (db: Database.Database) =>
    db.prepare("SELECT type, name, sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name").all();
  const open = (name: string) => new Database(join(t.dir, name));
  // Each run needs its own backup file name, and the name has one-second resolution.
  let tick = 0;
  const now = () => new Date(Date.UTC(2026, 9, 14, 0, 0, tick++));

  it("the shipped list is strictly ascending, gapless from 1, and ends at CODE_VERSION", () => {
    MIGRATIONS.forEach((m, i) => expect(m.version).toBe(i + 1));
    expect(MIGRATIONS[MIGRATIONS.length - 1]!.version).toBe(CODE_VERSION);
  });

  it("any order of the migrations list gives the same schema as the sorted one", () => {
    const r = rng(SEED + 3);
    const ref = open("ref.db");
    migrate(ref);
    const want = schemaOf(ref);
    for (let i = 0; i < 12; i++) {
      const db = open(`perm${i}.db`);
      const state = migrate(db, { migrations: r.shuffle(MIGRATIONS), now });
      expect(state.schemaVersion, `perm ${i}`).toBe(CODE_VERSION);
      expect(schemaOf(db), `perm ${i}`).toEqual(want);
      db.close();
    }
  }, 30_000);

  it("stopping at any version and resuming equals one run, and a version is never applied twice", () => {
    const ref = open("one.db");
    migrate(ref);
    const want = schemaOf(ref);
    for (let stop = 1; stop <= CODE_VERSION; stop++) {
      const db = open(`stop${stop}.db`);
      expect(migrate(db, { codeVersion: stop, now }).schemaVersion).toBe(stop);
      expect(readSchemaState(db).schemaVersion).toBe(stop);
      expect(migrate(db, { now }).schemaVersion).toBe(CODE_VERSION);
      expect(schemaOf(db), `stop ${stop}`).toEqual(want);
      // a further run changes nothing
      expect(migrate(db, { now }).schemaVersion).toBe(CODE_VERSION);
      expect(schemaOf(db)).toEqual(want);
      db.close();
    }
  }, 30_000);

  it("a migration above codeVersion is never applied, and older code never lowers the stored version", () => {
    const extra: Migration = { version: CODE_VERSION + 1, sql: "CREATE TABLE should_not_exist (x INTEGER);" };
    const db = open("cap.db");
    const s = migrate(db, { migrations: [...MIGRATIONS, extra], now });
    expect(s.schemaVersion).toBe(CODE_VERSION);
    expect(db.prepare("SELECT 1 FROM sqlite_master WHERE name='should_not_exist'").get()).toBeUndefined();
    // One database, upgraded to the newest schema, then opened by code that knows only an older one:
    // code at or above the reader fence leaves the version and schema put; code below it is refused.
    for (let v = 1; v < CODE_VERSION; v++) {
      const d = open(`down${v}.db`);
      migrate(d, { now });
      const before = schemaOf(d);
      if (v < readSchemaState(d).minReaderVersion) {
        expect(() => migrate(d, { codeVersion: v, now }), `code ${v}`).toThrow(SchemaTooNewError);
        expect(readSchemaState(d).schemaVersion, `code ${v}`).toBe(CODE_VERSION);
        d.close();
        continue;
      }
      expect(migrate(d, { codeVersion: v, now }).schemaVersion, `code ${v}`).toBe(CODE_VERSION);
      expect(readSchemaState(d).schemaVersion, `code ${v}`).toBe(CODE_VERSION);
      expect(schemaOf(d), `code ${v}`).toEqual(before);
      d.close();
    }
  }, 30_000);
});

describe(`ask round-trip (seed ${SEED})`, () => {
  const schemaDir = join(__dirname, "..", "..", "..", "schema", "home-ask", "v1");
  const vectors = JSON.parse(readFileSync(join(schemaDir, "vectors.json"), "utf8"));

  it("decoding a vector's normalized form gives the same normalized form, identity, revision and key", () => {
    for (const c of vectors.asks) {
      const first = parseAsk(c.input);
      const again = parseAsk(JSON.parse(normalizedJson(first)));
      expect(normalizedJson(again), c.name).toBe(c.normalized);
      expect(identity(again), c.name).toBe(c.identity);
      expect(revision(again), c.name).toBe(c.revision);
      expect(semanticKey(again), c.name).toBe(c.semantic_key);
    }
  });

  const words = ["alpha", "Beta", "gamma  delta", "É", "é", "x", "A b", " pad ", "ünï", "Z"];
  const genAsk = (r: ReturnType<typeof rng>) => {
    const n = 2 + r.int(3);
    const used = new Set<string>();
    const options: { label: string }[] = [];
    while (options.length < n) {
      const label = `${r.pick(words)} ${r.int(1000)}`;
      if (!used.has(label.toLowerCase())) {
        used.add(label.toLowerCase());
        options.push({ label });
      }
    }
    return {
      v: 1, kind: "decide", project: "Autarch", project_root: "/home/dev/projects/Autarch", asker: "thread",
      thread: `thr_${r.int(100000)}`, question: `${r.pick(words)} ${r.pick(words)}?`, subject: r.int(2) ? `s ${r.pick(words)}` : "", options,
    };
  };

  it("normalizing twice equals normalizing once, and key order never matters (generated asks)", () => {
    const r = rng(SEED + 4);
    for (let i = 0; i < 300; i++) {
      const input = genAsk(r);
      const once = parseAsk(input);
      const twice = parseAsk(JSON.parse(normalizedJson(once)));
      expect(normalizedJson(twice), `case ${i}`).toBe(normalizedJson(once));
      const shuffled = Object.fromEntries(r.shuffle(Object.entries(input)));
      expect(normalizedJson(parseAsk(shuffled)), `case ${i}`).toBe(normalizedJson(once));
      expect(identity(twice), `case ${i}`).toBe(identity(once));
      expect(canonicalJson(shuffled), `case ${i}`).toBe(canonicalJson(input));
    }
  });
});
