// Same-user, primary-host filesystem boundary. No Home writes or ordinary wake-loop participation.
import { constants as C, openSync, closeSync, fstatSync, mkdirSync, readdirSync, readSync, writeSync, fsyncSync, renameSync } from "node:fs";
import { isAbsolute } from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { spawnSync } from "node:child_process";
import { z } from "zod";
import { resolveReminderTarget, type ReminderThreadRow } from "./followthrough-target.js";
import type { WakeSdk } from "./wakes.js";
const hash = (s: string) => createHash("sha256").update(s).digest("hex");
const MAX_BYTES = 16384;
const requestSchema = z.object({
  version: z.literal(1), pick_id: z.string().regex(/^[a-zA-Z0-9_-]{1,160}$/), generation: z.number().int().positive(),
  revision: z.string().min(1).max(256), pacific_date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/),
  expires_at: z.iso.datetime({ offset: true }), reason: z.enum(["confirmed-nondelivery", "overdue-outcome"]),
}).strict();
export type ReminderRequest = z.infer<typeof requestSchema>;
export function pacificDate(now: number): string {
  const parts = new Intl.DateTimeFormat("en-US", { timeZone: "America/Los_Angeles", year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(now);
  return ["year", "month", "day"].map(key => parts.find(p => p.type === key)!.value).join("-");
}
export function parseReminderRequest(raw: unknown, now: number): ReminderRequest | null {
  const parsed = requestSchema.safeParse(raw);
  if (!parsed.success) return null;
  const r = parsed.data;
  if (r.pacific_date !== pacificDate(now) || Date.parse(r.expires_at) > now + 86400000) return null;
  return r;
}
export const reminderActionId = (r: Pick<ReminderRequest, "pick_id" | "generation" | "pacific_date">): string => hash(JSON.stringify([r.pick_id, r.generation, r.pacific_date]));
export function reminderDataDir(host: { readonly experimental_dataDir?: unknown }): string | null {
  try { const d = host.experimental_dataDir; return typeof d === "string" && d.trim() !== "" && isAbsolute(d) ? d : null; } catch { return null; }
}
export interface ReminderPick {
  pickId: string; generation: number; revision: string; option: string; owner: string | null;
  eligible: boolean; protected: boolean; delivery: "confirmed" | "none-confirmed" | "unknown";
  cancelled: boolean;
}
export interface ReminderSources {
  readPick: (id: string) => ReminderPick | null;
  listThreads: () => Promise<ReminderThreadRow[]>;
  sdk: WakeSdk;
  now: () => number;
}
const receiptSchema = z.object({
  action: z.string().regex(/^[a-f0-9]{64}$/), pick_id: z.string(), generation: z.number().int().positive(), pacific_date: z.string(),
  status: z.enum(["claimed", "accepted", "queued", "rejected", "unknown", "capped", "expired", "no-route"]),
  attempted: z.boolean(), target: z.string().nullable(), input: z.string(),
}).strict();
type Receipt = z.infer<typeof receiptSchema>;
const journalSchema = z.object({ version: z.literal(1), entries: z.array(receiptSchema).max(3) }).strict();
const absent = (e: unknown) => (e as NodeJS.ErrnoException).code === "ENOENT";
/** Linux pinned directory descriptors: every untrusted component is opened O_NOFOLLOW.
 * /proc/self/fd refers to our already-open descriptor, never to a caller-provided symlink.
 * flock locks the inherited open file description and stays held by this process's fd.
 */
export class ReminderConsumer {
  private dirs: number[] = [];
  private root = -1;
  private requests = -1;
  private receipts = -1;
  private lock = -1;
  private busy = false;
  active = false;
  constructor(dataDir: string, private readonly s: ReminderSources, private readonly uid = process.getuid?.() ?? -1) {
    try {
      if (!isAbsolute(dataDir) || this.uid < 0) throw Error("unsupported host");
      let fd = this.keep(openSync("/", C.O_RDONLY | C.O_DIRECTORY | C.O_NOFOLLOW));
      for (const part of dataDir.split("/").filter(Boolean)) {
        if (part === "." || part === "..") throw Error("unsafe path");
        fd = this.keep(openSync(this.path(fd, part), C.O_RDONLY | C.O_DIRECTORY | C.O_NOFOLLOW));
      }
      for (const part of ["plugins", "autarch", "followthrough"]) fd = this.directory(fd, part);
      this.root = fd; this.requests = this.directory(fd, "requests"); this.receipts = this.directory(fd, "receipts");
      this.lock = this.file(this.root, "consumer.lock", C.O_RDWR | C.O_CREAT);
      const result = spawnSync("flock", ["--exclusive", "--nonblock", "3"], { stdio: ["ignore", "ignore", "ignore", this.lock] });
      if (result.error || result.status !== 0) throw Error("consumer locked or flock unavailable");
      this.active = true;
    } catch { this.close(); }
  }
  private path(fd: number, name: string) { if (!/^[a-zA-Z0-9_.-]+$/.test(name)) throw Error("unsafe name"); return `/proc/self/fd/${fd}/${name}`; }
  private keep(fd: number) { this.dirs.push(fd); return fd; }
  private privateStat(fd: number, directory: boolean) {
    const st = fstatSync(fd);
    if (st.uid !== this.uid || (st.mode & 0o7777) !== (directory ? 0o700 : 0o600)
      || !(directory ? st.isDirectory() : st.isFile()) || (!directory && st.nlink !== 1)) throw Error("unsafe spool metadata");
    return st;
  }
  private directory(parent: number, name: string) {
    try { mkdirSync(this.path(parent, name), { mode: 0o700 }); } catch (e) { if ((e as NodeJS.ErrnoException).code !== "EEXIST") throw e; }
    const fd = this.keep(openSync(this.path(parent, name), C.O_RDONLY | C.O_DIRECTORY | C.O_NOFOLLOW));
    this.privateStat(fd, true); return fd;
  }
  private file(parent: number, name: string, flags = C.O_RDONLY) {
    const fd = openSync(this.path(parent, name), flags | C.O_NOFOLLOW | C.O_NONBLOCK, 0o600);
    try { this.privateStat(fd, false); return fd; } catch (e) { closeSync(fd); throw e; }
  }
  private read(parent: number, name: string): unknown {
    const fd = this.file(parent, name);
    try {
      if (this.privateStat(fd, false).size > MAX_BYTES) throw Error("oversize spool input");
      const buf = Buffer.alloc(MAX_BYTES + 1); let n = 0, got: number;
      while (n < buf.length && (got = readSync(fd, buf, n, buf.length - n, null)) > 0) n += got;
      if (n > MAX_BYTES) throw Error("oversize spool input");
      return JSON.parse(buf.subarray(0, n).toString("utf8"));
    } finally { closeSync(fd); }
  }
  private write(parent: number, name: string, value: unknown, exclusive = false) {
    const temp = exclusive ? name : `.tmp-${randomUUID()}`;
    const fd = this.file(parent, temp, C.O_WRONLY | C.O_CREAT | C.O_EXCL);
    try { const bytes = Buffer.from(JSON.stringify(value)); let n = 0; while (n < bytes.length) n += writeSync(fd, bytes, n, bytes.length - n); fsyncSync(fd); }
    finally { closeSync(fd); }
    if (!exclusive) renameSync(this.path(parent, temp), this.path(parent, name));
    fsyncSync(parent);
  }
  private matching(p: ReminderPick | null, r: ReminderRequest): p is ReminderPick {
    return !!p && p.eligible && !p.protected && p.pickId === r.pick_id && p.generation === r.generation && p.revision === r.revision;
  }
  private async proof(r: ReminderRequest): Promise<{ pick: ReminderPick; target: string } | null> {
    const p = this.s.readPick(r.pick_id);
    if (!this.matching(p, r)) return null;
    const rows = await this.s.listThreads();
    const target = await resolveReminderTarget(p.owner, async () => rows);
    if (!target) return null;
    const owner = rows.find(t => t.id === p.owner);
    if (p.cancelled && (!owner || owner.archivedAt === null && owner.deletedAt === null)) return null;
    if (r.reason === "confirmed-nondelivery") {
      if (p.delivery !== "none-confirmed" || !owner || (owner.archivedAt === null && owner.deletedAt === null)) return null;
    } else if (p.delivery !== "confirmed") return null;
    const fresh = this.s.readPick(r.pick_id);
    if (!this.matching(fresh, r) || JSON.stringify(fresh) !== JSON.stringify(p)) return null;
    return { pick: fresh, target };
  }
  async tick(): Promise<void> {
    if (!this.active || this.busy) return;
    this.busy = true;
    try {
      for (const fd of [this.root, this.requests, this.receipts]) this.privateStat(fd, true);
      // Producer handshake from the owner-delivery plan: private readiness.json beside
      // requests/receipts. Design choice: 45 s validity for a 15 s poll. A stopped,
      // unlocked or unsafe consumer cannot refresh it; producers must fail closed on expiry.
      try { this.read(this.root, "readiness.json"); } catch (e) { if (!absent(e)) throw e; }
      const readyAt = this.s.now();
      this.write(this.root, "readiness.json", { version: 1, uid: this.uid,
        ready_at: new Date(readyAt).toISOString(), expires_at: new Date(readyAt + 45000).toISOString() });
      // Bound work per tick; a malformed item cannot authorize a send or starve another item.
      for (const name of readdirSync(`/proc/self/fd/${this.requests}`).slice(0, 1000)) {
        if (!/^[a-f0-9]{64}\.json$/.test(name)) continue;
        try { await this.consume(name); } catch { /* fail closed; no sensitive exception text in logs */ }
      }
    } catch { /* unsafe directory: report mode until its metadata is repaired */ }
    finally { this.busy = false; }
  }
  private async consume(name: string) {
    const r = parseReminderRequest(this.read(this.requests, name), this.s.now());
    if (!r || name !== `${reminderActionId(r)}.json`) return;
    const action = reminderActionId(r);
    const journalName = `generation-${hash(JSON.stringify([r.pick_id, r.generation]))}.json`;
    // This non-prunable journal is independent of daily requests/receipts. Never delete it
    // while the generation can still qualify. Missing daily history does not reset it.
    let entries: Receipt[] = [];
    try { entries = journalSchema.parse(this.read(this.receipts, journalName)).entries; } catch (e) { if (!absent(e)) throw e; }
    const putJournal = () => this.write(this.receipts, journalName, { version: 1, entries });
    // Recover a claim/result that was synced before its journal update (process death).
    for (const file of readdirSync(`/proc/self/fd/${this.receipts}`)) {
      if (!/^[a-f0-9]{64}\.json$/.test(file)) continue;
      const receipt = receiptSchema.parse(this.read(this.receipts, file));
      if (`${receipt.action}.json` !== file) throw Error("receipt identity conflict");
      if (receipt.pick_id === r.pick_id && receipt.generation === r.generation && receipt.attempted && !entries.some(e => e.action === receipt.action)) entries.push(receipt);
    }
    if (entries.length > 3 || entries.some(e => e.pick_id !== r.pick_id || e.generation !== r.generation)) throw Error("journal conflict");
    if (entries.length) putJournal();
    if (entries.some(e => e.action === action)) return;
    try { receiptSchema.parse(this.read(this.receipts, name)); return; } catch (e) { if (!absent(e)) throw e; }
    const base: Receipt = { action, pick_id: r.pick_id, generation: r.generation, pacific_date: r.pacific_date, status: "no-route", attempted: false, target: null, input: "" };
    const terminal = (status: Receipt["status"]) => this.write(this.receipts, name, { ...base, status }, true);
    if (Date.parse(r.expires_at) <= this.s.now()) { terminal("expired"); return; }
    if (entries.length >= 3) { terminal("capped"); return; }
    // Only positive exact-text host evidence settles an ambiguous claim. Absence is unknown.
    for (const prior of entries.filter(e => e.status === "unknown" || e.status === "claimed")) {
      if (!prior.target || !prior.input) return;
      const accepted = (await this.s.sdk.acceptedEvents(prior.target)).some(e => !!e.id && e.text === prior.input);
      const queued = (await this.s.sdk.queuedList(prior.target)).some(e => !!e.id && e.text === prior.input);
      if (!accepted && !queued) return;
      prior.status = accepted ? "accepted" : "queued"; putJournal();
    }
    const proof = await this.proof(r);
    if (!proof) { terminal("no-route"); return; }
    // A second fresh route read immediately before claiming/dispatch catches rotations during reconciliation.
    const last = await this.proof(r);
    if (!last || last.target !== proof.target || Date.parse(r.expires_at) <= this.s.now() || pacificDate(this.s.now()) !== r.pacific_date) return;
    for (const fd of [this.root, this.requests, this.receipts]) this.privateStat(fd, true);
    if (JSON.stringify(parseReminderRequest(this.read(this.requests, name), this.s.now())) !== JSON.stringify(r)) return;
    const input = `[followthrough ${action}] Advisory: selected option ${JSON.stringify(last.pick.option)} still needs outcome review. Read the authenticated action: bb home pick-action --pick ${r.pick_id}`;
    if (input.length > 2048) { terminal("no-route"); return; }
    const receipt: Receipt = { ...base, status: "claimed", attempted: true, target: last.target, input };
    this.write(this.receipts, name, receipt, true); // exclusive claim, fsync file AND directory before any SDK call
    entries.push(receipt); putJournal();
    try {
      const sent = await this.s.sdk.send({ threadId: last.target, input, mode: "queue-if-active", wakeCause: "retry", wakeRef: `followthrough:${action}` });
      receipt.status = sent.delivery === "queued" ? "queued" : sent.delivery === "sent" && sent.messageId ? "accepted" : "unknown";
    } catch (e) {
      // Only an affirmative transport refusal is definitive; timeouts and all other exceptions are ambiguous.
      receipt.status = (e as NodeJS.ErrnoException)?.code === "ECONNREFUSED" ? "rejected" : "unknown";
    }
    this.write(this.receipts, name, receipt); putJournal();
  }
  close() {
    this.active = false;
    if (this.lock >= 0) { closeSync(this.lock); this.lock = -1; }
    for (const fd of this.dirs.reverse()) closeSync(fd);
    this.dirs = [];
  }
}

interface ReminderHost {
  readonly experimental_dataDir?: unknown;
  background: { service: (name: string, definition: { start: (signal: AbortSignal) => Promise<void> }) => unknown };
}
/** Getter and value are both guarded before registration. No alternate path or send transport. */
export function registerReminderService(host: ReminderHost, sources: () => ReminderSources | null): void {
  const dataDir = reminderDataDir(host);
  if (!dataDir) return;
  host.background.service("home-followthrough-reminders", {
    async start(signal) {
      let consumer: ReminderConsumer | null = null;
      try {
        while (!signal.aborted) {
          const s = sources();
          if (!consumer && s) consumer = new ReminderConsumer(dataDir, s);
          if (consumer) await consumer.tick();
          if (signal.aborted) break;
          await new Promise<void>(resolve => {
            const done = () => { clearTimeout(timer); signal.removeEventListener("abort", done); resolve(); };
            const timer = setTimeout(done, 15000);
            signal.addEventListener("abort", done, { once: true });
          });
        }
      } finally { consumer?.close(); }
    },
  });
}
