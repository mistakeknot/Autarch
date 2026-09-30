// sha256 over the sorted relative paths and contents of a source tree,
// excluding identity.json, dist/ and node_modules/. Shared by the plugin's
// health RPC and scripts/build-identity.mjs so both compute the same value.
import { createHash } from "node:crypto";
import { lstatSync, readdirSync, readFileSync, readlinkSync } from "node:fs";
import { join } from "node:path";

const SKIP_TOP = new Set(["dist", "node_modules"]);

function walk(root, rel, out) {
  for (const name of readdirSync(join(root, rel)).sort()) {
    const r = rel ? `${rel}/${name}` : name;
    if (r === "identity.json" || SKIP_TOP.has(name)) continue;
    const st = lstatSync(join(root, r));
    if (st.isDirectory()) walk(root, r, out);
    else if (st.isSymbolicLink()) out.push([r, "link:" + readlinkSync(join(root, r))]);
    else if (st.isFile()) out.push([r, createHash("sha256").update(readFileSync(join(root, r))).digest("hex")]);
  }
}

export function sourceSha256(root) {
  const files = [];
  walk(root, "", files);
  files.sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0));
  const h = createHash("sha256");
  for (const [p, d] of files) h.update(`${p}\0${d}\n`);
  return h.digest("hex");
}
