import { expect, it } from "vitest";
import { confirmedRequestDelivery, type RequestEvent } from "../request-reminder-evidence.js";

const threadId = "thr_owner";
const ref = "wake:decision:pick";
const request = (data: Partial<RequestEvent["data"]> = {}, recipient = threadId): RequestEvent => ({
  type: "client/turn/requested", threadId: recipient, seq: 1, createdAt: 100,
  data: { requestId: "request-1", originPluginId: "autarch", wakeRef: ref, ...data },
});
const accepted = (id = "request-1", recipient = threadId): RequestEvent => ({
  type: "turn/input/accepted", threadId: recipient, seq: 3, createdAt: 500,
  data: { clientRequestId: id },
});

it("takes the provider acceptance timestamp for the exact recipient and request", () => {
  expect(confirmedRequestDelivery([accepted(), request()], threadId, ref, 100)).toEqual({ at: 500, id: "request-1" });
});

it.each([
  ["dispatch without acceptance", [request()]],
  ["acceptance without dispatch", [accepted()]],
  ["wrong recipient", [request(), accepted("request-1", "thr_other")]],
  ["another request", [request(), accepted("request-2")]],
  ["another plugin", [request({ originPluginId: "tasks" }), accepted()]],
  ["unattributed wake", [request({ originPluginId: undefined }), accepted()]],
  ["merged wake", [request({ inputGroups: [[], []] }), accepted()]],
  ["truncated provenance", [request({ wakeSourcesTruncated: true }), accepted()]],
  ["rejected dispatch", [request(), accepted(), { ...request(), type: "client/turn/rejected", seq: 2 }]],
  ["two independent dispatches of the same operation", [request(), request({ requestId: "request-2" }), accepted(), accepted("request-2")]],
] as const)("refuses %s", (_, events) => {
  expect(confirmedRequestDelivery(events, threadId, ref, 100)).toBeNull();
});

it("uses the first accepted retry when the original was not accepted", () => {
  const retry = { ...request({ requestId: "request-2", retryOfRequestId: "request-1", retryAttempt: 2 }), seq: 2, createdAt: 300 };
  expect(confirmedRequestDelivery([request(), retry, accepted("request-2")], threadId, ref, 100)).toEqual({ at: 500, id: "request-1" });
});

it("does not start a timer before the source operation existed", () => {
  expect(confirmedRequestDelivery([request(), accepted()], threadId, ref, 101)).toBeNull();
});
