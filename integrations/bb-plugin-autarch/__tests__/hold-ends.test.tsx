// Every new hold names its end (bead mk-j5ipa.11): a date or an event. When a date passes, the card comes back to
// mk's pick by itself and the holding thread is told once. Old holds (no end) read and behave exactly as before and
// show "no end set"; nothing here releases, prunes or rewrites one.
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { homeCli } from "../cli.js";
import { buildQueue } from "../queueview.js";
import { waitingNow } from "../waiting.js";
import { cleanupEnvs, opened, rig } from "./card-rig.js";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { holdEndText } from "../ui/holdend.js";
import { parseStoredEnd, untilMs } from "../holdend.js";
import { AsksPanel, type AsksData, type OwedAsk } from "../ui/asks.js";

const HOUR = 3_600_000;
const iso = (ms: number) => new Date(ms).toISOString();
const EVENT = { kind: "pr-merge", ref: "mistakeknot/Autarch#114", label: "PR 114 merges" };

describe("a new hold names its end", () => {
  afterEach(() => cleanupEnvs());

  it("(a) a hold with no date and no event is refused with a plain error that says what to pass", async () => {
    const r = rig();
    const { t } = await opened(r);
    const res = r.svc.hold(t.id, "script superseded", "thr_viz");
    expect(res.ok).toBe(false);
    expect((res as { error: string }).error).toMatch(/until/);
    expect((res as { error: string }).error).toMatch(/event/);
    expect(r.svc.holdOf(t.id)).toBeNull();
  });

  it("takes a date or an event, not both, and refuses a date that is unreadable or already past", async () => {
    const r = rig();
    const { t } = await opened(r);
    const now = r.env.clock.t;
    expect(r.svc.hold(t.id, "why", "thr_viz", { until: "next week" })).toMatchObject({ ok: false });
    expect(r.svc.hold(t.id, "why", "thr_viz", { until: iso(now - HOUR) })).toMatchObject({ ok: false });
    expect(r.svc.hold(t.id, "why", "thr_viz", { event: { kind: "pr-merge", ref: "x", label: "" } })).toMatchObject({ ok: false });
    expect(r.svc.hold(t.id, "why", "thr_viz", { event: { kind: "PR merge!", ref: "x", label: "ok" } })).toMatchObject({ ok: false });
    expect(r.svc.hold(t.id, "why", "thr_viz", { event: { kind: "pr-merge", ref: "x", label: "two\nlines" } })).toMatchObject({ ok: false });
    expect(r.svc.hold(t.id, "why", "thr_viz", { until: iso(now + HOUR), event: EVENT } as never)).toMatchObject({ ok: false });
    expect(r.svc.holdOf(t.id)).toBeNull();
    expect(r.svc.hold(t.id, "why", "thr_viz", { until: iso(now + HOUR) })).toEqual({ ok: true, task_id: t.id });
    expect(r.svc.holdOf(t.id)).toMatchObject({ reason: "why", end: { until: iso(now + HOUR) } });
    expect(r.svc.hold(t.id, "why", "thr_viz", { until: "2099-01-05" })).toMatchObject({ ok: true });
    expect(r.svc.holdOf(t.id)).toMatchObject({ end: { until: "2099-01-05" } });
    expect(r.svc.hold(t.id, "why", "thr_viz", { event: EVENT })).toMatchObject({ ok: true });
    expect(r.svc.holdOf(t.id)).toMatchObject({ end: { event: EVENT } });
  });

  it("changing a hold that is already live does not force an end, keeps the one it has, and accepts a new one", async () => {
    const r = rig();
    const { t } = await opened(r);
    const now = r.env.clock.t;
    r.svc.store.setSetting("holds", JSON.stringify({ [t.id]: { reason: "old", by: "thr_viz", at: iso(now - 5 * HOUR), n: "n0" } }));
    expect(r.svc.hold(t.id, "reworded", "thr_viz")).toMatchObject({ ok: true });
    expect(r.svc.holdOf(t.id)).toMatchObject({ reason: "reworded" });
    expect(r.svc.holdOf(t.id)!.end).toBeUndefined();
    expect(r.svc.hold(t.id, "dated now", "thr_viz", { until: iso(now + HOUR) })).toMatchObject({ ok: true });
    expect(r.svc.hold(t.id, "reworded again", "thr_viz")).toMatchObject({ ok: true });
    expect(r.svc.holdOf(t.id)).toMatchObject({ reason: "reworded again", end: { until: iso(now + HOUR) } });
  });

  it("(b) an old-format hold record still reads and behaves as before: held, no end, never released by time", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const start = r.env.clock.t;
    r.svc.store.setSetting("holds", JSON.stringify({ [t.id]: { reason: "from 11-02", by: "thr_viz", at: iso(start - 30 * 24 * HOUR), n: "n0" } }));
    expect(r.svc.holdOf(t.id)).toEqual({ reason: "from 11-02", by: "thr_viz", at: iso(start - 30 * 24 * HOUR), n: "n0" });
    expect(r.mkPick(g1.id)).toMatchObject({ ok: false, status: 409 });
    r.advance(400 * 24 * HOUR);
    await r.poll();
    await r.poll();
    expect(r.svc.holdOf(t.id)).toMatchObject({ reason: "from 11-02" });
    expect(JSON.parse(r.svc.store.setting("holds")!)[t.id]).toEqual({ reason: "from 11-02", by: "thr_viz", at: iso(start - 30 * 24 * HOUR), n: "n0" });
    expect(r.svc.store.obligationsFor(`hold:${t.id}`)).toHaveLength(0);
  });

  it("an end that cannot be read is ignored: the hold stays a hold with no end", async () => {
    const r = rig();
    const { t } = await opened(r);
    const at = iso(r.env.clock.t - HOUR);
    for (const end of ["soon", { until: "never" }, { event: { kind: "x" } }, { until: "2020-01-01", event: EVENT }, 5]) {
      r.svc.store.setSetting("holds", JSON.stringify({ [t.id]: { reason: "r", by: "v", at, end } }));
      expect(r.svc.holdOf(t.id)).toMatchObject({ reason: "r" });
      expect(r.svc.holdOf(t.id)!.end).toBeUndefined();
    }
  });
});

describe("when a hold's end passes", () => {
  afterEach(() => cleanupEnvs());

  it("(c) a dated hold past its date is back at mk's pick at once, and the holder is told exactly once", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const start = r.env.clock.t;
    r.svc.hold(t.id, "waiting on the script", "thr_viz", { until: iso(start + 2 * HOUR) });
    r.enableDelegation();
    const waiting = () => waitingNow(r.svc, r.dele, new Catchup(r.svc, r.dele));
    expect(waiting()).toMatchObject({ decide: 0, held: 1 });
    expect(r.mkPick(g1.id, "p-early")).toMatchObject({ ok: false, status: 409 });
    await r.poll();
    expect(r.svc.store.obligationsFor(`hold:${t.id}`)).toHaveLength(0);

    r.advance(3 * HOUR);
    // before any poll: reading is enough to count it as waiting on mk again
    expect(r.svc.holdOf(t.id)).toBeNull();
    expect(buildQueue(r.svc, new Asks(r.svc)).rows.find((x) => x.task_id === t.id)!.held).toBeNull();
    expect(waiting()).toMatchObject({ decide: 1, held: 0 });

    await r.poll();
    await r.poll();
    r.advance(61_000);
    await r.poll();
    const told = r.svc.store.obligationsFor(`hold:${t.id}`);
    expect(told).toHaveLength(1);
    expect(told[0]).toMatchObject({ recipient: "thr_viz" });
    expect(told[0]!.payload).toContain("waiting on the script");
    expect(told[0]!.payload).toContain(r.svc.cardKey(t.id)!);
    // the record is gone (a new hold with an end is the only kind the sweep releases)
    expect(JSON.parse(r.svc.store.setting("holds") ?? "{}")[t.id]).toBeUndefined();
    expect(r.mkPick(g1.id, "p-late")).toMatchObject({ ok: true });
  });

  it("a restart between the date passing and the next poll still tells the holder once", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.svc.hold(t.id, "why", "thr_viz", { until: iso(r.env.clock.t + HOUR) });
    r.advance(2 * HOUR);
    r.restart();
    await r.poll();
    await r.poll();
    expect(r.svc.store.obligationsFor(`hold:${t.id}`)).toHaveLength(1);
  });

  it("only an expired hold is released: other holds, old ones and Later data are untouched", async () => {
    const r = rig();
    const { t } = await opened(r);
    const now = r.env.clock.t;
    const old = { reason: "old", by: "thr_viz", at: iso(now - 9 * HOUR), n: "n0" };
    r.svc.store.setSetting("holds", JSON.stringify({ other: old, [t.id]: { reason: "dated", by: "thr_viz", at: iso(now - HOUR), n: "n1", end: { until: iso(now + HOUR) } } }));
    r.svc.store.setSetting("later", JSON.stringify({ lt: { at: iso(now - HOUR), by: "mk" } }));
    const laterBefore = r.svc.store.setting("later");
    r.advance(2 * HOUR);
    await r.poll();
    expect(JSON.parse(r.svc.store.setting("holds")!)).toEqual({ other: old });
    expect(r.svc.store.setting("later")).toBe(laterBefore);
  });

  it("a hold set again over an expired one still tells the holder about the first", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.svc.hold(t.id, "first", "thr_a", { until: iso(r.env.clock.t + HOUR) });
    r.advance(2 * HOUR);
    expect(r.svc.hold(t.id, "second", "thr_viz")).toMatchObject({ ok: false });
    expect(r.svc.hold(t.id, "second", "thr_viz", { until: "2099-01-05" })).toMatchObject({ ok: true });
    const told = r.svc.store.obligationsFor(`hold:${t.id}`);
    expect(told).toHaveLength(1);
    expect(told[0]).toMatchObject({ recipient: "thr_a" });
    await r.poll();
    expect(r.svc.store.obligationsFor(`hold:${t.id}`)).toHaveLength(1);
    expect(r.svc.holdOf(t.id)).toMatchObject({ reason: "second" });
  });

  it("an event end is reported through the hold tool: the card returns, the holder is told once, a repeat does nothing", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.svc.hold(t.id, "until PR 114", "thr_viz", { event: EVENT });
    await r.poll();
    r.advance(30 * 24 * HOUR);
    await r.poll();
    expect(r.svc.holdOf(t.id)).toMatchObject({ end: { event: EVENT } });
    expect(r.svc.endHold(t.id, "thr_other", EVENT.kind, EVENT.ref)).toMatchObject({ ok: true, ended: true });
    expect(r.svc.holdOf(t.id)).toBeNull();
    expect(r.svc.endHold(t.id, "thr_other", EVENT.kind, EVENT.ref)).toMatchObject({ ok: true, ended: false });
    const told = r.svc.store.obligationsFor(`hold:${t.id}`);
    expect(told).toHaveLength(1);
    expect(told[0]).toMatchObject({ recipient: "thr_viz" });
    expect(told[0]!.payload).toContain("PR 114 merges");
    expect(r.mkPick(g1.id)).toMatchObject({ ok: true });
  });

  it("a hold that has no end is not ended by the tool: only unhold releases it", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.svc.store.setSetting("holds", JSON.stringify({ [t.id]: { reason: "old", by: "thr_viz", at: iso(r.env.clock.t - HOUR), n: "n0" } }));
    expect(r.svc.endHold(t.id, "thr_viz", EVENT.kind, EVENT.ref)).toMatchObject({ ok: false });
    expect(r.svc.holdOf(t.id)).not.toBeNull();
    // and a date hold is released by its date only
    r.svc.hold(t.id, "d", "thr_viz", { until: "2099-01-05" });
    expect(r.svc.endHold(t.id, "thr_viz", EVENT.kind, EVENT.ref)).toMatchObject({ ok: false });
  });

  it("review 1: unhold over a date that passed but is not swept yet still tells the holder, once", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.svc.hold(t.id, "short", "thr_viz", { until: iso(r.env.clock.t + HOUR) });
    r.advance(2 * HOUR);
    expect(r.svc.unhold(t.id)).toMatchObject({ ok: true, was: false });
    const told = r.svc.store.obligationsFor(`hold:${t.id}`);
    expect(told).toHaveLength(1);
    expect(told[0]).toMatchObject({ recipient: "thr_viz" });
    expect(r.svc.sweepExpiredHolds()).toBe(0);
    expect(r.svc.store.obligationsFor(`hold:${t.id}`)).toHaveLength(1);
  });

  it("review 1: unhold of a live hold tells nobody (the vizier released it itself)", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.svc.hold(t.id, "live", "thr_viz", { until: "2099-01-05" });
    expect(r.svc.unhold(t.id)).toMatchObject({ ok: true, was: true });
    expect(r.svc.store.obligationsFor(`hold:${t.id}`)).toHaveLength(0);
  });

  it("review 2: a retried event report cannot release a replacement hold awaiting a different event", async () => {
    const r = rig();
    const { t } = await opened(r);
    const other = { kind: "pr-merge", ref: "mistakeknot/Autarch#115", label: "PR 115 merges" };
    r.svc.hold(t.id, "114", "thr_viz", { event: EVENT });
    expect(r.svc.endHold(t.id, "thr_viz", EVENT.kind, EVENT.ref)).toMatchObject({ ok: true, ended: true });
    r.svc.hold(t.id, "115", "thr_viz", { event: other });
    expect(r.svc.endHold(t.id, "thr_viz", EVENT.kind, EVENT.ref)).toMatchObject({ ok: true, ended: false });
    expect(r.svc.holdOf(t.id)).toMatchObject({ end: { event: other } });
    expect(r.svc.store.obligationsFor(`hold:${t.id}`)).toHaveLength(1);
    expect(r.svc.endHold(t.id, "thr_viz", other.kind, other.ref)).toMatchObject({ ok: true, ended: true });
    expect(r.svc.holdOf(t.id)).toBeNull();
  });

  it("review 2: hold-ended names the event it reports; without --event-ref it is refused", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.enableDelegation();
    const cli = homeCli({ svc: r.svc, asks: new Asks(r.svc), catchup: new Catchup(r.svc, r.dele), rule: (id, o, why, ctx) => r.dele.rule(id, o, why, ctx), isVizier: (th) => th === "thr_viz" });
    const go = (argv: string[]) => Promise.resolve(cli.run(argv, { threadId: "thr_viz" }));
    r.svc.hold(t.id, "x", "thr_viz", { event: EVENT });
    const bare = await go(["hold-ended", t.id]);
    expect(bare.exitCode).not.toBe(0);
    expect(bare.stderr).toMatch(/--event-ref/);
    expect(bare.stderr).toMatch(/--event-kind/);
    expect(r.svc.holdOf(t.id)).not.toBeNull();
    const noKind = await go(["hold-ended", t.id, "--event-ref", EVENT.ref]);
    expect(noKind.exitCode).not.toBe(0);
    expect(r.svc.holdOf(t.id)).not.toBeNull();
    const wrong = await go(["hold-ended", t.id, "--event-kind", EVENT.kind, "--event-ref", "mistakeknot/Autarch#999"]);
    expect(JSON.parse(wrong.stdout!)).toMatchObject({ ok: true, ended: false });
    expect(r.svc.holdOf(t.id)).not.toBeNull();
  });

  it("review 2: a delayed report of one kind of event cannot release a replacement hold on the same ref with another kind", async () => {
    const r = rig();
    const { t } = await opened(r);
    const review = { kind: "pr-review", ref: EVENT.ref, label: "PR 114 reviewed" };
    r.svc.hold(t.id, "review", "thr_viz", { event: review });
    r.svc.unhold(t.id);
    r.svc.hold(t.id, "merge", "thr_viz", { event: EVENT });
    expect(r.svc.endHold(t.id, "thr_viz", review.kind, review.ref)).toMatchObject({ ok: true, ended: false });
    expect(r.svc.holdOf(t.id)).toMatchObject({ end: { event: EVENT } });
    expect(r.svc.endHold(t.id, "thr_viz", EVENT.kind, EVENT.ref)).toMatchObject({ ok: true, ended: true });
  });

  it("review 2: what holdOf and holds return is a copy all the way down: editing it changes nothing stored", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.svc.hold(t.id, "d", "thr_viz", { until: "2099-01-05" });
    (r.svc.holdOf(t.id)!.end as { until: string }).until = "2000-01-01";
    (r.svc.holds()[t.id]!.end as { until: string }).until = "2000-01-01";
    expect(r.svc.holdOf(t.id)).toMatchObject({ end: { until: "2099-01-05" } });
    r.svc.hold(t.id, "e", "thr_viz", { event: EVENT });
    r.svc.holdOf(t.id)!.end = undefined;
    (r.svc.holds()[t.id]!.end as { event: { ref: string } }).event.ref = "changed";
    expect(r.svc.holdOf(t.id)).toMatchObject({ end: { event: EVENT } });
  });

  it("review 3: a calendar date that does not exist is refused, and a stored one is no end", async () => {
    const r = rig();
    const { t } = await opened(r);
    for (const bad of ["2099-02-30", "2099-11-31", "2099-13-01", "2099-00-10", "2099-01-05T25:00:00Z", "2099-02-30T10:00:00Z", "2099-01-05T10:61:00Z"]) {
      expect(r.svc.hold(t.id, "x", "thr_viz", { until: bad }), bad).toMatchObject({ ok: false });
    }
    expect(untilMs("2100-02-29")).toBeNaN();
    expect(untilMs("2096-02-29")).not.toBeNaN();
    expect(parseStoredEnd({ until: "2099-11-31" })).toBeUndefined();
  });

  it("bb home hold needs --until or an event, hold-ended reports one, and both are vizier only", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.enableDelegation();
    const cli = homeCli({ svc: r.svc, asks: new Asks(r.svc), catchup: new Catchup(r.svc, r.dele), rule: (id, o, why, ctx) => r.dele.rule(id, o, why, ctx), isVizier: (th) => th === "thr_viz" });
    const go = (argv: string[], threadId?: string) => Promise.resolve(cli.run(argv, threadId ? { threadId } : {}));
    const bare = await go(["hold", t.id, "--reason", "x"], "thr_viz");
    expect(bare.exitCode).not.toBe(0);
    expect(bare.stderr).toMatch(/--until/);
    expect(bare.stderr).toMatch(/--event-label/);
    expect(r.svc.holdOf(t.id)).toBeNull();
    const dated = await go(["hold", t.id, "--reason", "x", "--until", "2099-01-05"], "thr_viz");
    expect(dated.exitCode).toBe(0);
    expect(r.svc.holdOf(t.id)).toMatchObject({ end: { until: "2099-01-05" } });
    const half = await go(["hold", t.id, "--reason", "x", "--event-kind", "pr-merge"], "thr_viz");
    expect(half.exitCode).not.toBe(0);
    const ev = await go(["hold", t.id, "--reason", "x", "--event-kind", EVENT.kind, "--event-ref", EVENT.ref, "--event-label", EVENT.label], "thr_viz");
    expect(ev.exitCode).toBe(0);
    expect(r.svc.holdOf(t.id)).toMatchObject({ end: { event: EVENT } });
    expect((await go(["hold-ended", t.id, "--event-kind", EVENT.kind, "--event-ref", EVENT.ref], "thr_a")).exitCode).not.toBe(0);
    expect(r.svc.holdOf(t.id)).not.toBeNull();
    const done = await go(["hold-ended", t.id, "--event-kind", EVENT.kind, "--event-ref", EVENT.ref], "thr_viz");
    expect(JSON.parse(done.stdout!)).toMatchObject({ ok: true, ended: true });
    expect(r.svc.holdOf(t.id)).toBeNull();
    const list = JSON.parse((await go(["list", "--json"])).stdout!) as { task_id: string; held: unknown }[];
    expect(list.find((x) => x.task_id === t.id)!.held).toBeNull();
  });
});

describe("a hold's end on screen", () => {
  const NOW = Date.parse("2026-11-09T12:00:00.000Z");
  const held = (end?: unknown) => ({ reason: "script superseded", by: "thr_viz", at: "2026-11-01T00:00:00.000Z", ...(end === undefined ? {} : { end }) }) as never;

  it("(d) an old hold shows 'no end set'", () => {
    expect(holdEndText(held(), NOW)).toBe("no end set");
    expect(holdEndText(held(null), NOW)).toBe("no end set");
  });

  it("a date shows as 'until Nov 14' (the year only when it is not this year); an event shows its label", () => {
    expect(holdEndText(held({ until: "2026-11-14" }), NOW)).toBe("until Nov 14");
    expect(holdEndText(held({ until: "2026-11-14T18:30:00.000Z" }), NOW)).toBe("until Nov 14");
    expect(holdEndText(held({ until: "2026-11-10" }), NOW)).toBe("until Nov 10");
    expect(holdEndText(held({ until: "2027-01-03" }), NOW)).toBe("until Jan 3, 2027");
    expect(holdEndText(held({ event: EVENT }), NOW)).toBe("until PR 114 merges");
  });

  it("an end that is already past still reads as a date, and a missing label falls back to the event kind", () => {
    expect(holdEndText(held({ until: "2026-11-08" }), NOW)).toBe("until Nov 8");
    expect(holdEndText(held({ event: { kind: "pr-merge", ref: "x", label: "" } }), NOW)).toBe("until pr-merge x");
  });

  const owed = (h: unknown, over: Partial<OwedAsk> = {}): OwedAsk => ({
    id: "dec1",
    project: "Autarch",
    thread: "thr-a",
    subject: "Ship it?",
    asker: "thread",
    filed_at: "2026-09-29T10:00:00Z",
    revision: "rev1",
    mentions: 0,
    task_id: "task-1",
    ask: { question: "Which day?", options: [{ id: "day", label: "Day", kind: "ruling-only" }] },
    held: h as never,
    ...over,
  });
  const data = (o: OwedAsk[]): AsksData => ({ owed: o, runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {} });

  it("the desktop On hold list shows the end beside the reason, a long event label wraps, and an old hold says no end set", () => {
    const long = "PR 114 merges and the registrar finishes the deploy and mk has had a look at the phone view " + "x".repeat(200);
    const html = renderToStaticMarkup(
      <AsksPanel
        data={data([owed(held({ until: "2026-11-14" })), owed(held(), { id: "dec2", task_id: "task-2" }), owed(held({ event: { kind: "pr-merge", ref: "r", label: long } }), { id: "dec3", task_id: "task-3" })])}
        onPick={(() => {}) as never}
        onOpen={() => {}}
        onLater={() => {}}
      />,
    );
    expect(html).toContain("until Nov 14");
    expect(html).toContain("no end set");
    expect(html).toContain(`until ${long}`);
    expect(html.match(/data-hold-end/g)).toHaveLength(3);
    expect(html).toMatch(/<[a-z]+ [^>]*overflow-wrap:anywhere[^>]*data-hold-end|<[a-z]+ [^>]*data-hold-end[^>]*overflow-wrap:anywhere/);
  });
});
