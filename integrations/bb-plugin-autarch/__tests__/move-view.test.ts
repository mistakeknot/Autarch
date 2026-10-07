// The moves read model, the --check record, the git helper and the unbound-pick refusal (chunk B backend).
import { afterEach, describe, expect, it } from "vitest";
import { gitCommand, scriptCommands } from "../moves.js";
import { moveViews } from "../moveview.js";
import { cleanupEnvs, opened, rig, SHA } from "./card-rig.js";

afterEach(cleanupEnvs);

const fence = (m: object) => "```home-move\n" + JSON.stringify(m) + "\n```\n";
const script = { schema: "home-move/v1", kind: "script", script: { path: "/opt/run.sh", sha256: SHA, args: ["--day"], recover: { path: "/opt/undo.sh", sha256: "b".repeat(64) } } };

describe("moveViews", () => {
  it("carries the commands built by the safe builder, in run order, and the card title", async () => {
    const r = rig();
    const t = r.card({ title: "Run the thing" });
    t.description += fence(script);
    await r.poll();
    const g = moveViews(r.svc);
    expect(g.yourMove).toHaveLength(1);
    const v = g.yourMove[0]!;
    expect(v).toMatchObject({ kind: "script", title: "Run the thing", state: "open", checked_at: null, script: { path: "/opt/run.sh", sha256: SHA } });
    expect(v.commands.map((c) => c.label)).toEqual(["sha256sum", "check", "run", "recover"]);
    expect(v.commands).toEqual(scriptCommands({ path: "/opt/run.sh", sha256: SHA, args: ["--day"], recover: { path: "/opt/undo.sh", sha256: "b".repeat(64) } }));
    for (const c of v.commands) expect(c.command).not.toMatch(/[\n`]|^\s|\s$/);
  });

  it("shows a report as plain fields and a no-report as its deadline", async () => {
    const r = rig();
    const t = r.card({});
    t.description += fence(script);
    await r.poll();
    r.svc.claimMove(t.id, 1);
    r.svc.store.setMoveReport(t.id, 1, "failed", { outcome: "failed", failing_step: "step 3", error_line: "ERROR: <b>boom</b>", report_link: "/x/report.md", reported_at: "2026-10-01T01:00:00.000Z" });
    const v = moveViews(r.svc).reported[0]!;
    expect(v.report).toMatchObject({ outcome: "failed", failing_step: "step 3", error_line: "ERROR: <b>boom</b>", report_link: "/x/report.md" });
    r.svc.store.setMoveReport(t.id, 1, "no-report", { deadline_at: "2026-10-01T03:00:00.000Z" });
    expect(moveViews(r.svc).reported[0]!.report).toMatchObject({ outcome: "no-report" });
  });
});

describe("checkMove (I ran --check)", () => {
  it("records once, claims nothing, starts no deadline", async () => {
    const r = rig();
    const t = r.card({});
    t.description += fence(script);
    await r.poll();
    expect(r.svc.checkMove(t.id, 1)).toMatchObject({ ok: true, replay: false, state: "open" });
    expect(r.svc.checkMove(t.id, 1)).toMatchObject({ ok: true, replay: true });
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "open", claimed_at: null, report_deadline_at: null });
    expect(moveViews(r.svc).yourMove[0]!.checked_at).not.toBeNull();
  });
  it("is for script moves only", async () => {
    const r = rig();
    const t = r.card({});
    t.description += fence({ schema: "home-move/v1", kind: "pr", pr: { url: "https://github.com/o/r/pull/12" } });
    await r.poll();
    expect(r.svc.checkMove(t.id, 1)).toMatchObject({ ok: false, status: 400 });
    expect(r.svc.checkMove("nope", 1)).toMatchObject({ ok: false, status: 404 });
  });
});

describe("gitCommand", () => {
  it("runs git as mk, quoted, on one line", () => {
    expect(gitCommand("/home/example/repo", ["status", "--short"])).toBe("runuser -u mk -- git -C '/home/example/repo' 'status' '--short'");
    expect(gitCommand("/home/example/my repo", ["commit"])).toBe("runuser -u mk -- git -C '/home/example/my repo' 'commit'");
    expect(() => gitCommand("/home/example/repo", ["commit", "-m", "it's done"])).toThrow(/quote/);
  });
});

describe("pick on an unbound card", () => {
  it("is refused for every option, and nothing is recorded", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.db.prepare("UPDATE cards SET root_state = 'unverified' WHERE task_id = ?").run(t.id);
    expect(r.mkPick(g1.id)).toMatchObject({ ok: false, status: 403, error: "Project not bound: add a root to rule this" });
    expect(r.svc.store.pick(g1.id)).toBeUndefined();
  });
});
