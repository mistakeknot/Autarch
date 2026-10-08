import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { UpdateMenu, type UpdateInfoView } from "../ui/update.js";

const A = "a".repeat(40);
const B = "b".repeat(40);
const base: UpdateInfoView = { status: { installed: A, latest: B, count: 3, subjects: ["fix x", "add y"], checked_at: "t" }, result: null, request: null, available: true, pending: false };
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
