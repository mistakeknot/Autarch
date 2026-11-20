import {createRequire} from 'node:module';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
const {chromium}=createRequire(resolve('integrations/bb-plugin-autarch/node_modules/x.js'))('playwright-core');
const dest=process.env.DEST,base=process.env.PREVIEW_URL;if(!dest||!base)throw Error('DEST and PREVIEW_URL required');mkdirSync(dest,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROME??'/usr/bin/google-chrome',args:['--no-sandbox']});
try{
for(const [width,height] of [[1440,900],[390,844],[360,800]]){
 const p=await browser.newPage({viewport:{width,height}});p.on('pageerror',e=>{throw e});
 await p.goto(base);await p.waitForSelector('[data-queue-tabs]');
 const check=async()=>{
  const geometry=await p.evaluate(()=>({overflow:document.documentElement.scrollWidth>innerWidth,header:document.querySelector('[data-queue-tabs]').parentElement.getBoundingClientRect().height}));
  if(geometry.overflow||geometry.header>(width<640?92:48))throw Error('header overflow '+JSON.stringify(geometry));
  if(await p.locator('[data-home-tabs],[data-glance-answers],[data-glance-time],[data-glance-lead]').count())throw Error('retired header or duplicate card summary');
  return geometry;
 };
 console.log(width,JSON.stringify(await check()));
 await p.screenshot({path:`${dest}/final-${width}x${height}.png`,fullPage:true});
 await p.locator('[data-project-filter-trigger]').click();
 const popup=await p.locator('[data-project-filter-popup]').boundingBox();if(!popup||popup.x<0||popup.x+popup.width>width)throw Error('filter popup clipped');
 await p.getByRole('option',{name:/Example/}).click();await p.getByRole('tab',{name:'Needs you now (0)',exact:true}).waitFor();await check();
 await p.locator('[data-project-filter-all]').click();await p.keyboard.press('Escape');
 await p.getByRole('tab',{name:'Needs you now (2)',exact:true}).waitFor();
 const open=async(name)=>{await p.locator('[data-home-overflow] summary').click();await p.getByRole('button',{name,exact:true}).click();};
 const queue=async()=>{await p.getByRole('tab',{name:'Needs you now (2)',exact:true}).click();await p.locator('[data-decision]').first().waitFor();};
 await open('Learn');await p.getByRole('button',{name:'Got it',exact:true}).click();await p.waitForSelector('[data-learn-mark="got"]');await queue();
 await open('Ideas');
 for(const action of ['Pursue','Park','Drop']){
  await p.locator('[data-idea-project]').fill('Example');await p.getByRole('option',{name:/Example/}).first().click();
  await p.locator('[data-idea-text]').fill('Try a smaller sample page');await p.locator('[data-idea-file]').click();await p.waitForSelector('[data-idea="sample"]');
  await p.getByRole('button',{name:action,exact:true}).click();await p.locator('[data-idea="sample"]').waitFor({state:'detached'});
 }
 await queue();await open('Archive');await p.waitForSelector('[data-archive-row]');
 await p.locator('[data-archive-search]').fill('No matching sample');await p.locator('[data-archive-apply]').click();await p.locator('[data-archive-row]').waitFor({state:'detached'});
 await p.locator('[data-archive-search]').fill('Sample');await p.locator('[data-archive-apply]').click();await p.waitForSelector('[data-archive-row]');
 await p.locator('[data-archive-bringback]').click();await p.locator('[data-archive-confirm]').click();await p.getByRole('status').filter({hasText:'Fixture archive question reopened'}).waitFor();
 await queue();await open('Vizier');await p.waitForSelector('[data-tell-vizier]');await p.getByText('Host thread chat attachment (fixture).',{exact:false}).waitFor();
 await queue();await open('Settings');await p.locator('input[name=vizierThreadId]').fill('sample-thread');await p.locator('[data-save]').click();await p.getByRole('status').filter({hasText:'Fixture settings saved'}).waitFor();await queue();await check();
 await p.close();
}
console.log('Permanent header verified without URL options at all widths; More destinations, local actions and Projects filter passed.');
}finally{await browser.close()}
