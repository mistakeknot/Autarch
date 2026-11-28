import Database from "better-sqlite3";
import { afterEach, expect, it, vi } from "vitest";
import { createFakePluginHost, makePluginAgentConfigurationContext, makeThreadResponse } from "@get-bb/plugin-sdk/testing";
import { wireHome } from "../server.js";
import { createStoreHandle } from "../store.js";
import { ServeClient } from "../serve.js";
import { REQUEST_SIGNAL_TOOL } from "../request-reminder-runtime.js";
import { decision, pickOf } from "./helpers.js";
import { FakeSdk } from "./wakes-helpers.js";

afterEach(() => { vi.useRealTimers(); vi.unstubAllEnvs(); });
const projectId = "proj_fixture";
const start = Date.parse("2026-10-01T00:00:00Z");
const source = "wake:decision_fixture:pick_fixture";
const accepted = start + 30_000;
const step = 15_000;

it.each([
  ["report", "thr_owner", null, 9],
  ["on", "thr_owner", null, 9],
  ["on", "thr_coordinator", null, 14],
  ["on", "thr_coordinator", "thr_coordinator", 9],
  ["on", "thr_vizier", null, 9],
] as const)("runs the normal reminder entry point in %s with recipient %s and explicit owner %s", async (mode, recipient, explicitOwner, due) => {
  vi.useFakeTimers({ now: start });
  vi.stubEnv("AUTARCH_REQUEST_REMINDERS_FROM", mode === "on" ? new Date(start).toISOString() : "");
  const { bb, harness } = createFakePluginHost({ pluginId: "autarch" });
  const db = new Database(":memory:");
  const handle = createStoreHandle(() => db, {});
  const sdk = new FakeSdk();
  const owner = makeThreadResponse({ id: "thr_owner", projectId, providerId: "codex", title: "Worker" });
  const coordinator = { ...owner, id: "thr_coordinator", title: "Autarch | coordinator" };
  const vizier = { ...owner, id: "thr_vizier", title: "Masaq' | vizier" };
  const threads = [owner, coordinator, vizier];
  harness.sdk.stub("threads.get", async ({ threadId }) => threads.find(row => row.id === threadId)! as Awaited<ReturnType<typeof bb.sdk.threads.get>>);
  harness.sdk.stub("threads.list", async () => threads);
  harness.sdk.stub("projects.get", async () => ({ id: projectId, name: "Autarch" }) as Awaited<ReturnType<typeof bb.sdk.projects.get>>);
  harness.sdk.stub("plugins.callRpc", async () => ({ comments: [{
    id: "comment_fixture", taskId: "task_fixture", kind: "agent", authorName: "Worker",
    threadId: recipient, body: "The work is done.", createdAt: new Date(accepted + step).toISOString(),
  }] }));
  let delivered = false;
  harness.sdk.stub("threads.events.list", async () => ([
    ...(delivered ? [{ type: "turn/input/accepted", threadId: recipient, seq: 2, createdAt: accepted, data: { clientRequestId: "request-1" } }] : []),
    { type: "client/turn/requested", threadId: recipient, seq: 1, createdAt: start,
      data: { requestId: "request-1", originPluginId: "autarch", wakeRef: source } },
  ]) as Awaited<ReturnType<typeof bb.sdk.threads.events.list>>);
  wireHome(bb, handle, { serveAddr: "127.0.0.1:1", serveTokenFile: "/fixture", serveProjectDirs: [], autarchBin: "autarch" },
    { serve: new ServeClient({ addr: "127.0.0.1:1", readToken: () => "" }), sdk });
  expect(await bb.sdk.threads.get({ threadId: recipient })).toMatchObject({ id: recipient, archivedAt: null, deletedAt: null, visibility: "visible" });
  const run = harness.runService("home-request-reminders");
  try {
    const visible = await harness.resolveAgentConfiguration(makePluginAgentConfigurationContext());
    expect(visible.tools.map(tool => tool.name)).toContain(REQUEST_SIGNAL_TOOL);
    await vi.advanceTimersByTimeAsync(0);
    const store = handle.store();
    store.setSetting("vizierThreadId", "thr_vizier");
    db.prepare("INSERT INTO cards(task_id,title,state,status) VALUES ('task_fixture','Fixture','open','in_progress')").run();
    store.insertDecision(decision({ id: "decision_fixture", source: "card", task_id: "task_fixture", card_fp: "fixture",
      generation: 1, thread: recipient, owner_thread: explicitOwner ?? undefined, project: "Autarch",
      body_json: JSON.stringify({ v: 1, kind: "decide", project: "Autarch", project_root: "/fixture",
        asker: "thread", thread: recipient, question: "Should this go ahead?", recommendation: "a", options: [
          { id: "a", kind: "instruction", label: "Do it", instruction: "Work the fixture." },
          { id: "b", kind: "ruling-only", label: "Wait" },
        ] }) }));
    expect(store.recordPick(pickOf("decision_fixture", { pick_id: "pick_fixture", picked_at: new Date(start).toISOString() }),
      [{ id: "wake_fixture", kind: "wake", op: source, recipient }])).toMatchObject({ ok: true });
    await vi.advanceTimersByTimeAsync(step);
    expect(sdk.sent).toHaveLength(0);
    delivered = true;
    await vi.advanceTimersByTimeAsync(accepted - start - step + due * 60_000 - 1);
    expect(sdk.sent).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(1);
    expect(sdk.sent.map(send => send.threadId)).toEqual(mode === "on" && recipient !== "thr_vizier" ? [recipient] : []);
    const own = await harness.callAgentTool(REQUEST_SIGNAL_TOOL, { action: "list" }, { threadId: recipient, projectId });
    expect(JSON.parse(String(own))).toMatchObject({ ok: true, requests: [
      expect.objectContaining({ request: "pick_fixture", accepted: recipient !== "thr_vizier" }),
    ] });
    if (recipient === "thr_owner") {
      const answer = { action: "reply", request: "pick_fixture", comment: "comment_fixture" };
      const wrong = await harness.callAgentTool(REQUEST_SIGNAL_TOOL, answer, { threadId: "thr_coordinator", projectId });
      expect(JSON.parse(String(wrong))).toMatchObject({ ok: false });
      const right = await harness.callAgentTool(REQUEST_SIGNAL_TOOL, answer, { threadId: recipient, projectId });
      expect(JSON.parse(String(right))).toEqual({ ok: true });
      await vi.advanceTimersByTimeAsync(42 * 60_000);
      expect(sdk.sent.map(send => send.threadId)).toEqual(mode === "on" ? [recipient] : []);
    }
  } finally {
    run.controller.abort();
    await run.done;
    await harness.dispose();
    if (db.open) db.close();
  }
});
