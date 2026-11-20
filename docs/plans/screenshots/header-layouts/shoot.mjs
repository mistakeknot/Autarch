import {createRequire} from 'node:module';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
const {chromium}=createRequire(resolve('integrations/bb-plugin-autarch/node_modules/x.js'))('playwright-core');
const dest=process.env.DEST,base=process.env.PREVIEW_URL;if(!dest||!base)throw Error('DEST and PREVIEW_URL required');mkdirSync(dest,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROME??'/usr/bin/google-chrome',args:['--no-sandbox']});
try{
for(const [width,height] of [[1440,900],[390,844],[360,800]])for(const layout of ['compact','merged','wrap']){
 const p=await browser.newPage({viewport:{width,height}});p.on('pageerror',e=>{throw e});
 await p.goto(base+'/?headerLayout='+layout);await p.waitForSelector('[data-queue-tabs]');
 await p.screenshot({path:`${dest}/${layout}-${width}x${height}.png`,fullPage:true});
 const overflow=await p.evaluate(()=>document.documentElement.scrollWidth>innerWidth);if(await p.locator("[data-glance-time]").count())throw Error("empty time chip");
 if(await p.locator("[data-glance-lead]").count())throw Error("duplicate opening summary");
 if(await p.locator("[data-glance-answers]").count())throw Error("duplicate answers");
 const geometry=await p.evaluate(()=>({navLines:new Set([...document.querySelectorAll('[data-home-tabs] button')].map(e=>Math.round(e.getBoundingClientRect().y))).size,queueHeight:document.querySelector('[data-queue-tabs]').parentElement.getBoundingClientRect().height}));
 if(layout!=='merged'&&geometry.navLines!==1)throw Error('top navigation wraps '+width);
 if(layout==='merged'&&geometry.queueHeight>92)throw Error('merged row too tall '+width+' '+JSON.stringify(geometry));
 console.log(layout,width,JSON.stringify(geometry));
 if(await p.evaluate(()=>[...document.querySelectorAll("[data-home-tabs],[data-queue-tabs]")].some(n=>n.scrollWidth>n.clientWidth)))throw Error("scrolling tabs");
 if(overflow)throw Error('page overflow '+layout+' '+width);
 await p.locator('[data-project-filter-trigger]').click();
 const popup=await p.locator('[data-project-filter-popup]').boundingBox();if(!popup||popup.x<0||popup.x+popup.width>width)throw Error('filter popup clipped');
 await p.getByRole('option',{name:/Example/}).click();await p.getByRole('tab',{name:'Needs you now (0)',exact:true}).waitFor();
 if(layout==='merged'){
  if(await p.locator('[data-queue-tabs]').evaluate(e=>e.parentElement.getBoundingClientRect().height)>92)throw Error('filtered merged row too tall');
  await p.locator('[data-home-overflow] summary').click();
  for(const name of ['Ideas','Learn','Archive','Vizier','Settings'])await p.getByRole('button',{name,exact:true}).waitFor();
  await p.getByRole('button',{name:'Learn',exact:true}).click();await p.waitForSelector('[data-learn-panel]');
  await p.getByRole('tab',{name:'Needs you now (0)',exact:true}).click();await p.waitForSelector('[data-queue-section], [data-decision], [data-queue-tabs]');
 }
 await p.getByRole('button',{name:layout==='compact'?'C · Wrapped':'A · Compact',exact:true}).click();await p.waitForURL('**headerLayout='+ (layout==='compact'?'wrap':'compact'));
 await p.close();
}
const p=await browser.newPage({viewport:{width:1440,height:900}});await p.goto(base);
await p.getByRole('button',{name:'Learn',exact:true}).click();await p.getByRole('button',{name:'Got it',exact:true}).click();await p.waitForSelector('[data-learn-mark="got"]');await p.screenshot({path:dest+'/learn.png'});
await p.getByRole('button',{name:/^Ideas/}).click();await p.locator('[data-idea-project]').fill('Example');await p.getByRole('option',{name:'Example',exact:false}).first().click();await p.locator('[data-idea-text]').fill('Try a smaller sample page');await p.locator('[data-idea-file]').click();await p.waitForSelector('[data-idea="sample"]');await p.screenshot({path:dest+'/ideas.png'});
await p.getByRole('button',{name:'Vizier',exact:true}).click();await p.waitForSelector('[data-tell-vizier]');await p.screenshot({path:dest+'/vizier.png'});
await p.goto(base+'/map-before.html');await p.waitForSelector('[role=tablist]');await p.getByRole('tab',{name:'allocation'}).click();await p.screenshot({path:dest+'/map-before.png'});
console.log('Captured layouts and tab evidence; no page overflow, toggle, Learn marking and idea filing verified.');
}finally{await browser.close()}
