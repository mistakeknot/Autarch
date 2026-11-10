// One "waiting on you" count (bead mk-j5ipa.9). It is the open decision cards that need a pick now: not on hold, not set
// aside for Later. Notices, held and Later cards are shown beside it by name and never inside it, and every surface
// (the rail badge, the Queue tab, the phone inbox, and what agents read from `bb home stats`) shows that one number.
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { homeCli } from "../cli.js";
import { owedCounts, waitingLine } from "../waitingcount.js";
import { waitingNow } from "../waiting.js";
import { HomeTabs, QueueTabs } from "../ui/tabs.js";
import { railCount } from "../ui/waiting.js";
import { inboxRows, MobileInbox } from "../ui/inbox.js";
import type { AsksData, OwedAsk } from "../ui/asks.js";
import { cleanupEnvs, rig, type Rig } from "./card-rig.js";

afterEach(cleanupEnvs);

let n = 0;
const card = (r: Rig) => {
  n += 1;
  return r.card({ key: `one-${n}`, ident: n.toString(16).padStart(16, "0") });
};

/** Three plain decide cards, two on hold, two set aside for Later, and one unseen notice (a vizier adoption). */
async function fixture() {
  const r = rig();
  r.enableDelegation();
  const plain = [card(r), card(r), card(r)];
  const held = [card(r), card(r)];
  const later = [card(r), card(r)];
  await r.poll();
  for (const [i, t] of held.entries()) expect(r.svc.hold(t.id, `waiting on the vendor ${i}`, "mk")).toMatchObject({ ok: true });
  for (const t of later) expect(r.svc.later(t.id, "mk")).toMatchObject({ ok: true });
  r.svc.store.recordEvent("vizier-adopted", null, { from: null, to: "thr_viz2", reason: "unset" });
  const cu = new Catchup(r.svc, r.dele);
  return { r, cu, plain, held, later };
}

const NOW = Date.parse("2026-10-09T12:00:00Z");
const ask = (over: Partial<OwedAsk> = {}): OwedAsk => ({
  id: "d1",
  project: "Clavain",
  thread: "thr-a",
  subject: "Which effort change?",
  asker: "thread",
  filed_at: "2026-10-09T07:00:00Z",
  revision: "r1",
  task_id: "t1",
  ask: { question: "Pick one.", recommendation: "a", options: [{ id: "a", label: "A: no change", kind: "ruling-only" }, { id: "b", label: "B: change it", kind: "ruling-only" }] },
  ...over,
});
const HELD = (reason: string) => ({ reason, by: "vizier", at: "2026-11-02T00:00:00Z" });
const LATER = { at: "2026-10-09T09:00:00Z", by: "mk" };
const asks = (owed: OwedAsk[]): AsksData => ({ owed, runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {} });
const noop = () => {};
const phone = (a: AsksData, waiting: number, initial?: { tab?: "waiting" | "later" | "since"; holdOpen?: boolean }) =>
  renderToStaticMarkup(<MobileInbox asks={a} waiting={waiting} onPick={noop as never} onOpen={noop} onNote={noop} onLater={noop} sinceNode={<p>since body</p>} sinceCount={0} onDesktop={noop} now={NOW} {...(initial ? { initial } : {})} />);

describe("the one waiting count: the server", () => {
  it("is the cards that need a pick now; notices are named beside it, not inside it", async () => {
    const { r, cu } = await fixture();
    const w = waitingNow(r.svc, r.dele, cu);
    expect(w).toMatchObject({ decide: 3, held: 2, later: 2, notices: 1 });
    expect(w.total).toBe(3); // was 4: the notice used to be added to the number
    expect(w.line).toBe("3 · 2 on hold · 2 later");
  });

  it("agents read the same number from bb home stats, with the held and Later counts by name", async () => {
    const { r, cu } = await fixture();
    const cli = homeCli({ svc: r.svc, asks: new Asks(r.svc), catchup: cu, waiting: () => waitingNow(r.svc, r.dele, cu), rule: (id, o, why, ctx) => r.dele.rule(id, o, why, ctx), isVizier: () => false });
    const s = JSON.parse((await cli.run(["stats", "--json"], {})).stdout!);
    expect(s.waiting).toMatchObject({ total: 3, decide: 3, held: 2, later: 2, notices: 1, line: "3 · 2 on hold · 2 later" });
  });
});

describe("every surface shows that number", () => {
  it("the rail badge, the Queue tab, the Decide tab and the phone header all say 3 for the fixture", async () => {
    const { r, cu } = await fixture();
    const w = waitingNow(r.svc, r.dele, cu);
    // The same rows the listAsks handler builds, reduced to what the surfaces read.
    const rows = r.svc.owed().map((d, i) => ask({ id: String(d.id), task_id: String(d.task_id), subject: `card ${i}`, held: r.svc.holdOf(String(d.task_id)), later: r.svc.laterOf(String(d.task_id)) }));
    expect(owedCounts(rows)).toEqual({ pick: 3, held: 2, later: 2 });
    expect(railCount(w.decide, rows)).toBe(3);
    expect(renderToStaticMarkup(<HomeTabs waiting={w.total} onOpen={noop} />)).toContain("Queue (3)");
    expect(renderToStaticMarkup(<QueueTabs tab="decide" onTab={noop} counts={{ decide: w.decide, later: 2, blocks: 0, catchup: 0 }} line={w.line} />)).toContain("Needs you now (3)");
    expect(phone(asks(rows), w.total)).toContain("Waiting on you · 3");
  });
});

describe("waitingLine", () => {
  it("names held and Later, and leaves out a part that is zero", () => {
    expect(waitingLine({ pick: 10, held: 12, later: 8 })).toBe("10 · 12 on hold · 8 later");
    expect(waitingLine({ pick: 10, held: 0, later: 8 })).toBe("10 · 8 later");
    expect(waitingLine({ pick: 0, held: 3, later: 0 })).toBe("0 · 3 on hold");
    expect(waitingLine({ pick: 0, held: 0, later: 0 })).toBe("0");
  });
  it("a card that is both held and set aside counts as held only", () => {
    expect(owedCounts([{ held: HELD("x"), later: LATER }, { later: LATER }, {}])).toEqual({ pick: 1, held: 1, later: 1 });
  });
});

describe("Desktop Queue tab line", () => {
  it("shows the count with held and Later by name, and nothing extra when there are none", () => {
    const counts = { decide: 10, later: 8, blocks: 0, catchup: 0 };
    const html = renderToStaticMarkup(<QueueTabs tab="decide" onTab={noop} counts={counts} line="10 · 12 on hold · 8 later" />);
    expect(html).toContain("10 · 12 on hold · 8 later");
    expect(html).toContain("data-waiting-line");
    expect(renderToStaticMarkup(<QueueTabs tab="decide" onTab={noop} counts={counts} line="10" />)).not.toContain("data-waiting-line");
  });
});

describe("phone: the On hold row", () => {
  const held = (id: string, reason: string) => ask({ id, task_id: `t-${id}`, key: `K-${id}`, subject: `Held ${id}`, held: HELD(reason) });

  it("held cards used to be absent from the phone; now one closed On hold row carries their count", () => {
    const a = asks([ask(), held("h1", "waiting on the vendor"), held("h2", "sprint scope")]);
    const html = phone(a, 1);
    expect(html).toContain("data-inbox-hold");
    expect(html).toMatch(/On hold · 2/);
    expect(html).toContain('aria-expanded="false"');
    expect(html).not.toContain("waiting on the vendor"); // closed: the reasons are not listed
    expect(html).not.toContain("Held h1");
    expect(html).toContain("12 on hold".replace("12", "2")); // the line under the title
  });

  it("opening it lists each held card with its reason, read-only", () => {
    const html = phone(asks([ask(), held("h1", "waiting on the vendor"), held("h2", "sprint scope")]), 1, { holdOpen: true });
    expect(html).toContain('aria-expanded="true"');
    expect(html).toContain("On hold: waiting on the vendor");
    expect(html).toContain("On hold: sprint scope");
    expect(html).toContain("K-h1");
    expect(html).toContain("Held h2");
    // read-only: no pick, Later, hold or open controls on a held row
    const from = html.indexOf("data-inbox-hold-list");
    const list = html.slice(from, html.indexOf("</ul>", from));
    expect(list).not.toMatch(/data-option|data-bar-|data-later|data-inbox-row|data-confirm/);
    expect(list).not.toContain("<button");
  });

  it("the row is a 44px-tall tap target", () => {
    const html = phone(asks([held("h1", "x")]), 0);
    expect(html.match(/<button[^>]*data-inbox-hold-toggle[^>]*>/)![0]).toContain("min-h-11");
  });

  it("is absent with nothing on hold, and the empty Waiting list still says so", () => {
    expect(phone(asks([ask()]), 1)).not.toContain("data-inbox-hold");
    const empty = phone(asks([held("h1", "x")]), 0);
    expect(empty).toContain("Nothing is waiting on you.");
    expect(empty).toContain("data-inbox-hold");
  });

  it("one held card, many held cards and a long reason all render without clipping", () => {
    expect(phone(asks([held("h1", "x")]), 0)).toMatch(/On hold · 1/);
    const many = Array.from({ length: 60 }, (_, i) => held(`m${i}`, `reason ${i}`));
    const html = phone(asks(many), 0, { holdOpen: true });
    expect(html).toMatch(/On hold · 60/);
    expect(html.match(/data-held-row=/g)).toHaveLength(60);
    const long = "a very long reason ".repeat(40) + "https://example.test/" + "x".repeat(200);
    const lh = phone(asks([held("h1", long)]), 0, { holdOpen: true });
    expect(lh).toContain(long.trim());
    expect(lh).toMatch(/\[overflow-wrap:anywhere\]/);
  });

  it("the header shows held and Later by name under the count, and a card both held and Later is held only", () => {
    const a = asks([ask(), ask({ id: "d2", task_id: "t2", later: LATER }), ask({ id: "d3", task_id: "t3", held: HELD("x"), later: LATER })]);
    const html = phone(a, 1);
    expect(html).toContain("Waiting on you · 1");
    expect(html).toContain("1 on hold · 1 later");
    expect(inboxRows(a, NOW).held.map((x) => x.key)).toEqual(["ask:d3"]);
  });

  it("the Later tab is unchanged: Later cards stay visible there, so no new Later row is added", () => {
    const a = asks([ask({ later: LATER })]);
    expect(phone(a, 0, { tab: "later" })).toContain('data-inbox-row="ask:d1"');
  });
});
