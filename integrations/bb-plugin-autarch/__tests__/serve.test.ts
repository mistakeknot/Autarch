import { describe, expect, it } from "vitest";
import { ServeClient } from "../serve.js";

const sleepless = async () => {};

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
