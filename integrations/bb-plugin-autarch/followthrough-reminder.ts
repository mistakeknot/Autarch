// Same-user, primary-host filesystem boundary. No Home writes or ordinary wake-loop participation.
import { constants as C, openSync, closeSync, fstatSync, mkdirSync, readdirSync, readSync, writeSync, fsyncSync, renameSync, lstatSync, unlinkSync, rmdirSync, readFileSync, type Stats } from "node:fs";
import { isAbsolute } from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { z } from "zod";
import { resolveReminderTarget, type ReminderThreadRow } from "./followthrough-target.js";
import type { WakeSdk } from "./wakes.js";
const hash = (s: string) => createHash("sha256").update(s).digest("hex");
// Design choice: allow an interrupted exclusive lock creation 60 seconds to finish.
const LOCK_INIT_GRACE_MS = 60_000;
// Measure the cleanup age from the directory mtime set when owner.json was
// written. Retirement preserves that time. The stage-directory guard protects
// tombstones a delayed contender could still need.
const TAKEOVER_CLEANUP_AGE_MS = 86_400_000;
const TAKEOVER_GATE = ".consumer-takeover";
const TAKEOVER_STAGE = /^\.consumer-takeover-[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/;
const TAKEOVER_RETIRED = /^\.consumer-takeover-retired-[a-f0-9]{64}$/;
const holderKey = Symbol.for("autarch.followthrough.lock-holders");
const failedGateKey = Symbol.for("autarch.followthrough.failed-takeover-releases");
const processState = globalThis as typeof globalThis & { [key: symbol]: Map<string, string> | undefined };
const heldLocks = processState[holderKey] ??= new Map<string, string>();
const failedGateReleases = processState[failedGateKey] ??= new Map<string, string>();
class LockRefusal extends Error {
  constructor(readonly state: string) { super(state); }
}
/** Kernel-owned, read-only metadata at fixed proc paths, never caller-supplied paths.
 * Spool files still use bounded O_NOFOLLOW descriptor reads. Boot plus start ticks distinguishes PID reuse,
 * including reboots. A matching PID alone never proves another process is a consumer.
 * Same-process holders additionally require the shared registry's exact token; this
 * registry survives module reloads. Legacy PID-only locks age out after the grace.
 */
function kernelProcessIdentity(pid: number): string | null {
  try {
    const stat = readFileSync(`/proc/${pid}/stat`, { encoding: "utf8" });
    const start = stat.slice(stat.lastIndexOf(")") + 2).trim().split(/\s+/)[19];
    const boot = readFileSync("/proc/sys/kernel/random/boot_id", { encoding: "utf8" }).trim();
    if (!start || !/^\d+$/.test(start) || !boot) throw new LockRefusal("lock-owner-unavailable");
    return `${boot}:${start}`;
  } catch (e) {
    if (["ENOENT", "ESRCH"].includes((e as NodeJS.ErrnoException).code ?? "")) return null;
    throw new LockRefusal("lock-owner-unavailable");
  }
}
const MAX_BYTES = 16384;
const lockOwnerSchema = z.object({ pid: z.number().int().positive() });
const lockSchema = lockOwnerSchema.extend({ token: z.string(), start: z.string().optional() }).strict();
type TakeoverOwner = { pid: number; token: string; start: string };
type LockSnapshot = { text: string; stat: Stats };
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
export function reminderDataDir(host: { readonly server?: { readonly experimental_dataDir?: unknown } }): string | null {
  try { const d = host.server?.experimental_dataDir; return typeof d === "string" && d.trim() !== "" && isAbsolute(d) ? d : null; } catch { return null; }
}
export interface ReminderPick {
  pickId: string; generation: number; revision: string; option: string; owner: string | null;
  eligible: boolean; protected: boolean; delivery: "confirmed" | "none-confirmed" | "unknown";
  cancelled: boolean;
}
export interface ReminderSources {
  readPick: (id: string) => ReminderPick | null;
  // Null means the bounded read cannot prove a route.
  listThreads: (owner: string) => Promise<ReminderThreadRow[] | null>;
  sdk: WakeSdk;
  now: () => number;
  processIdentity?: (pid: number) => string | null;
}
const receiptSchema = z.object({
  action: z.string().regex(/^[a-f0-9]{64}$/), pick_id: z.string(), generation: z.number().int().positive(), pacific_date: z.string(),
  status: z.enum(["claimed", "accepted", "queued", "rejected", "unknown", "capped", "day-used", "expired", "no-route"]),
  resolved_date: z.string().optional(), handle: z.string().optional(), attempted: z.boolean(), target: z.string().nullable(), input: z.string(),
}).strict();
type Receipt = z.infer<typeof receiptSchema>;
const journalSchema = z.object({ version: z.literal(1), entries: z.array(receiptSchema).max(3) }).strict();
const absent = (e: unknown) => (e as NodeJS.ErrnoException).code === "ENOENT";
function acceptedDate(createdAt: number | undefined): string | undefined {
  return typeof createdAt === "number" && Number.isFinite(new Date(createdAt).getTime()) ? pacificDate(createdAt) : undefined;
}
/** Linux pinned directory descriptors: every untrusted component is opened O_NOFOLLOW.
 * /proc/self/fd refers to our already-open descriptor, never to a caller-provided symlink.
 * Lock snapshots are checked for liveness and stale takeovers serialize through a directory rename.
 */
export class ReminderConsumer {
  private dirs: number[] = [];
  private spoolDirs = new Set<number>();
  private root = -1;
  private requests = -1;
  private receipts = -1;
  private lock = -1;
  private lockKey = "";
  private lockToken: string | null = null;
  state = "setup-unavailable";
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
      // Design choice: experimental_dataDir itself is never a write destination.
      // Only our plugin's own directory may hold the followthrough spool, beside data.db.
      // These two host-created ancestors may be 0755; ownership and O_NOFOLLOW suffice.
      const pluginDir = this.ancestor(this.ancestor(fd, "plugins"), "autarch");
      try { mkdirSync(this.path(pluginDir, "followthrough"), { mode: 0o700 }); } catch (e) { if ((e as NodeJS.ErrnoException).code !== "EEXIST") throw e; }
      this.root = this.keep(openSync(this.path(pluginDir, "followthrough"), C.O_RDONLY | C.O_DIRECTORY | C.O_NOFOLLOW));
      this.privateStat(this.root, true); this.spoolDirs.add(this.root);
      this.requests = this.directory(this.root, "requests"); this.receipts = this.directory(this.root, "receipts");
      const rootStat = fstatSync(this.root); this.lockKey = `${rootStat.dev}:${rootStat.ino}`;
      this.acquireLock();
      // Consumer temporaries exist only here and in receipts/. Never clean requests/:
      // the producer writes an exclusive 0600 .request-tmp-<uuid> of at most 16384 bytes,
      // fsyncs the file, publishes it atomically as <action-id>.json and fsyncs requests/.
      // The action ID in the filename must match the request content. The producer
      // must never replace an existing request and removes its own stale .request-tmp-
      // files. Legacy .tmp-* files are producer-owned or ambiguous.
      // A receipt with attempted:false is never a claim, regardless of status.
      for (const parent of [this.root, this.receipts]) {
        for (const name of readdirSync(`/proc/self/fd/${parent}`)) {
          if (/^\.consumer-tmp-[a-f0-9-]{36}$/.test(name)) {
            try { this.remove(parent, name); } catch { /* unsafe strays are ignored, never chmodded or followed */ }
          }
        }
      }
      this.cleanTakeoverLeftovers();
      this.active = true; this.state = "ready";
    } catch (e) { this.state = e instanceof LockRefusal ? e.state : "setup-unavailable"; this.close(); }
  }
  private path(fd: number, name: string) { if (!/^[a-zA-Z0-9_.-]+$/.test(name)) throw Error("unsafe name"); return `/proc/self/fd/${fd}/${name}`; }
  private spoolPath(fd: number, name: string) {
    if (!this.spoolDirs.has(fd)) throw Error("outside followthrough spool");
    this.privateStat(fd, true); return this.path(fd, name);
  }
  private ancestor(parent: number, name: string) {
    const fd = this.keep(openSync(this.path(parent, name), C.O_RDONLY | C.O_DIRECTORY | C.O_NOFOLLOW));
    if (fstatSync(fd).uid !== this.uid) throw Error("unsafe ancestor owner");
    return fd;
  }
  private acquireLock() {
    let snapshot: LockSnapshot | null = null;
    try { snapshot = this.readText(this.root, "consumer.lock", true); } catch (e) { if (!absent(e)) throw e; }
    const identity = this.s.processIdentity ?? kernelProcessIdentity;
    if (snapshot) {
      let value: unknown;
      try { if (snapshot.stat.size <= MAX_BYTES) value = JSON.parse(snapshot.text); } catch { /* incomplete write */ }
      const old = lockSchema.safeParse(value);
      const young = this.s.now() - snapshot.stat.mtimeMs <= LOCK_INIT_GRACE_MS;
      if (!old.success) {
        if (value !== undefined) {
          // A newer schema may name a live owner we cannot otherwise identify.
          const owner = lockOwnerSchema.safeParse(value);
          // Design choice: a valid JSON lock without a positive PID never ages out.
          // To clear it, stop all consumers and verify the spool is idle before
          // removing the damaged consumer.lock. Leave requests and receipts intact.
          if (!owner.success) throw new LockRefusal("lock-owner-unavailable");
          if (identity(owner.data.pid)) throw new LockRefusal("lock-held");
        }
        if (young) throw new LockRefusal("lock-initializing");
      } else if (old.data.pid === process.pid) {
        if (heldLocks.get(this.lockKey) === old.data.token) throw new LockRefusal("lock-held");
      } else {
        let current: string | null;
        try { current = identity(old.data.pid); }
        catch (e) {
          // A legacy PID-only file cannot attest ownership, even on EPERM. Give it
          // the initialization grace; valid new-format metadata errors refuse visibly.
          if (old.data.start || young) throw new LockRefusal("lock-owner-unavailable");
          current = null;
        }
        if (old.data.start && current === old.data.start) throw new LockRefusal("lock-held");
        if (!old.data.start && current && young) throw new LockRefusal("lock-initializing");
      }
    }
    const start = identity(process.pid);
    if (!start) throw new LockRefusal("lock-owner-unavailable");
    const token = randomUUID();
    const owner = { pid: process.pid, token, start };
    const gate = this.readTakeoverGate();
    const retryRelease = gate && this.failedGateRelease(gate.owner);
    if (!retryRelease) failedGateReleases.delete(this.lockKey);
    if (snapshot || retryRelease) this.takeOverLock(snapshot, owner, identity);
    this.write(this.root, "consumer.lock", owner, true);
    this.lock = this.file(this.root, "consumer.lock"); this.lockToken = token;
    heldLocks.set(this.lockKey, token);
  }
  private failedGateRelease(owner: TakeoverOwner) {
    return owner.pid === process.pid && failedGateReleases.get(this.lockKey) === owner.token;
  }
  private readTakeoverGate(): { fd: number; owner: TakeoverOwner } | null {
    // Fail closed on a damaged gate even if consumer.lock is absent. Stop all
    // consumers using the spool and verify no takeover is running, then remove
    // only <dataDir>/plugins/autarch/followthrough/.consumer-takeover while they
    // remain stopped. Restart the consumers after clearing that entry.
    let fd: number;
    try { fd = this.openDirectory(this.root, TAKEOVER_GATE); }
    catch (e) {
      if (absent(e)) return null;
      throw new LockRefusal("takeover-gate-invalid");
    }
    try {
      const owner = lockSchema.parse(this.read(fd, "owner.json"));
      if (!owner.start) throw Error("missing takeover start");
      return { fd, owner: { ...owner, start: owner.start } };
    } catch { throw new LockRefusal("takeover-gate-invalid"); }
  }
  private takeOverLock(snapshot: LockSnapshot | null, owner: TakeoverOwner, identity: (pid: number) => string | null) {
    const parent = this.root;
    const gate = TAKEOVER_GATE;
    const temp = `${gate}-${owner.token}`;
    const stage = this.directory(parent, temp);
    let published = false;
    try {
      this.write(stage, "owner.json", owner, true);
      // Stage before reading the incumbent so cleanup can see every contender
      // that might still be about to rename a gate into a tombstone.
      let incumbent = this.readTakeoverGate();
      if (!incumbent) {
        try { this.move(parent, temp, gate); }
        catch (e) {
          if (!["EEXIST", "ENOTEMPTY"].includes((e as NodeJS.ErrnoException).code ?? "")) throw e;
          incumbent = this.readTakeoverGate();
          if (!incumbent) throw e;
        }
      }
      if (incumbent) {
        const old = incumbent.owner, retryRelease = this.failedGateRelease(old);
        if (identity(old.pid) === old.start && !retryRelease) throw new LockRefusal("lock-contended");
        if (!this.sameFile(parent, gate, incumbent.fd)) throw new LockRefusal("lock-changed");
        // POSIX rename cannot overwrite a nonempty directory. Keep this
        // tombstone so a delayed rival for the same gate cannot move its
        // live replacement there, even after the winner releases the gate.
        this.move(parent, gate, `${gate}-retired-${hash(old.token)}`);
        if (retryRelease) failedGateReleases.delete(this.lockKey);
        this.move(parent, temp, gate);
      }
      published = true;
      if (snapshot) this.remove(parent, "consumer.lock", snapshot.stat, snapshot.text);
    } finally {
      // Move the gate out of the way while it is still nonempty.
      if (published) {
        try { this.move(parent, gate, temp); }
        catch {
          // Record the exact failed release for the next run, even if the stale
          // consumer.lock is gone. The service logs the receipt-free refusal.
          failedGateReleases.set(this.lockKey, owner.token);
          throw new LockRefusal("takeover-gate-release-failed");
        }
      }
      try { this.remove(stage, "owner.json"); } catch (e) { if (!absent(e)) throw e; }
      rmdirSync(this.spoolPath(parent, temp));
    }
  }
  private cleanTakeoverLeftovers() {
    const parent = this.root;
    const names = readdirSync(`/proc/self/fd/${parent}`);
    for (const name of names) if (TAKEOVER_STAGE.test(name)) this.cleanTakeoverDirectory(parent, name, true);
    // A contender stages before reading the gate. Keep tombstones while any
    // staging directory could still be in use by a delayed rival.
    if (readdirSync(`/proc/self/fd/${parent}`, { withFileTypes: true }).some(e => e.isDirectory() && TAKEOVER_STAGE.test(e.name))) return;
    for (const name of names) if (TAKEOVER_RETIRED.test(name)) this.cleanTakeoverDirectory(parent, name, false);
  }
  private cleanTakeoverDirectory(parent: number, name: string, staged: boolean) {
    let fd = -1;
    try {
      fd = openSync(this.spoolPath(parent, name), C.O_RDONLY | C.O_DIRECTORY | C.O_NOFOLLOW);
      const stat = this.privateStat(fd, true);
      if (this.s.now() - stat.mtimeMs <= TAKEOVER_CLEANUP_AGE_MS) return;
      const files = readdirSync(`/proc/self/fd/${fd}`);
      if (files.some(file => file !== "owner.json") || !this.sameFile(parent, name, fd)) return;
      this.spoolDirs.add(fd);
      if (files.length) {
        if (staged) {
          const owner = lockSchema.parse(this.read(fd, "owner.json"));
          if (!owner.start || (this.s.processIdentity ?? kernelProcessIdentity)(owner.pid) === owner.start) return;
        }
        this.remove(fd, "owner.json");
      }
      if (!this.sameFile(parent, name, fd)) return;
      const temp = name;
      rmdirSync(this.spoolPath(parent, temp)); fsyncSync(parent);
    } catch { /* Keep unknown, unsafe or concurrently changed leftovers. */ }
    finally { if (fd >= 0) { this.spoolDirs.delete(fd); closeSync(fd); } }
  }
  private sameFile(parent: number, name: string, fd: number): boolean {
    const a = fstatSync(fd), b = lstatSync(this.spoolPath(parent, name));
    return a.dev === b.dev && a.ino === b.ino;
  }
  private remove(parent: number, name: string, expected?: Stats, expectedText?: string) {
    const fd = this.file(parent, name);
    try {
      const found = fstatSync(fd);
      if (expected && (found.dev !== expected.dev || found.ino !== expected.ino)) throw new LockRefusal("lock-changed");
      // Also compare the captured content: a freed inode can be reused between
      // reading a stale lock and opening the replacement. The open fd pins it now.
      if (expectedText !== undefined && this.readText(parent, name, true).text !== expectedText) throw new LockRefusal("lock-changed");
      if (!this.sameFile(parent, name, fd)) throw new LockRefusal("lock-changed");
      unlinkSync(this.spoolPath(parent, name)); fsyncSync(parent);
    } finally { closeSync(fd); }
  }
  private keep(fd: number) { this.dirs.push(fd); return fd; }
  private privateStat(fd: number, directory: boolean) {
    const st = fstatSync(fd);
    if (st.uid !== this.uid || (st.mode & 0o7777) !== (directory ? 0o700 : 0o600)
      || !(directory ? st.isDirectory() : st.isFile()) || (!directory && st.nlink !== 1)) throw Error("unsafe spool metadata");
    return st;
  }
  private directory(parent: number, name: string) {
    try { mkdirSync(this.spoolPath(parent, name), { mode: 0o700 }); } catch (e) { if ((e as NodeJS.ErrnoException).code !== "EEXIST") throw e; }
    return this.openDirectory(parent, name);
  }
  private openDirectory(parent: number, name: string) {
    const fd = this.keep(openSync(this.spoolPath(parent, name), C.O_RDONLY | C.O_DIRECTORY | C.O_NOFOLLOW));
    this.privateStat(fd, true); this.spoolDirs.add(fd); return fd;
  }
  private file(parent: number, name: string, flags = C.O_RDONLY) {
    const fd = openSync(this.spoolPath(parent, name), flags | C.O_NOFOLLOW | C.O_NONBLOCK, 0o600);
    try { this.privateStat(fd, false); return fd; } catch (e) { closeSync(fd); throw e; }
  }
  private readText(parent: number, name: string, lockSnapshot = false): { text: string; stat: Stats } {
    const fd = this.file(parent, name);
    try {
      const stat = this.privateStat(fd, false);
      if (stat.size > MAX_BYTES && !lockSnapshot) throw Error("oversize spool input");
      const buf = Buffer.alloc(MAX_BYTES + 1); let n = 0, got: number;
      while (n < buf.length && (got = readSync(fd, buf, n, buf.length - n, null)) > 0) n += got;
      if (n > MAX_BYTES && !lockSnapshot) throw Error("oversize spool input");
      return { text: buf.subarray(0, n).toString("utf8"), stat };
    } finally { closeSync(fd); }
  }
  private read(parent: number, name: string): unknown { return JSON.parse(this.readText(parent, name).text); }
  private write(parent: number, name: string, value: unknown, exclusive = false) {
    const temp = exclusive ? name : `.consumer-tmp-${randomUUID()}`;
    const fd = this.file(parent, temp, C.O_WRONLY | C.O_CREAT | C.O_EXCL);
    try { const bytes = Buffer.from(JSON.stringify(value)); let n = 0; while (n < bytes.length) n += writeSync(fd, bytes, n, bytes.length - n); fsyncSync(fd); }
    finally { closeSync(fd); }
    if (!exclusive) this.move(parent, temp, name);
    fsyncSync(parent);
  }
  private move(parent: number, temp: string, name: string) {
    renameSync(this.spoolPath(parent, temp), this.spoolPath(parent, name));
  }
  private matching(p: ReminderPick | null, r: ReminderRequest): p is ReminderPick {
    return !!p && p.eligible && !p.protected && p.pickId === r.pick_id && p.generation === r.generation && p.revision === r.revision;
  }
  private async proof(r: ReminderRequest): Promise<{ pick: ReminderPick; target: string } | "incomplete" | null> {
    const p = this.s.readPick(r.pick_id);
    if (!this.matching(p, r) || !p.owner) return null;
    const rows = await this.s.listThreads(p.owner);
    if (!rows) return "incomplete";
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
      if (!this.sameFile(this.root, "consumer.lock", this.lock)) { this.state = "lock-changed"; this.active = false; return; }
      for (const fd of [this.root, this.requests, this.receipts]) this.privateStat(fd, true);
      this.state = "ready";
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
    } catch { this.state = "spool-unavailable"; /* report mode until repaired */ }
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
    for (const prior of entries.filter(e => e.status === "unknown" || e.status === "claimed" || e.status === "queued")) {
      if (!prior.target || !prior.input) return;
      const accepted = (await this.s.sdk.acceptedEvents(prior.target)).find(e => !!e.id && e.text === prior.input);
      const queued = (await this.s.sdk.queuedList(prior.target)).some(e => !!e.id && e.text === prior.input);
      if (queued) { prior.status = "queued"; putJournal(); return; }
      // Design choice: delete/cancel is never permission to re-nag. The real
      // adapter has only queued/unknown state; missing history stays unresolved.
      const landed = acceptedDate(accepted?.createdAt);
      if (!landed) return;
      prior.status = "accepted"; prior.resolved_date = landed;
      putJournal();
    }
    // A prior-day send that landed today consumes today's slot as well.
    // Persist that observation in the non-prunable journal before refusing today.
    if (entries.some(e => e.resolved_date === r.pacific_date)) {
      terminal("day-used"); return;
    }
    const proof = await this.proof(r);
    if (!proof) { terminal("no-route"); return; }
    if (proof === "incomplete") { terminal("unknown"); return; }
    // A second fresh route read immediately before claiming/dispatch catches rotations during reconciliation.
    const last = await this.proof(r);
    if (last === "incomplete") { terminal("unknown"); return; }
    if (!last || last.target !== proof.target || Date.parse(r.expires_at) <= this.s.now() || pacificDate(this.s.now()) !== r.pacific_date) return;
    for (const fd of [this.root, this.requests, this.receipts]) this.privateStat(fd, true);
    if (JSON.stringify(parseReminderRequest(this.read(this.requests, name), this.s.now())) !== JSON.stringify(r)) return;
    const input = `[followthrough ${action}] Advisory: selected option ${JSON.stringify(last.pick.option)} still needs outcome review. Read the authenticated action: bb home pick-action --pick ${r.pick_id}`;
    if (input.length > 2048) { terminal("no-route"); return; }
    const receipt: Receipt = { ...base, status: "claimed", attempted: true, target: last.target, input };
    if (!this.sameFile(this.root, "consumer.lock", this.lock)) { this.state = "lock-changed"; this.active = false; return; }
    this.write(this.receipts, name, receipt, true); // exclusive claim, fsync file AND directory before any SDK call
    entries.push(receipt); putJournal();
    try {
      const sent = await this.s.sdk.send({ threadId: last.target, input, mode: "queue-if-active", wakeCause: "retry", wakeRef: `followthrough:${action}` });
      if (sent.delivery === "queued") receipt.handle = sent.queuedMessageId;
      receipt.status = sent.delivery === "queued" ? "queued" : sent.delivery === "sent" && sent.messageId ? "accepted" : "unknown";
    } catch (e) {
      // Only an affirmative transport refusal is definitive; timeouts and all other exceptions are ambiguous.
      receipt.status = (e as NodeJS.ErrnoException)?.code === "ECONNREFUSED" ? "rejected" : "unknown";
    }
    if (receipt.status === "accepted") {
      // An acknowledged send still uses the return date when host event
      // history is unavailable. Prefer the exact accepted turn when visible.
      const returnedDate = pacificDate(this.s.now());
      let landed: string | undefined;
      try {
        const accepted = (await this.s.sdk.acceptedEvents(last.target)).find(e => !!e.id && e.text === input);
        landed = acceptedDate(accepted?.createdAt);
      } catch { /* keep the acknowledged send on the conservative return date */ }
      receipt.resolved_date = landed ?? returnedDate;
    }
    this.write(this.receipts, name, receipt); putJournal();
  }
  close() {
    this.active = false;
    if (this.lock >= 0) {
      try { if (this.sameFile(this.root, "consumer.lock", this.lock)) this.remove(this.root, "consumer.lock", fstatSync(this.lock)); } catch { /* never remove another consumer's lock */ }
      if (heldLocks.get(this.lockKey) === this.lockToken) heldLocks.delete(this.lockKey);
      closeSync(this.lock); this.lock = -1;
    }
    for (const fd of this.dirs.reverse()) closeSync(fd);
    this.dirs = [];
  }
}

interface ReminderHost {
  readonly server?: { readonly experimental_dataDir?: unknown };
  log?: { info: (message: string) => unknown };
  background: { service: (name: string, definition: { start: (signal: AbortSignal) => Promise<void> }) => unknown };
}
/** Getter and value are both guarded before registration. No alternate path or send transport. */
export function registerReminderService(host: ReminderHost, sources: () => ReminderSources | null): void {
  const dataDir = reminderDataDir(host);
  if (!dataDir) {
    try { host.log?.info("followthrough reminders: report mode (server data directory unavailable)"); } catch { /* diagnostic only; host initialization must still succeed */ }
    return;
  }
  host.background.service("home-followthrough-reminders", {
    async start(signal) {
      let consumer: ReminderConsumer | null = null;
      let lastState: string | null = null;
      try {
        while (!signal.aborted) {
          const s = sources();
          if (consumer && !consumer.active) { consumer.close(); consumer = null; }
          if (!consumer && s) consumer = new ReminderConsumer(dataDir, s);
          if (consumer) await consumer.tick();
          const state = consumer?.state ?? "waiting-store";
          if (state !== lastState) {
            lastState = state;
            try { host.log?.info(`followthrough reminders: state=${state}`); } catch { /* diagnostic only */ }
          }
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
