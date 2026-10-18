// Screenshots the built harness at phone sizes. Usage: node shoot.mjs <outdir>
import { createRequire } from "node:module";
import { createServer } from "node:http";
import { readFileSync, mkdirSync } from "node:fs";
import { dirname, resolve, extname } from "node:path";
import { fileURLToPath } from "node:url";
const here = dirname(fileURLToPath(import.meta.url));
const PLUGIN_NM = process.env.PLUGIN_NM ?? resolve(here, "../../../../../integrations/bb-plugin-autarch/node_modules");
const SRC = process.env.OUT ?? "/tmp/hm-shots";
const DEST = resolve(process.argv[2] ?? resolve(here, ".."));
mkdirSync(DEST, { recursive: true });
const { chromium } = createRequire(`${PLUGIN_NM}/x.js`)("playwright-core");
const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" };
const server = createServer((req, res) => { try { const f = resolve(SRC, "." + req.url.split("?")[0]); res.writeHead(200, { "content-type": types[extname(f)] ?? "text/plain" }); res.end(readFileSync(f)); } catch { res.writeHead(404); res.end(); } });
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const base = `http://localhost:${server.address().port}`;
const browser = await chromium.launch();
for (const [w, h] of [[390, 844], [430, 932]]) for (const name of ["before", "after"]) {
  const page = await browser.newPage({ viewport: { width: w, height: h }, deviceScaleFactor: 2 });
  page.on("pageerror", (e) => console.log("pageerror", String(e).slice(0, 300)));
  await page.goto(`${base}/${name}.html`); await page.waitForSelector("nav");
  await page.screenshot({ path: `${DEST}/${name}-${w}x${h}.png` });
  const m = await page.evaluate(() => ({ pageOverflowX: document.documentElement.scrollWidth > innerWidth, tabsOneLine: (() => { const n = document.querySelector("nav"); return n.getBoundingClientRect().height; })(), tabsScroll: (() => { const n = document.querySelector("nav"); return n.scrollWidth > n.clientWidth; })(), minTapHeight: Math.min(...[...document.querySelectorAll("nav button, ol button")].map((b) => b.getBoundingClientRect().height)), projectLines: document.querySelectorAll("[data-ask-project]").length }));
  console.log(name, w, JSON.stringify(m)); await page.close();
}
await browser.close(); server.close();
