// Bundles before.tsx and after.tsx and compiles Tailwind v4 for both trees. Needs esbuild, react and Tailwind from the
// plugin's node_modules and the bb plugin-build toolchain (BB_BUILD_NM). Output goes to OUT (default /tmp/hu-shots).
import { createRequire } from "node:module";
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const PLUGIN_NM = process.env.PLUGIN_NM ?? resolve(here, "../../../../../integrations/bb-plugin-autarch/node_modules");
const BB_BUILD = process.env.BB_BUILD; // path to the bb plugin-build package (required)
if (!BB_BUILD) throw new Error("set BB_BUILD to the bb plugin-build package directory");
const OUT = process.env.OUT ?? "/tmp/hu-shots";
mkdirSync(OUT, { recursive: true });
const { build } = createRequire(`${PLUGIN_NM}/x.js`)("esbuild");
for (const name of ["before", "after"]) {
  await build({ entryPoints: [resolve(here, `${name}.tsx`)], bundle: true, outfile: `${OUT}/${name}.js`, format: "iife", jsx: "automatic", nodePaths: [PLUGIN_NM], loader: { ".ts": "ts", ".tsx": "tsx" }, define: { "process.env.NODE_ENV": '"production"' }, logLevel: "warning" });
}
const req = createRequire(`${BB_BUILD}/package.json`);
const { compile } = req("@tailwindcss/node");
const { Scanner } = req("@tailwindcss/oxide");
const compiler = await compile(readFileSync(resolve(here, "theme.css"), "utf8"), { base: BB_BUILD, onDependency() {} });
const scanner = new Scanner({ sources: [
  { base: resolve(here, "../../../../../integrations/bb-plugin-autarch/ui"), pattern: "**/*", negated: false },
  { base: "/tmp/hu-before/integrations/bb-plugin-autarch/ui", pattern: "**/*", negated: false },
] });
writeFileSync(`${OUT}/theme.css`, compiler.build(scanner.scan()));
for (const name of ["before", "after"]) writeFileSync(`${OUT}/${name}.html`, `<!doctype html><meta charset=utf-8><link rel=stylesheet href=theme.css><div id=root></div><script src=${name}.js></script>`);
console.log("built into", OUT);
