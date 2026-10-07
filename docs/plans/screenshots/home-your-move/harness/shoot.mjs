// Serves the built harness over http://localhost (a secure context, so navigator.clipboard exists) and takes the
// screenshots, plus two real-browser clipboard checks. Usage: node shoot.mjs <outdir>
import { createRequire } from "node:module";
import { createServer } from "node:http";
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, resolve, extname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const PLUGIN_NM = process.env.PLUGIN_NM ?? resolve(here, "../../../../../integrations/bb-plugin-autarch/node_modules");
const SRC = process.env.OUT ?? "/tmp/ym-shots";
const DEST = resolve(process.argv[2] ?? resolve(here, ".."));
mkdirSync(DEST, { recursive: true });
const { chromium } = createRequire(`${PLUGIN_NM}/x.js`)("playwright-core");
const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" };
const server = createServer((req, res) => {
  try {
    const f = resolve(SRC, "." + (req.url.split("?")[0] === "/" ? "/after.html" : req.url.split("?")[0]));
    res.writeHead(200, { "content-type": types[extname(f)] ?? "text/plain" });
    res.end(readFileSync(f));
  } catch { res.writeHead(404); res.end(); }
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const base = `http://localhost:${server.address().port}`;
const browser = await chromium.launch();
const results = {};
const open = async (page, name, scene, width, height = 900) => {
  await page.setViewportSize({ width, height });
  await page.goto(`${base}/${name}.html#${scene}`);
  await page.reload();
  await page.waitForSelector("article, p");
};
const shot = async (page, file, opts = {}) => page.screenshot({ path: `${DEST}/${file}`, fullPage: true, ...opts });

const ctx = await browser.newContext({ permissions: ["clipboard-read", "clipboard-write"], deviceScaleFactor: 1 });
const page = await ctx.newPage();
for (const name of ["before", "after"]) {
  await open(page, name, "overview", 1100);
  await shot(page, `${name}-overview-1100.png`);
  await open(page, name, "unbound", 1100);
  await shot(page, `${name}-unbound-1100.png`);
}
await open(page, "after", "overview", 390);
await shot(page, "after-overview-390.png");
results.narrowHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
results.commandBoxesScroll = await page.evaluate(() => [...document.querySelectorAll("[data-command-box]")].map((b) => ({ scrolls: b.scrollWidth > b.clientWidth, overflowX: getComputedStyle(b).overflowX, whiteSpace: getComputedStyle(b).whiteSpace })));

// Real clipboard: click Copy on the real-run block, read the clipboard back, compare to the block text.
await open(page, "after", "overview", 1100);
const run = page.locator('[data-command="3. The real run"]');
const text = await run.locator("[data-command-text]").evaluate((e) => e.textContent);
await run.locator("[data-copy-button]").click();
const clip = await page.evaluate(() => navigator.clipboard.readText());
results.clipboardApi = { equalsBlockText: clip === text, length: clip.length, hasNewline: /[\n\r]/.test(clip), hasBacktick: clip.includes("`"), trailingSpace: clip !== clip.trim() };
results.copiedLabel = await run.locator("[data-copy-button]").innerText();
await shot(page, "after-copied-state.png", { fullPage: false });

// Fallback: no navigator.clipboard at all, so Home must use the select-all and execCommand path.
const ctx2 = await browser.newContext({ permissions: ["clipboard-read", "clipboard-write"] });
await ctx2.addInitScript(() => { Object.defineProperty(navigator, "clipboard", { value: undefined, configurable: true }); });
const p2 = await ctx2.newPage();
await open(p2, "after", "overview", 1100);
await p2.evaluate(() => { window.__copied = null; document.addEventListener("copy", (e) => { window.__copied = document.activeElement?.value ?? null; }, true); });
const run2 = p2.locator('[data-command="3. The real run"]');
await run2.locator("[data-copy-button]").click();
results.fallbackPath = { copyEventText: await p2.evaluate(() => window.__copied), equalsBlockText: (await p2.evaluate(() => window.__copied)) === text, label: await run2.locator("[data-copy-button]").innerText() };

// Selection: triple-click selects the command line only.
await run.locator("[data-command-text]").click({ clickCount: 3 });
results.tripleClickSelection = await page.evaluate(() => String(getSelection()).trim());
results.tripleClickEqualsCommand = results.tripleClickSelection === text;

// Hover and keyboard focus on buttons.
await open(page, "after", "overview", 1100, 700);
const first = page.locator("[data-move-action]").first();
await first.focus();
await page.keyboard.press("Tab");
await page.locator("[data-move-action]").nth(2).hover();
const card = page.locator("[data-move=mv-script] [data-move-buttons]");
await card.scrollIntoViewIfNeeded();
await card.screenshot({ path: `${DEST}/after-button-focus-and-hover.png` });

// Other box filled, counter, both buttons enabled.
await open(page, "after", "overview", 1100, 700);
const box = page.locator("[data-decision=dec-1] [data-other]");
await box.locator("textarea").fill("Run it Thursday instead, after the freeze lifts. Ask thr-cutover to confirm the rollback.");
await box.scrollIntoViewIfNeeded();
await box.screenshot({ path: `${DEST}/after-other-box-filled.png` });
results.otherCounter = await box.locator("[data-other-counter]").innerText();

writeFileSync(`${DEST}/browser-checks.json`, JSON.stringify(results, null, 2) + "\n");
console.log(JSON.stringify(results, null, 2));
await browser.close();
server.close();
