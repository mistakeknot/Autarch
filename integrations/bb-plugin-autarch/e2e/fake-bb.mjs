// A stand-in `bb` for fake mode: forwards `bb home ...` to the harness's in-process plugin CLI.
// Mirrors what the real bb CLI does that matters here: `--X-stdin` becomes `--X <stdin>`, and
// BB_THREAD_ID is the caller's thread.
import http from "node:http";

const url = process.env.E2E_BB_URL;
let argv = process.argv.slice(2);
if (argv[0] !== "home" || !url) {
  process.stderr.write("fake bb: only `bb home ...` is supported\n");
  process.exit(1);
}
argv = argv.filter((a) => a !== "--json");
const at = argv.findIndex((a) => a.endsWith("-stdin"));
if (at >= 0) {
  const chunks = [];
  for await (const c of process.stdin) chunks.push(c);
  argv.splice(at, 1, `--${argv[at].slice(2, -6)}`, Buffer.concat(chunks).toString("utf8"));
}
const body = JSON.stringify({ argv: argv.slice(1), threadId: process.env.BB_THREAD_ID || undefined });
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
