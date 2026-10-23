import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { UPDATE_POLL_IDLE_MS, UPDATE_POLL_PENDING_MS, UpdateMenu, updatePollMs, type UpdateInfoView } from "../ui/update.js";

const A = "a".repeat(40);
const B = "b".repeat(40);
const base: UpdateInfoView = { status: { installed: A, latest: B, count: 3, subjects: ["fix x", "add y"], checked_at: "t" }, result: null, request: null, available: true, pending: false, now: "2026-10-23T18:32:00Z", interval_ms: 300_000, busy: false, log_path: "/state/runner.log", unit: "autarch-home-update.service" };
const pend: UpdateInfoView = { ...base, available: false, pending: true, request: { sha: B, clicked_at: "2026-10-23T18:30:00Z" } };
const html = (info: UpdateInfoView | null, error?: string) => renderToStaticMarkup(<UpdateMenu info={info} onRequest={() => {}} error={error} />);

describe("UpdateMenu", () => {
  it("renders nothing without status or without an update", () => {
    expect(html(null)).toBe("");
    expect(html({ ...base, available: false })).toBe("");
  });
  it("labels the commit count, lists subjects and offers the latest sha", () => {
    const h = html(base);
    expect(h).toContain("Update available (3 commits)");
    expect(h).toContain("fix x");
    expect(h).toContain("Request update to bbbbbbb");
    expect(h).toContain("data-update-request");
  });
  it("shows pending with no button", () => {
    const h = html({ ...base, available: false, pending: true });
    expect(h).toContain("Update requested");
    expect(h).not.toContain("data-update-request");
  });
  it("shows a failed result and an error", () => {
    const h = html({ ...base, result: { sha: B, ok: false, finished_at: "t2", message: "rolled back" } }, "refused");
    expect(h).toContain("Update failed");
    expect(h).toContain("rolled back");
    expect(h).toContain("refused");
    expect(h).toContain("Retry update to bbbbbbb");
    expect(h).not.toContain("Update available");
  });
});

describe("UpdateMenu placement", () => {
  it("opens in flow, not absolutely positioned inside a scrolling row", () => {
    expect(html(base)).not.toContain("absolute");
  });
  it("HomeTabs puts the extra outside the scrolling nav", async () => {
    const { HomeTabs } = await import("../ui/tabs.js");
    const h = renderToStaticMarkup(<HomeTabs extra={<UpdateMenu info={base} onRequest={() => {}} />} onOpen={() => {}} onTodos={() => {}} />);
    const nav = h.slice(h.indexOf("<nav"), h.indexOf("</nav>"));
    expect(nav).not.toContain("data-update-menu");
    expect(h).toContain("data-home-extra");
    expect(h.indexOf("data-update-menu")).toBeGreaterThan(h.indexOf("</nav>"));
  });
});

describe("UpdateMenu pending label", () => {
  it("names the requested sha, not a newer latest", () => {
    const h = html({ ...base, available: false, pending: true, request: { sha: A, clicked_at: "t" } });
    expect(h).toContain("Update requested (aaaaaaa)");
    expect(h).not.toContain("Update requested (bbbbbbb)");
  });
});

describe("UpdateMenu pending states", () => {
  it("waiting: says when the request was made and when the runner should start", () => {
    const h = html(pend);
    expect(h).toContain("Requested 2 min ago. The runner checks every 5 minutes, so it should start by 18:35 UTC.");
    expect(h).toContain('data-update-pending-kind="waiting"');
    expect(h).not.toContain("on its own schedule");
    expect(h).not.toContain("Installing");
  });
  it("waiting: uses the interval it is given", () => {
    expect(html({ ...pend, interval_ms: 600_000 })).toContain("checks every 10 minutes, so it should start by 18:40 UTC");
  });
  it("installing: shows Installing instead of Waiting while the runner holds its lock", () => {
    const h = html({ ...pend, busy: true });
    expect(h).toContain('data-update-pending-kind="installing"');
    expect(h).toContain("Installing…");
    expect(h).toContain("Installing update (bbbbbbb)…");
    expect(h).not.toContain("should start by");
    expect(h).not.toContain("Update requested");
  });
  it("stuck: after two intervals with no result, names the log and the unit", () => {
    const h = html({ ...pend, now: "2026-10-23T18:41:00Z" });
    expect(h).toContain('data-update-pending-kind="stuck"');
    expect(h).toContain("looks stuck");
    expect(h).toContain("/state/runner.log");
    expect(h).toContain("systemctl --user status autarch-home-update.service");
    expect(h).not.toContain("should start by");
  });
  it("not stuck at exactly two intervals, and a running install is never called stuck", () => {
    expect(html({ ...pend, now: "2026-10-23T18:40:00Z" })).toContain('data-update-pending-kind="waiting"');
    expect(html({ ...pend, now: "2026-10-23T19:40:00Z", busy: true })).toContain('data-update-pending-kind="installing"');
  });
  it("an unreadable request time still says something plain", () => {
    expect(html({ ...pend, request: { sha: B, clicked_at: "soon" } })).toContain("The runner checks every 5 minutes.");
  });
});

describe("update polling", () => {
  it("polls fast while a request is out so Installing and the age show, slowly otherwise", () => {
    expect(updatePollMs(pend)).toBe(UPDATE_POLL_PENDING_MS);
    expect(updatePollMs(base)).toBe(UPDATE_POLL_IDLE_MS);
    expect(updatePollMs(null)).toBe(UPDATE_POLL_IDLE_MS);
    expect(UPDATE_POLL_PENDING_MS).toBeLessThan(UPDATE_POLL_IDLE_MS);
  });
  it("app.tsx re-arms its timer from updatePollMs", async () => {
    const { readFileSync } = await import("node:fs");
    const src = readFileSync(new URL("../app.tsx", import.meta.url), "utf8");
    expect(src).toMatch(/updatePollMs\(update\)/);
    expect(src).toMatch(/setInterval\(loadUpdate, updatePoll\)/);
    expect(src).toMatch(/\[loadUpdate, updatePoll\]/);
  });
});
