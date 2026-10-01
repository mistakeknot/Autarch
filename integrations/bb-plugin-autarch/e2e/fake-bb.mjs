// A stand-in `bb` for fake mode. Its server URL comes from BB_SERVER_URL (the rig builds the child
// environment from nothing, so there is no E2E_BB_URL). It forwards `bb home ...` to the harness's
// in-process plugin CLI, `bb tasks ...` to the harness's tasks emulation (__tests__/fake-bb-server.ts), and `bb plugin rpc call tasks <method> --input-file <f> --json` to the
// harness's fake tasks plugin at <BB_SERVER_URL>/rpc.
// Mirrors what the real bb CLI does that matters here: `--X-stdin` becomes `--X <stdin>`, and
// BB_THREAD_ID is the caller's thread.
import http from "node:http";

const url = process.env.BB_SERVER_URL;
let argv = process.argv.slice(2);
if (url && argv[0] === "plugin" && argv[1] === "rpc" && argv[2] === "call") {
  const { readFileSync } = await import("node:fs");
  const f = argv.indexOf("--input-file");
  const input = f >= 0 ? JSON.parse(readFileSync(argv[f + 1], "utf8")) : null;
  const body = JSON.stringify({ plugin: argv[3], method: argv[4], input });
  const req = http.request(new URL("/rpc", url), { method: "POST", headers: { "content-type": "application/json" } }, (res) => {
    res.pipe(process.stdout, { end: false });
    res.on("end", () => process.exit(res.statusCode && res.statusCode < 400 ? 0 : 1));
  });
  req.on("error", (e) => (process.stderr.write(`fake bb: ${e.message}\n`), process.exit(1)));
  req.end(body);
  await new Promise(() => {});
}
const group = argv[0];
if ((group !== "home" && group !== "tasks") || !url) {
  process.stderr.write("fake bb: only `bb home ...` and `bb tasks ...` are supported\n");
  process.exit(1);
}
argv = argv.filter((a) => a !== "--json");
const at = argv.findIndex((a) => a.endsWith("-stdin"));
if (at >= 0) {
  const chunks = [];
  for await (const c of process.stdin) chunks.push(c);
  argv.splice(at, 1, `--${argv[at].slice(2, -6)}`, Buffer.concat(chunks).toString("utf8"));
}
const body = JSON.stringify({ group, argv: argv.slice(1), threadId: process.env.BB_THREAD_ID || undefined });
const req = http.request(url, { method: "POST", headers: { "content-type": "application/json" } }, (res) => {
  let text = "";
  res.on("data", (d) => (text += d));
  res.on("end", () => {
    const r = JSON.parse(text);
    if (r.stdout) process.stdout.write(`${r.stdout}\n`);
    if (r.stderr) process.stderr.write(r.stderr);
    process.exit(r.exitCode ?? 0);
  });
});
req.on("error", (e) => {
  process.stderr.write(`${e.message}\n`);
  process.exit(1);
});
req.end(body);
