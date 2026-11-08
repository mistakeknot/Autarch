// update-tick (plan 2.5/2.6): the scheduled runner. Under the lock: reconcile, publish status, and run a pending click's
// deploy in runner mode. A paused or refused deploy leaves result.json {ok:false} (deploy writes it).
import { spawnSync } from "node:child_process";
import { writeFileSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { childEnv } from "./bb.mjs";
import { deploy, publishAll, readRequest } from "./deploy.mjs";
import { EXIT, SHA40, refuse } from "./util.mjs";

const parseTime = (s) => {
  const t = Date.parse(s);
  return Number.isNaN(t) ? null : t;
};

/** A request is pending when it names a sha other than the installed one and no result for it is at least as new. */
export function pendingRequest(s, request, result) {
  if (!request) return null;
  const installed = s.live?.state === "unknown" ? null : s.live?.commit;
  if (installed === request.sha) return null;
  if (result && result.sha === request.sha) {
    const fin = parseTime(result.finished_at);
    const click = parseTime(request.clicked_at);
    if (fin !== null && click !== null && fin >= click) return null;
  }
  return request;
}

function reportOut(s, title, report) {
  const hostReport = s.dirs.host?.report;
  if (!hostReport) return;
  const dir = mkdtempSync(join(tmpdir(), "bpd-report-"));
  try {
    const f = join(dir, "message.txt");
    writeFileSync(f, `${report.message}\n`, { mode: 0o600 });
    const argv = [...hostReport.command, "--title", hostReport.title, "--message-file", f];
    spawnSync(argv[0], argv.slice(1), { env: childEnv({ serverUrl: s.serverUrl }), stdio: "ignore", timeout: 60_000 });
  } catch {
    /* reporting is best effort */
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

export function updateTick(s, o, readResult) {
  const report = { command: "update-tick", ok: true, exit: EXIT.OK, message: "nothing to do", warnings: [] };
  if (!s.plugin.updateRequest) throw refuse("the plugin file has no updateRequest: update-tick cannot read the pending click");
  const pub = publishAll(s, null);
  if (pub.warning) report.warnings.push(pub.warning);
  const request = readRequest(s);
  const pend = pendingRequest(s, request, readResult(s.stateDir));
  if (!pend) return report;
  if (!SHA40.test(pend.sha)) return report;
  const classes = s.plugin.classes;
  const cls = o.cls ?? (classes.length === 1 ? classes[0] : null);
  if (!cls) throw refuse("update-tick needs --class when the plugin file lists more than one class");
  const d = deploy(s, { commit: pend.sha, cls, mode: "runner", expectTree: undefined, confirmedFirstRun: o.confirmedFirstRun });
  reportOut(s, "deploy", d);
  return { ...d, command: "update-tick" };
}


