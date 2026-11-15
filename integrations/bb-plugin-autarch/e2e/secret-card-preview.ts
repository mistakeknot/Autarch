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
      const row=()=>({id:"pint_23456789ab",threadId:metadata.thread,createdAt:Date.now(),expiresAt:Date.now()+60000,origin:{kind:"plugin",pluginId:"secrets",rendererId:"secret-request"},status:"pending",resolution:null,payload:{kind:"plugin",title:"Add secrets",data:{threadId:metadata.thread,project:metadata.project,destination:{kind:"dotenv",path:metadata.path,consumer:"fake-test-consumer"},expiresAtMs:Date.now()+60000,fields:[{name:"FAKE_TEST_TOKEN",description:null,existing:false}]}}});
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
  for (const state of ["waiting", "expired", "submitted"]) {
    const page = await browser.newPage({ viewport: { width: 900, height: 700 } });
    await page.route("**/*", route => route.abort());
    await page.setContent(`<html><head><style>body{background:#171923;color:#e4e7ef;font:16px system-ui;padding:40px}article{max-width:720px;border:1px solid #555c70;border-radius:12px;padding:24px}section{border:1px solid #555c70;border-radius:8px;padding:16px}input,button{background:#222633;color:#e4e7ef;border:1px solid #68718c;border-radius:5px;padding:10px;margin:6px}label{display:block}code{font-size:14px}p{line-height:1.5}</style></head><body><div id="root"></div></body></html>`);
    await page.evaluate(s => { (window as unknown as { fakeState: string }).fakeState=s; }, state);
    await page.addScriptTag({ content: bundled.outputFiles[0]!.text });
    await page.locator(`[data-secret-state="${state === "submitted" ? "waiting" : state}"]`).waitFor();
    if (state === "submitted") {
      await page.locator('input[type="password"]').fill("fake-test-value-not-a-secret");
      await page.getByRole("button", { name: "Submit securely" }).click();
      await page.locator('[data-secret-state="submitted"]').waitFor();
      if ((await page.content()).includes("fake-test-value-not-a-secret")) throw new Error("value echo");
    }
    await page.screenshot({ path: join(out, `secret-card-${state}.png`), fullPage: true });
    await writeFile(join(out, `secret-card-${state}.html`), await page.content());
    await page.close();
  }
} finally { await browser.close(); }
