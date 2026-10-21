// Card-key deep links (bead mk-a51hu): /plugins/autarch/home/<KEY> opens Home on that decision card. A key that is not
// an open card gets a notice with the card's state, and Home's own list stays under it.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { cardHref, cardKeyFrom, CardKeyNotice, focusedAsks } from "../ui/cardlink.js";
import { cardStateOf } from "../cardstate.js";
import type { AsksData, OwedAsk } from "../ui/asks.js";
import { openStore, tmpDir } from "./helpers.js";

describe("cardKeyFrom", () => {
  it("reads a card key from the rest of the path", () => {
    expect(cardKeyFrom("DEMO-1046")).toBe("DEMO-1046");
    expect(cardKeyFrom("/DEMO-111/")).toBe("DEMO-111");
    expect(cardKeyFrom("demo-1046")).toBe("DEMO-1046");
    expect(cardKeyFrom("DEMO-1046%0A")).toBeNull();
  });
  it("returns null for anything that is not one key", () => {
    for (const bad of [undefined, "", "/", "MSQ", "DEMO-", "1046", "DEMO-1046/extra", "../x", "DEMO-1046?x=1", "%E0%A4%A", "A-1"]) expect(cardKeyFrom(bad)).toBeNull();
  });
  it("builds the host-relative link", () => {
    expect(cardHref("DEMO-1046")).toBe("/plugins/autarch/home/DEMO-1046");
  });
});

describe("cardStateOf", () => {
  const withCards = (rows: [string, string, string, string | null][], run: (db: ReturnType<typeof openStore>["db"]) => void) => {
    const t = tmpDir();
    try {
      const store = openStore(`${t.dir}/h.db`);
      const ins = store.db.prepare("INSERT INTO cards (task_id, card_key, title, state, deleted_at) VALUES (?, ?, ?, ?, ?)");
      for (const [id, key, title, state] of rows) ins.run(id, key, title, state, null);
      run(store.db);
    } finally { t.cleanup(); }
  };
  it("reports an open card, a closed card and an unknown key", () => {
    withCards([["t1", "DEMO-1", "Open one", "open"], ["t2", "DEMO-2", "Done one", "closed"], ["t3", "DEMO-3", "Ruled one", "ruled"]], (db) => {
      expect(cardStateOf(db, "DEMO-1")).toEqual({ found: true, state: "open", title: "Open one" });
      expect(cardStateOf(db, "DEMO-2")).toEqual({ found: true, state: "closed", title: "Done one" });
      expect(cardStateOf(db, "DEMO-3")).toEqual({ found: true, state: "ruled", title: "Ruled one" });
      expect(cardStateOf(db, "DEMO-9")).toEqual({ found: false });
    });
  });
  it("does not report a deleted card", () => {
    withCards([["t1", "DEMO-1", "Gone", "open"]], (db) => {
      db.prepare("UPDATE cards SET deleted_at = '2026-10-21T00:00:00Z'").run();
      expect(cardStateOf(db, "DEMO-1")).toEqual({ found: false });
    });
  });
});

describe("CardKeyNotice", () => {
  const html = (state: Parameters<typeof CardKeyNotice>[0]["lookup"]) => renderToStaticMarkup(<CardKeyNotice cardKey="DEMO-9" lookup={state} />);
  it("says there is no such open card, with the state when the card exists", () => {
    const closed = html({ found: true, state: "closed", title: "Done one" });
    expect(closed).toContain("DEMO-9");
    expect(closed).toContain("no such open card");
    expect(closed).toContain("closed");
    expect(closed).toContain("Done one");
  });
  it("says it is unknown when no card has the key", () => {
    const h = html({ found: false });
    expect(h).toContain("no such open card");
    expect(h).toContain("no card with this key");
  });
  it("shows nothing while the lookup is pending and says so when it fails", () => {
    expect(html(null)).toBe("");
    expect(html("error")).toContain("could not be looked up");
  });
});

describe("focusedAsks", () => {
  it("keeps the one card and drops the rest of Home's items", () => {
    const card = { id: "d1", project: "p", thread: "t", subject: "s", key: "DEMO-1", asker: "card", filed_at: "2026-10-21T00:00:00Z", revision: "r", ask: { question: "q", options: [] } } as OwedAsk;
    const other = { ...card, id: "d2", key: "DEMO-2" };
    const lane = { id: "l1", subject: "waiting", thread: "t", owner: null, detail: "", updated_at: "2026-10-21T00:00:00Z" };
    const asks = {
      owed: [card, other], runbook: [{ thread: "t", items: [] }], lane: [lane], asks: [lane], undeliverable: [{ id: "o1" }], failures: [{ id: "o2" }], uncertain: [{ id: "o3" }],
      delegation: { settings: {}, suspended: false }, approvals: [{ approval_id: "a1", decision_id: "d2", kind: "k", target: "t", identity: "i", minted_at: "x", expires_at: "y" }], machineOwners: { m: "o" },
    } as AsksData;
    const f = focusedAsks(asks, card);
    expect(f.owed).toEqual([card]);
    for (const k of ["runbook", "lane", "asks", "undeliverable", "failures", "uncertain"] as const) expect(f[k]).toEqual([]);
    expect(f.approvals).toBeUndefined();
    expect(f.delegation).toBe(asks.delegation);
  });
});
