import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { homeCli } from "../cli.js";
import { cliOnlyVerbs, homeCliMap, homeMethods } from "../contract.js";
import { makeEnv, type Env, verifiedDelegation } from "./service-helpers.js";

let env: Env;
let cli: ReturnType<typeof homeCli>;
beforeEach(() => {
  env = makeEnv(["Autarch"]);
  const svc = env.open();
  const dele = verifiedDelegation(svc);
  cli = homeCli({ svc, asks: new Asks(svc), catchup: new Catchup(svc, dele), rule: (a, b, c, d) => dele.rule(a, b, c, d), isVizier: () => false });
});
afterEach(() => env.cleanup());

const verbs = (): string[] => {
  const c = cli.commands as unknown;
  return Array.isArray(c) ? c.map((x) => (x as { name: string }).name) : Object.keys(c as object);
};
const mirrored = Object.entries(homeCliMap).flatMap(([method, m]) => ("verb" in m ? [{ method, ...m }] : []));
const rpcOnly = Object.entries(homeCliMap).flatMap(([method, m]) => ("rpcOnly" in m ? [{ method, why: m.rpcOnly }] : []));

describe("the CLI and the RPC contract", () => {
  it("map every RPC method exactly once, and nothing else", () => {
    expect(Object.keys(homeCliMap).sort()).toEqual(Object.keys(homeMethods).sort());
  });

  it("have a verb for every mirrored method, and every verb is mirrored or listed as CLI only", () => {
    const have = verbs().sort();
    const expected = [...mirrored.map((m) => m.verb), ...cliOnlyVerbs.map((v) => v.replace(" ", "-"))].sort();
    expect(have).toEqual(expected);
    expect(new Set(expected).size).toBe(expected.length);
  });

  it("show the contract's summary in the help of each mirrored verb", async () => {
    const help = (await Promise.resolve(cli.run(["--help"], {}))).stdout!;
    for (const m of mirrored) {
      expect(help).toContain(`bb home ${m.verb}`);
      expect(help).toContain(m.summary);
    }
  });

  it("give every RPC-only method a reason and no verb", () => {
    expect(rpcOnly.length).toBeGreaterThan(20);
    for (const r of rpcOnly) {
      expect(r.why.length, r.method).toBeGreaterThan(5);
      // `note` is also a CLI-only verb, a different and vizier-only one.
      if (!(cliOnlyVerbs as readonly string[]).includes(r.method)) expect(verbs(), r.method).not.toContain(r.method);
    }
    // No delegation setting is reachable from a verb [D-16].
    expect(rpcOnly.map((r) => r.method)).toEqual(expect.arrayContaining(["setDelegation", "override"]));
  });

  it("read the stats window from the same limits as the stats method", async () => {
    expect((await Promise.resolve(cli.run(["stats", "--since", "400d", "--json"], {}))).exitCode).toBe(2);
    expect((await Promise.resolve(cli.run(["stats", "--since", "14d", "--json"], {}))).exitCode).toBe(0);
  });
});

describe("RPC outputs", () => {
  it("are no longer unchecked: each schema rejects a value of the wrong kind", () => {
    for (const [name, m] of Object.entries(homeMethods)) {
      expect(m.output.safeParse(7).success, name).toBe(false);
      expect(m.output.safeParse("x").success, name).toBe(false);
    }
  });

  it("accept the shapes the service returns, extra fields included", () => {
    expect(homeMethods.pick.output.safeParse({ ok: true, status: 201, pick: { id: "p" } }).success).toBe(true);
    expect(homeMethods.pick.output.safeParse({ ok: false, status: 409, error: "ruled" }).success).toBe(true);
    expect(homeMethods.pick.output.safeParse({ status: 200 }).success).toBe(false);
    expect(homeMethods.catchup.output.safeParse({ items: [] }).success).toBe(true);
    expect(homeMethods.catchup.output.safeParse({ items: "no" }).success).toBe(false);
    expect(homeMethods.listRecent.output.safeParse({ recent: [{ id: "x" }] }).success).toBe(true);
    expect(homeMethods.conversationUnread.output.safeParse({ unread: { t1: 2 } }).success).toBe(true);
    expect(homeMethods.conversationUnread.output.safeParse({ unread: { t1: "two" } }).success).toBe(false);
    expect(homeMethods.health.output.safeParse({ ready: true, error: null }).success).toBe(true);
    expect(homeMethods.queue.output.safeParse([]).success).toBe(false);
  });
});
