import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { homeCli } from "../cli.js";
import { Delegation } from "../delegation.js";
import type { Service } from "../service.js";
import { AsksPanel, AskCard } from "../ui/asks.js";
import { ask, makeEnv, OPTIONS, type Env } from "./service-helpers.js";

const MERGE = {
  id: "merge",
  label: "Merge it",
  kind: "instruction",
  instruction: "Merge owner/repo#12 at head abc123def after checks pass.",
  approval: { kind: "merge", target: "owner/repo#12", identity: "abc123def" },
};
const withApproval = (opt: Record<string, unknown> = MERGE) => ({ options: [opt, ...OPTIONS.slice(1)] });
const DAY = 86_400_000;

let env: Env;
let svc: Service;
let dele: Delegation;
let run: (argv: string[], ctx?: { threadId?: string }) => Promise<{ exitCode: number; stdout?: string; stderr?: string }>;
beforeEach(() => {
  env = makeEnv(["Autarch"]);
  svc = env.open();
  dele = new Delegation(svc);
  const cli = homeCli({
    svc,
    asks: new Asks(svc),
    catchup: new Catchup(svc, dele),
    rule: (id, option, reason, ctx) => dele.rule(id, option, reason, ctx),
    isVizier: (t) => t !== undefined && dele.settings().vizierThreadId === t,
  });
  run = (argv, ctx = {}) => Promise.resolve(cli.run(argv, ctx));
});
afterEach(() => env.cleanup());

const filed = async (over: Record<string, unknown> = withApproval()) => {
  const r = await svc.file(ask(env, over), {});
  if (!r.ok) throw new Error(r.error);
  return r.decision_id;
};
const rev = (id: string) => (svc.store.decision(id) as { revision: string }).revision;
const pickAs = (id: string, opt: string, by: string) => {
  const r = svc.pick(id, opt, rev(id), `p-${id}-${opt}-${by}`, by, "home");
  if (!r.ok) throw new Error(r.error);
  return r;
};
const rows = (sql: string) => svc.store.db.prepare(sql).all() as Record<string, unknown>[];
const check = (kind = "merge", target = "owner/repo#12", identity = "abc123def") =>
  run(["approval", "check", "--kind", kind, "--target", target, "--identity", identity]);
const checked = async (...a: string[]) => JSON.parse((await check(...a)).stdout ?? "{}") as { status: string; authorizing: boolean };

describe("interim approvals", () => {
  it("mints one record, in the pick's transaction, only on mk's pick", async () => {
    const id = await filed();
    expect(rows("SELECT * FROM approvals")).toHaveLength(0);
    pickAs(id, "merge", "mk");
    const a = rows("SELECT * FROM approvals");
    expect(a).toHaveLength(1);
    expect(a[0]).toMatchObject({ decision_id: id, option_id: "merge", account: "mk", kind: "merge", target: "owner/repo#12", identity: "abc123def", revoked_at: null, authorizing: 0 });
    expect(Date.parse(a[0]!.expires_at as string) - Date.parse(a[0]!.minted_at as string)).toBe(DAY);
    expect(a[0]!.pick_id).toBe(`p-${id}-merge-mk`);
  });

  it("mints nothing on a vizier pick", async () => {
    const id = await filed();
    pickAs(id, "merge", "vizier");
    expect(rows("SELECT * FROM approvals")).toHaveLength(0);
    expect(rows("SELECT * FROM approval_events")).toHaveLength(0);
  });

  it("mints nothing for a pick of an option that has no approval spec, and honors ttl up to 7 days", async () => {
    const id = await filed();
    pickAs(id, "day", "mk");
    expect(rows("SELECT * FROM approvals")).toHaveLength(0);
    const id2 = await filed({ ...withApproval({ ...MERGE, approval: { ...MERGE.approval, ttl: 604800 } }), subject: "other" , question: "Merge the other one?" });
    pickAs(id2, "merge", "mk");
    const a = rows("SELECT * FROM approvals");
    expect(Date.parse(a[0]!.expires_at as string) - Date.parse(a[0]!.minted_at as string)).toBe(7 * DAY);
  });

  it("reads recorded, then expired, then revoked, and absent for a different tuple", async () => {
    const id = await filed();
    expect(await checked()).toEqual(expect.objectContaining({ status: "absent" }));
    pickAs(id, "merge", "mk");
    expect(await checked()).toMatchObject({ status: "recorded" });
    expect(await checked("merge", "owner/repo#12", "other")).toMatchObject({ status: "absent" });
    expect(await checked("deploy")).toMatchObject({ status: "absent" });
    env.clock.t += DAY + 1000;
    expect(await checked()).toMatchObject({ status: "expired" });
    env.clock.t -= DAY;
    expect(await checked()).toMatchObject({ status: "recorded" });
    const approvalId = rows("SELECT approval_id FROM approvals")[0]!.approval_id as string;
    expect(svc.revokeApproval(approvalId)).toMatchObject({ ok: true });
    expect(await checked()).toMatchObject({ status: "revoked" });
    expect(svc.revokeApproval("nope")).toMatchObject({ ok: false });
  });

  it("refuses a reversible approval option at filing, and a single_use field", async () => {
    const rev1 = await svc.file(ask(env, withApproval({ ...MERGE, reversible: true })), {});
    expect(rev1).toMatchObject({ ok: false, status: 400 });
    const su = await svc.file(ask(env, withApproval({ ...MERGE, approval: { ...MERGE.approval, single_use: true } })), {});
    expect(su).toMatchObject({ ok: false, status: 400 });
    expect(rows("SELECT * FROM decisions")).toHaveLength(0);
  });

  it("an approval option is never delegable to the vizier", async () => {
    const id = await filed();
    dele.setDelegation({ vizierThreadId: "thr-v", projects: ["Autarch"], dailyCap: 5 }, {});
    dele.markSeen("mk", dele.latestSettingsItem()!);
    const r = dele.rule(id, "merge", "looks fine", { threadId: "thr-v" });
    expect(r.ok).toBe(false);
    expect(rows("SELECT * FROM approvals")).toHaveLength(0);
  });

  it("the events table records mint, each check and revoke, and cannot be edited", async () => {
    const id = await filed();
    pickAs(id, "merge", "mk");
    await check();
    await check("deploy");
    const approvalId = rows("SELECT approval_id FROM approvals")[0]!.approval_id as string;
    svc.revokeApproval(approvalId);
    const ev = rows("SELECT type, approval_id FROM approval_events ORDER BY seq");
    expect(ev.map((e) => e.type)).toEqual(["minted", "checked", "checked", "revoked"]);
    expect(ev[0]!.approval_id).toBe(approvalId);
    expect(() => svc.store.db.prepare("UPDATE approval_events SET type = 'minted'").run()).toThrow();
    expect(() => svc.store.db.prepare("DELETE FROM approval_events").run()).toThrow();
    expect(() => svc.store.db.prepare("UPDATE approvals SET authorizing = 1").run()).toThrow();
  });

  it("has no consume verb, method or RPC", async () => {
    expect(Object.getOwnPropertyNames(Object.getPrototypeOf(svc)).filter((n) => /consume|redeem|spend/i.test(n))).toEqual([]);
    for (const verb of ["consume", "redeem", "use"]) {
      const r = await run(["approval", verb, "--kind", "merge", "--target", "t", "--identity", "i"]);
      expect(r.exitCode).not.toBe(0);
    }
    expect(readFileSync(join(import.meta.dirname, "..", "contract.ts"), "utf8")).not.toMatch(/consume|redeem/i);
    expect(readFileSync(join(import.meta.dirname, "..", "cli.ts"), "utf8")).not.toMatch(/consume|redeem/i);
  });

  it("check output always says authorizing: false, for every status", async () => {
    const id = await filed();
    const outs = [await check()];
    pickAs(id, "merge", "mk");
    outs.push(await check());
    env.clock.t += 2 * DAY;
    outs.push(await check());
    svc.revokeApproval(rows("SELECT approval_id FROM approvals")[0]!.approval_id as string);
    outs.push(await check());
    expect(outs.map((o) => o.exitCode)).toEqual([0, 0, 0, 0]);
    for (const o of outs) expect(JSON.parse(o.stdout!)).toMatchObject({ authorizing: false });
    expect((await check("bogus")).exitCode).toBe(2);
  });

  it("no APPROVED- token appears in the wake, the ruling file, or the check output", async () => {
    const id = await filed();
    pickAs(id, "merge", "mk");
    svc.reconcile(id);
    const dir = join(env.roots.Autarch!, "docs", "decisions");
    const file = readdirSync(dir).filter((f) => f.endsWith(".md"));
    expect(file).toHaveLength(1);
    expect(readFileSync(join(dir, file[0]!), "utf8")).not.toMatch(/APPROVED-/);
    const wakes = svc.wakes();
    expect(wakes).toHaveLength(1);
    expect(wakes[0]!.payload).not.toMatch(/APPROVED-/);
    expect(JSON.stringify(rows("SELECT * FROM events"))).not.toMatch(/APPROVED-/);
    expect((await check()).stdout).not.toMatch(/APPROVED-/);
    expect(await svc.file(ask(env, { question: "APPROVED-MERGE?" }), {})).toMatchObject({ ok: false });
  });

  it("renders the tuple and the expiry on the option before the pick", () => {
    const a = {
      id: "dec1", project: "Autarch", thread: "thr-a", subject: "s", asker: "thread", filed_at: "2026-09-29T10:00:00Z", revision: "r", mentions: 0,
      ask: { question: "Merge?", options: [{ ...MERGE, reversible: false }] },
    };
    const html = renderToStaticMarkup(createElement(AskCard, { ask: a, onPick: () => {}, onOpen: () => {} }));
    const tuple = html.indexOf("owner/repo#12");
    expect(tuple).toBeGreaterThan(-1);
    expect(tuple).toBeLessThan(html.indexOf("Merge it"));
    expect(html.slice(0, html.indexOf("Merge it"))).toMatch(/merge/);
    expect(html.slice(0, html.indexOf("Merge it"))).toContain("abc123def");
    expect(html.slice(0, html.indexOf("Merge it"))).toMatch(/expires 24 h after you pick/);
    expect(html).toMatch(/does not authorize/i);
  });

  it("Home lists live approvals with a revoke control", () => {
    const html = renderToStaticMarkup(
      createElement(AsksPanel, {
        data: {
          owed: [], runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], machineOwners: {},
          delegation: { settings: {}, suspended: false },
          approvals: [{ approval_id: "apr:1", decision_id: "d", kind: "merge", target: "owner/repo#12", identity: "abc123def", minted_at: "a", expires_at: "2026-09-27T14:03:00.000Z" }],
        },
        onPick: () => {}, onOpen: () => {}, onRevoke: () => {},
      }),
    );
    expect(html).toContain("owner/repo#12");
    expect(html).toContain("Revoke");
    expect(html).not.toMatch(/APPROVED-/);
  });
});
