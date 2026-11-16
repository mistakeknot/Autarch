import type Database from "better-sqlite3";
import { z } from "zod";
import { dirname, join } from "node:path";
import type { PluginsLike } from "./tasks.js";
import { applyFloor, choose, measurePace, readFloor, weekly, weeklyObserved, weeklyReset, num, type Account, type Config, type Floor, type Part, type Samples } from "./governor-policy.js";

export type Mode = "off" | "shadow" | "live";
export type Card = { key?: string; level: number; answered?: string; declined_at?: number; filed_at?: number; renewals?: number };
export type State = { level: number; good_streak?: number; pace?: number | null; since?: number; why?: string; updated_at?: number; pending?: boolean; card?: Card; throttle?: Throttle };
export type Throttle = { level: number; name: string; allowed_max: number | null; cuts_protected: boolean; pace: number | null; updated_at: number; why: string };
export type Decision = { level: number; new_level: number; pace: number | null; note: string; why: string; floor: Floor | null; held?: string | null; applied: string[] };
export type View = { mode: Mode; state?: State; decision?: Decision; script?: unknown; last_run?: number; parts?: Part[]; error?: string; warnings?: string[] };
export type NoticePlan = { skipped: string[]; send(): Promise<string[]> };
export interface GovernorIO {
  now(): number; hostname(): string; config(): Promise<Config>; mode(): Promise<Mode>;
  timerActive(): Promise<boolean>; accounts(): Promise<Account[]>; readJson(path: string): Promise<unknown>;
  inferLevel(cfg: Config): Promise<number>; answer(key: string): Promise<"a" | "b" | null>;
  fileCard(cfg: Config, level: number, pace: number | null, renewal: number): Promise<string>;
  apply(cfg: Config, level: number, force: boolean): Promise<void>;
  prepareNotify(cfg: Config, old: number, next: number, pace: number | null, why: string): Promise<NoticePlan>;
  writeThrottle(cfg: Config, throttle: Throttle): Promise<void>; publish(view: View): void;
}
// Separate tables in the plugin's SQLite database; no changes to Home's reader contract.
export class GovernorStore {
  constructor(readonly db: Database.Database) {
    db.exec(`CREATE TABLE IF NOT EXISTS quota_governor_state (key TEXT PRIMARY KEY, json TEXT NOT NULL);
      CREATE TABLE IF NOT EXISTS quota_governor_samples (id TEXT NOT NULL, observed REAL NOT NULL, utilization REAL NOT NULL, reset REAL NOT NULL, PRIMARY KEY(id,observed,reset));`);
  }
  get<T>(key: string): T | null {
    const r = this.db.prepare("SELECT json FROM quota_governor_state WHERE key=?").get(key) as { json: string } | undefined;
    return r ? JSON.parse(r.json) as T : null;
  }
  put(key: string, v: unknown): void { this.db.prepare("INSERT OR REPLACE INTO quota_governor_state VALUES (?,?)").run(key, JSON.stringify(v)); }
  sample(accounts: Account[], now: number, cfg: Config): void {
    this.db.transaction(() => {
      const ins = this.db.prepare("INSERT OR REPLACE INTO quota_governor_samples VALUES (?,?,?,?)");
      for (const a of accounts) {
        const w = weekly(a, now), reset = weeklyReset(a), obs = weeklyObserved(a);
        if (a.provider === "claude" && w && num(w[0]) && num(reset) && num(obs) && obs <= now && obs >= now - cfg.recent_window_h * 3600000) ins.run(a.id, obs, w[0], reset);
      }
      this.db.prepare("DELETE FROM quota_governor_samples WHERE observed < ?").run(now - cfg.recent_window_h * 3600000);
    })();
  }
  samples(now: number, cfg: Config): Samples {
    const out: Samples = {};
    const rows = this.db.prepare("SELECT * FROM quota_governor_samples WHERE observed BETWEEN ? AND ? ORDER BY observed,utilization").all(now - cfg.recent_window_h * 3600000, now) as { id: string; observed: number; utilization: number; reset: number }[];
    for (const r of rows) (out[r.id] ??= []).push([r.observed, r.utilization, r.reset]);
    return out;
  }
}
export class Governor {
  private current: View = { mode: "off" };
  constructor(private store: GovernorStore, private io: GovernorIO) {}
  view(): View { return this.current; }
  async tick(): Promise<void> {
    const io = this.io;
    try {
      const mode = await io.mode();
      this.current = { ...this.current, mode, error: undefined };
      if (mode === "off") return;
      const cfg = await io.config();
      if (!cfg.bb_host || io.hostname() !== cfg.bb_host) { this.current.error = "Inactive on this host"; return; }
      if (mode === "live" && await io.timerActive()) { this.current.error = "Legacy quota-governor.timer or quota-governor.service is active; live refused"; return; }
      const now = io.now();
      const script = await io.readJson(join(cfg.state_dir, "state.json"));
      let st = this.store.get<State>(mode);
      if (!st) {
        st = script && typeof script === "object" ? structuredClone(script) as State : { level: await io.inferLevel(cfg) };
        // Reconcile inherited incomplete script transitions only in live mode.
        this.store.put(mode, st);
      }
      if (!Number.isInteger(st.level) || !cfg.levels[st.level]) throw Error("Invalid persisted governor level");
      const accounts = await io.accounts();
      this.store.sample(accounts, now, cfg);
      const parts: Part[] = [];
      const [pace, note] = measurePace(accounts, now, cfg, this.store.samples(now, cfg), parts);
      const floor = readFloor(await io.readJson(join(cfg.state_dir, "floor.json")).catch(() => null), now, cfg);
      let [next, streak, why] = applyFloor(st.level, ...choose(st.level, pace, st.good_streak ?? 0, cfg), floor);
      const cur = st.level, levels = cfg.levels;
      const ns: State = { ...st, good_streak: streak, pace, updated_at: Math.floor(now / 1000) };
      const dec: Decision = { level: cur, new_level: next, pace, note, why, floor, applied: [] };
      if (st.pending && mode === "live") {
        try { await io.apply(cfg, cur, true); delete ns.pending; dec.applied.push(`reconciled level ${cur} after an earlier failure`); }
        catch (e) { next = cur; dec.applied.push(`reconcile FAILED: ${String(e)}`); }
      }
      if (next > cur && levels[next].cuts_protected && !levels[cur].cuts_protected) {
        let card: Card = st.card ?? { level: next };
        let ans = card.key && card.level === next ? await io.answer(card.key) : null;
        if (ans == null && card.answered === "b" && card.level === next) ans = "b";
        if (ans === "b") {
          const first = (card.answered === "b" ? card.declined_at : undefined) || Math.floor(now / 1000);
          if (now / 1000 - first >= cfg.card_hold_declined_s) { card = { level: next, renewals: (card.renewals ?? 0) + 1 }; ans = null; }
          else card = { ...card, declined_at: first };
        }
        if (ans === "a") ns.card = { ...card, answered: "a" };
        else {
          let held = `level ${next} would cut protected work; `;
          if (ans === "b") { ns.card = { ...card, answered: "b" }; held += `mk declined (card ${card.key})`; }
          else if (card.key && card.level === next && !card.answered) held += `waiting on card ${card.key}`;
          else if (mode === "live") {
            try {
              const rn = card.level === next ? card.renewals ?? 0 : 0;
              const key = await io.fileCard(cfg, next, pace, rn);
              ns.card = { key, level: next, filed_at: Math.floor(now / 1000), renewals: rn };
              held += `filed card ${key}`;
            } catch (e) { held += `card not filed (${String(e)})`; }
          } else held += "would file a card";
          next = Math.max(cur, ...levels.filter(l => l.level > cur && !l.cuts_protected && l.enter != null && pace != null && pace >= l.enter).map(l => l.level));
          dec.held = held;
        }
      }
      dec.new_level = next;
      let notices: NoticePlan | undefined;
      const warnings: string[] = [];
      if (next !== cur) {
        let ready = mode === "shadow";
        try {
          if (mode === "live") { notices = await io.prepareNotify(cfg, cur, next, pace, why); warnings.push(...notices.skipped); ready = true; }
        } catch (e) { dec.applied.push(`FAILED: notification preflight: ${String(e)}`); }
        if (ready) {
          try {
            if (mode === "live") await io.apply(cfg, next, false);
            ns.level = next; ns.since = Math.floor(now / 1000); ns.why = why; ns.good_streak = 0;
            dec.applied.push(`${mode === "shadow" ? "shadow: would apply" : "applied"} level ${next}`);
          } catch (e) {
            dec.applied.push(`FAILED: ${String(e)}`);
            try { await io.apply(cfg, cur, true); } catch { ns.pending = true; }
          }
        }
      }
      const lv = levels[ns.level];
      ns.throttle = { level: ns.level, name: lv.name, allowed_max: lv.allowed_max, cuts_protected: lv.cuts_protected, pace, updated_at: Math.floor(now / 1000), why };
      this.store.put(mode, ns);
      const errors: string[] = [];
      let scriptThrottle: unknown = null;
      try { scriptThrottle = await io.readJson(join(cfg.state_dir, "throttle.json")); }
      catch (e) { errors.push(`throttle.json read failed: ${String(e)}`); }
      this.current = { mode, state: ns, decision: dec, script: { state: script, throttle: scriptThrottle }, last_run: now, parts, warnings };
      const failure = dec.applied.find(s => /FAILED/.test(s)) ?? (dec.held?.includes("card not filed") ? dec.held : undefined);
      if (failure) errors.push(failure);
      if (mode === "live") {
        try { await io.writeThrottle(cfg, ns.throttle); }
        catch (e) { errors.push(`throttle.json write failed: ${String(e)}`); }
      }
      if (errors.length) this.current.error = errors.join("; ");
      // Policy and the persisted level now agree. A compatibility-file failure
      // must not suppress notices, and delivery must never undo the commit.
      if (notices && ns.level === next && next !== cur) {
        try { warnings.push(...await notices.send()); }
        catch (e) { warnings.push(`Notification delivery failed after commit: ${String(e)}`); }
      }
      this.store.put("view", this.current);
    } catch (e) { this.current.error = String(e); }
    finally { io.publish(this.current); }
  }
}
export async function setHostLimit(plugins: PluginsLike, host: string, limit: number | "auto", signal?: AbortSignal): Promise<void> {
  const schema = z.object({ globalLimit: z.number().nullable(), hostOverrides: z.array(z.object({ hostId: z.string(), limit: z.number() })), hosts: z.array(z.object({ id: z.string(), name: z.string() })) });
  const config = schema.parse(await plugins.callRpc({ pluginId: "concurrency-limit", method: "getConfiguration", input: null, outputSchema: schema, signal }));
  const targets = config.hosts.filter(h => h.name === host);
  if (targets.length !== 1) throw Error("Governed host not uniquely found");
  const hostId = targets[0].id;
  const hostOverrides = config.hostOverrides.filter(h => h.hostId !== hostId);
  if (limit !== "auto") hostOverrides.push({ hostId, limit });
  await plugins.callRpc({ pluginId: "concurrency-limit", method: "setConfiguration", input: { globalLimit: config.globalLimit, hostOverrides }, outputSchema: z.unknown(), signal });
}
export interface AtomicFS {
  mkdir(path: string, options: { recursive: true }): Promise<unknown>;
  writeFile(path: string, data: string, options: { mode: number }): Promise<unknown>;
  rename(from: string, to: string): Promise<unknown>;
}
export async function atomicJson(path: string, obj: unknown, fs: AtomicFS): Promise<void> {
  await fs.mkdir(dirname(path), { recursive: true });
  await fs.writeFile(`${path}.tmp`, JSON.stringify(obj, null, 1), { mode: 0o600 });
  await fs.rename(`${path}.tmp`, path);
}
