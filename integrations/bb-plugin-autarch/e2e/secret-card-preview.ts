// Fake-only browser proof for mk-tp8hn. No server, live Home data or secret request.
import { build } from "esbuild";
import { chromium } from "playwright-core";
import { mkdir, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const plugin = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const out = process.argv[2];
if (!out) throw new Error("usage: tsx e2e/secret-card-preview.ts <artifact-directory>");
await mkdir(out, { recursive: true });
const bundled = await build({
  absWorkingDir: plugin, bundle: true, write: false, format: "iife", jsx: "automatic",
  define: { "process.env.NODE_ENV": '"production"' },
  stdin: { resolveDir: plugin, loader: "tsx", contents: `
    import { createRoot } from "react-dom/client";
    import { AskCard } from "./ui/asks.js";
    const metadata={thread:"thr_23456789ab",names:["FAKE_TEST_TOKEN"],path:"/fake-test/credentials.env",project:"fake-test-project"};
    const ask={id:"fake-decision",project:metadata.project,thread:metadata.thread,subject:"Provide the requested credential",asker:"thread",filed_at:"2026-01-01",revision:"fake-revision",secret:metadata,ask:{question:"Enter the requested credential below.",options:[]}};
    createRoot(document.getElementById("root")).render(<AskCard ask={ask} onPick={()=>{throw Error("unexpected pick")}} onNote={()=>{throw Error("unexpected note")}} onOpen={()=>{}} />);
  ` },
  plugins: [{ name: "fake-session-sdk", setup(b) {
    b.onResolve({ filter: /^@get-bb\/plugin-sdk\/app$/ }, () => ({ path: "fake-sdk", namespace: "fake" }));
    b.onLoad({ filter: /.*/, namespace: "fake" }, () => ({ loader: "js", contents: `
      const metadata={thread:"thr_23456789ab",names:["FAKE_TEST_TOKEN"],path:"/fake-test/credentials.env",project:"fake-test-project"};
      const row=()=>({id:"pint_23456789ab",threadId:metadata.thread,createdAt:Date.now(),expiresAt:Date.now()+60000,origin:{kind:"plugin",pluginId:"secrets",rendererId:"secret-request"},status:"pending",resolution:null,payload:{kind:"plugin",title:"Add secrets",data:{threadId:metadata.thread,project:metadata.project,purpose:"Connect the fake test service for this request.",requestId:"fake-test-request",destination:{kind:"dotenv",path:metadata.path,consumer:"fake-test-consumer"},expiresAtMs:Date.now()+60000,fields:[{name:"FAKE_TEST_TOKEN",description:"An obvious placeholder for the fake test service.",existing:true}]}}});
      const sdk={threads:{interactions:{list:async()=>window.fakeState==="expired"?[]:[row()],respond:async()=>({...row(),status:"resolved",resolution:{kind:"plugin_submitted"}})}}};
      export const useSdk=()=>sdk;
      export const useRpc=()=>({call:()=>{throw Error("unexpected Citadel RPC")}});
      export const ThreadChat=()=>null;
      export const experimental_Icon=()=>null;
    ` }));
  } }],
});
const browser = await chromium.launch({ headless: true, ...(process.env.SECRET_PREVIEW_BROWSER ? { executablePath: process.env.SECRET_PREVIEW_BROWSER } : {}) });
try {
  const css = `
    :root{--foreground:#e4e7ef;--muted:#9ca6bd;--card:#1b1e2b;--raised:#282d3f;--border:#3d445d;--destructive:#fca5a5}
    *{box-sizing:border-box}body{margin:0;background:#171923;color:var(--foreground);font:14px/1.5 system-ui;padding:40px}article{max-width:760px;margin:auto;padding:24px;border:1px solid var(--border);border-radius:12px}h3{font-size:14px;margin:0 0 8px}p{margin:0}article>p{margin-top:8px;color:var(--muted);font-size:12px}article>p button{padding:0;border:0;color:inherit;background:none;text-decoration:underline}
    [data-secret-state]{margin-top:16px}section[data-secret-state]{container-type:inline-size;container-name:secrets}form>*+*{margin-top:16px}form>div:first-child>*+*{margin-top:8px}form p{font-size:12px;color:var(--muted)}form p.text-sm{font-size:14px;color:var(--foreground);line-height:1.6}code{padding:2px 6px;border-radius:4px;background:var(--raised);color:var(--foreground);font-family:ui-monospace,monospace;overflow-wrap:anywhere}dl{display:grid;grid-template-columns:auto minmax(0,1fr);gap:2px 12px;font-size:12px;margin:0;color:var(--muted)}dd{margin:0;color:var(--foreground);overflow-wrap:anywhere}.font-mono{font-family:ui-monospace,monospace}
    form>div:nth-child(2)>div+div{margin-top:14px}label{display:block;font:600 12px ui-monospace,monospace}.relative{position:relative;margin-top:6px}input{display:block;width:100%;min-height:44px;padding:8px 44px 8px 12px;border:1px solid var(--border);border-radius:6px;background:var(--card);color:var(--foreground);font:16px ui-monospace,monospace}input:focus-visible,button:focus-visible{outline:2px solid #829fff;outline-offset:2px}
    button{cursor:pointer;font:500 14px system-ui}button[aria-label^="Show "],button[aria-label^="Hide "]{position:absolute;right:4px;top:50%;transform:translateY(-50%);display:flex;align-items:center;justify-content:center;width:28px;height:28px;border:0;border-radius:6px;padding:0;background:transparent;color:var(--muted)}button[aria-label]:hover{background:var(--raised)}svg{display:block;flex:none}
    [data-secret-error]{border:1px solid #753f4d;background:#36212b;border-radius:6px;padding:8px;color:var(--destructive);font-size:12px}form>div:last-child{display:flex;justify-content:flex-end;border-top:1px solid var(--border);padding-top:16px}button[type=submit]{display:inline-flex;align-items:center;justify-content:center;gap:6px;min-height:36px;padding:8px 14px;border:1px solid transparent;border-radius:6px;background:var(--foreground);color:#171923}button:disabled{opacity:.5;cursor:default}
    @container secrets (max-width:383px){button[type=submit]{width:100%}}@media(max-width:600px){body{padding:16px}article{padding:16px}}
  `;
  for (const viewport of [{name:"desktop",width:1280,height:900},{name:"phone",width:390,height:844}]) {
   for (const state of ["waiting", "revealed", "error", "submitted", "expired"]) {
    const page = await browser.newPage({ viewport: { width: viewport.width, height: viewport.height } });
    await page.route("**/*", route => route.abort());
    await page.setContent(`<html><head><meta name="viewport" content="width=device-width, initial-scale=1"><style>${css}</style></head><body><div id="root"></div></body></html>`);
    await page.evaluate(s => { (window as unknown as { fakeState: string }).fakeState=s; }, state);
    await page.addScriptTag({ content: bundled.outputFiles[0]!.text });
    await page.locator(`[data-secret-state="${state === "expired" ? "expired" : "waiting"}"]`).waitFor();
    if (state === "revealed") {
      await page.locator('input[type="password"]').fill("fake-test-value-not-a-secret");
      await page.getByRole("button",{name:"Show FAKE_TEST_TOKEN"}).click();
      await page.getByRole("button",{name:"Hide FAKE_TEST_TOKEN"}).waitFor();
    }
    if (state === "error") {
      await page.locator('input[type="password"]').fill("fake-test-".repeat(2000));
      await page.getByRole("button",{name:"Add secrets"}).click();
      await page.locator('[data-secret-error][aria-live="polite"]').waitFor();
    }
    if (state === "submitted") {
      await page.locator('input[type="password"]').fill("fake-test-value-not-a-secret");
      await page.getByRole("button", { name: "Add secrets" }).click();
      await page.locator('[data-secret-state="submitted"]').waitFor();
      if ((await page.content()).includes("fake-test-value-not-a-secret")) throw new Error("value echo");
    }
    if (state !== "expired" && state !== "submitted") {
      const fontSize=await page.locator("input").evaluate(el=>Number.parseFloat(getComputedStyle(el).fontSize));
      if(fontSize<16)throw Error("input text below 16px");
    }
    const file=`secret-card-${viewport.name}-${state}`;
    await page.screenshot({ path: join(out, `${file}.png`), fullPage: true });
    await writeFile(join(out, `${file}.html`), await page.content());
    await page.close();
   }
  }
} finally { await browser.close(); }
