import { describe, expect, it } from "vitest";
import { ServeClient, ServeSupervisor } from "../serve.js";

const sleepless = async () => {};

describe("ServeSupervisor", () => {
  const mk = (healthy: () => boolean, clock: { t: number }) => {
    const spawned: string[][] = [];
    const sup = new ServeSupervisor({
      addr: "127.0.0.1:8110",
      bin: "autarch",
      tokenFile: "/t/serve.token",
      projectDirs: ["/p1", "/p2"],
      health: async () => healthy(),
      spawn: (bin, args) => {
        spawned.push([bin, ...args]);
      },
      now: () => clock.t,
    });
    return { sup, spawned };
  };

  it("never spawns while /health answers", async () => {
    const { sup, spawned } = mk(() => true, { t: 0 });
    await sup.check();
    expect(spawned).toEqual([]);
  });

  it("spawns with the token file and each project dir when health fails", async () => {
    const { sup, spawned } = mk(() => false, { t: 0 });
    await sup.check();
    expect(spawned).toEqual([["autarch", "serve", "--addr", "127.0.0.1:8110", "--token-file", "/t/serve.token", "--project-dir", "/p1", "--project-dir", "/p2"]]);
  });

  it("restarts at most 3 times per 10 minutes, then again once the window passes", async () => {
    const clock = { t: 0 };
    const { sup, spawned } = mk(() => false, clock);
    for (let i = 0; i < 6; i++) {
      await sup.check();
      clock.t += 1000;
    }
    expect(spawned.length).toBe(3);
    clock.t += 10 * 60_000;
    await sup.check();
    expect(spawned.length).toBe(4);
  });
});

describe("ServeClient", () => {
  it("sends the bearer token in a header only and reports errors", async () => {
    const seen: { url: string; auth?: string }[] = [];
    const c = new ServeClient({
      addr: "127.0.0.1:8110",
      readToken: () => "tok",
      fetch: async (url, init) => {
        seen.push({ url: String(url), auth: (init?.headers as Record<string, string>)?.Authorization });
        return new Response(JSON.stringify([{ name: "Autarch", root: "/r", dev: 1, ino: 2 }]), { status: 200 });
      },
    });
    expect(await c.projects()).toEqual([{ name: "Autarch", root: "/r", dev: 1, ino: 2 }]);
    expect(seen[0]).toEqual({ url: "http://127.0.0.1:8110/api/projects", auth: "Bearer tok" });
    const bad = new ServeClient({ addr: "x:1", readToken: () => { throw new Error("no token"); }, fetch: async () => new Response("", { status: 200 }) });
    await expect(bad.projects()).rejects.toThrow("no token");
    void sleepless;
  });
});
