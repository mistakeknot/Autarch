import { afterEach, describe, expect, it } from "vitest";
import { Asks } from "../asks.js";
import { buildQueue } from "../queueview.js";
import { cleanupEnvs, opened, rig } from "./card-rig.js";

afterEach(cleanupEnvs);

describe("an open card that loses its Request line", () => {
  it("stays in the queue, display-only and flagged, while it is still labelled and open in tasks", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.edit(t, { description: t.description.replace(/^Request: .*\n/m, "") });
    await r.poll();
    const q = buildQueue(r.svc, new Asks(r.svc));
    const row = q.rows.find((x: any) => x.task_id === t.id);
    expect(row, JSON.stringify(r.cardRow(t.id))).toBeDefined();
    expect(row).toMatchObject({ display_only: true });
    expect(row!.display_reason).toMatch(/^Request line missing: re-file/);
  });

  it("the withdrawn card is not ruled twice: restoring the Request line reopens it", async () => {
    const r = rig();
    const { t } = await opened(r);
    const original = t.description;
    r.edit(t, { description: original.replace(/^Request: .*\n/m, "") });
    await r.poll();
    r.edit(t, { description: original });
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("open");
    const q = buildQueue(r.svc, new Asks(r.svc));
    expect(q.rows.find((x: any) => x.task_id === t.id)).toMatchObject({ display_only: false });
  });
});
