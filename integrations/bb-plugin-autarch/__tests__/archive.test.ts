import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { archiveView } from "../archive.js";
import { renderToStaticMarkup } from "react-dom/server";
import { createElement } from "react";
import { ArchiveRowView } from "../ui/archive.js";
import { homeTabs } from "../ui/tabs.js";
import type { Service } from "../service.js";
import { ask, makeEnv, type Env } from "./service-helpers.js";
import { cleanupEnvs, opened, rig } from "./card-rig.js";

let env: Env;
let svc: Service;
beforeEach(() => { env = makeEnv(["Autarch"]); svc = env.open(); });
afterEach(() => { env.cleanup(); cleanupEnvs(); });

const view = (q = {}) => archiveView(svc.store.db, q);

describe("archive view", () => {
  it("lists nothing while no card is decided, and an open card is not listed", async () => {
    await svc.file(ask(env, { thread: "thr-a", question: "Still open?", subject: "open" }), {});
    expect(view().total).toBe(0);
  });

  it("lists a decided card with the pick label, who picked and when, and filters by search and pick", async () => {
    const r = rig();
    const one = await opened(r, { title: "Alpha question" });
    const two = await opened(r, { title: "Beta question" });
    r.mkPick(one.g1.id, "p1", "a");
    r.mkPick(two.g1.id, "p2", "b");
    const v = (q = {}) => archiveView(r.db, q);
    r.db.prepare("UPDATE cards SET state='ruled'").run();
    const all = v();
    expect(all.total).toBe(2);
    expect(all.rows[0]!.by).toBe("mk");
    expect(all.rows.map((x) => x.pick_label).sort()).toEqual(["Per day", "Per project"]);
    expect(all.rows.every((x) => !x.no_pick)).toBe(true);
    expect(v({ search: "Alpha" }).total).toBe(1);
    expect(v({ pick: "Per day" }).total).toBe(1);
    expect(v({ search: "no such thing" }).total).toBe(0);
    expect(v({ limit: 1 }).rows).toHaveLength(1);
    expect(v({ limit: 1, offset: 1 }).rows).toHaveLength(1);
    expect(v({ from: "2999-01-01" }).total).toBe(0);
    const day = all.rows[0]!.decided_at.slice(0, 10);
    expect(v({ to: day }).total).toBe(2); // the whole of the named day counts
    expect(v({ to: "2000-01-01" }).total).toBe(0);
    expect(v({ search: "100%" }).total).toBe(0);
  });

  it("lists a closed card that never had a pick, marked as such", () => {
    svc.store.db.prepare("INSERT INTO cards (task_id, project_id, card_key, title, state, updated_at) VALUES ('t1','p','AUTA-1','Old card','closed','2026-01-01T00:00:00Z')").run();
    const v = view({ project: "AUTA" });
    expect(v.total).toBe(1);
    expect(v.rows[0]!.no_pick).toBe(true);
    expect(v.projects).toContain("AUTA");
    expect(view({ project: "OTHER" }).total).toBe(0);
  });

  it("adds the Archive tab to the tab row", () => {
    expect(homeTabs().map((t) => t[0])).toContain("archive");
  });

  it("renders a row with the pick, and the no-pick note for a card Home never ruled", () => {
    const base = { task_id: "t", card_key: "AUTA-1", title: "T", project: "AUTA", state: "ruled", decided_at: "2026-10-01T10:00:00Z", pick_label: "Per day", option_id: "b", by: "mk", reason: null, generation: 1, no_pick: false };
    const a = renderToStaticMarkup(createElement(ArchiveRowView, { row: base }));
    expect(a).toContain("You picked");
    expect(a).toContain("Per day");
    const b = renderToStaticMarkup(createElement(ArchiveRowView, { row: { ...base, pick_label: null, option_id: null, by: null, no_pick: true } }));
    expect(b).toContain("without a pick");
  });
});
