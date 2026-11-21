import { createRoot } from "react-dom/client";
import { useState } from "react";
import { HomeTabs, HomeOverflow, QueueTabs, type QueueTab } from "../../../../integrations/bb-plugin-autarch/ui/tabs";
import { PickHoldBar, usePickHold } from "../../../../integrations/bb-plugin-autarch/ui/pickhold";
import { QueueSurface, useIsPhone } from "../../../../integrations/bb-plugin-autarch/ui/inbox";
import { AsksPanel } from "../../../../integrations/bb-plugin-autarch/ui/asks";
import { ProjectFilterBar } from "../../../../integrations/bb-plugin-autarch/ui/projectfilter";
import { ArchivePanel, type ArchiveApi } from "../../../../integrations/bb-plugin-autarch/ui/archive";
import { SettingsPanel } from "../../../../integrations/bb-plugin-autarch/ui/settings";
import { LearnPanel } from "../../../../integrations/bb-plugin-autarch/ui/learn";
import { IdeaDesk, type IdeaApi } from "../../../../integrations/bb-plugin-autarch/ui/ideas";
import { TellVizier } from "../../../../integrations/bb-plugin-autarch/ui/vizier";
import { asksData } from "../home-ux/harness/fixtures";
const data={...asksData,owed:asksData.owed.slice(0,2).map((o,i)=>({...o,id:`sample-${i}`,key:`EX-${i+1}`,project:"Example",subject:i?"Choose the next review":"Playtest: sample page",ask:{...o.ask,options:[{id:"a",label:"Ready to continue",kind:"instruction",instruction:"Continue with the reviewed sample."},{id:"b",label:"Needs changes",kind:"instruction",instruction:"Revise the sample labels before continuing."}],question:"What this is: a sample page ready for review.\n\nOpen https://sample.example.test and check whether the labels are readable."}}))};
function Preview(){
 const phone=useIsPhone();
 const [ideaOpen,setIdeaOpen]=useState(false);
 const [tab,setTab]=useState("queue"),[queue,setQueue]=useState<QueueTab>("decide"),[mark,setMark]=useState<"got"|"later"|null>(null),[notice,setNotice]=useState("");
 const hold=usePickHold(()=>setNotice("Fixture answer recorded"),()=>true);
 const [ideaCount,setIdeaCount]=useState(0);
 const [excluded,setExcluded]=useState<ReadonlySet<string>>(new Set());
 const [api]=useState(()=>{let ideas:any[]=[];return {projects:async()=>[{id:"sample",name:"Example",prefix:"EX"}],digest:async()=>({ideas,show:ideas.length>0}),file:async(_p:string,text:string)=>{ideas=[...ideas,{task_id:"sample",project:"Example",key:"EX-3",words:text}];setIdeaCount(ideas.length);return {ok:true}},act:async()=>{ideas=[];setIdeaCount(0);return {ok:true}},clear:async()=>{ideas=[];}} as IdeaApi;});
 const [settings,setSettings]=useState({vizierThreadId:"sample-thread",projects:["Example"],dailyCap:0});
 const [archiveApi]=useState(()=>({load:async(f)=>{const row={task_id:"archived-sample",card_key:"EX-4",title:"Sample review complete",project:"Example",state:"ruled",decided_at:"2026-10-08T00:00:00Z",pick_label:"Ready to continue",option_id:"a",by:"mk",reason:null,generation:1,decision_id:"sample-choice",no_pick:false};const rows=(row.title+row.card_key).toLowerCase().includes(f.search.toLowerCase())?[row]:[];return {rows,total:rows.length,projects:["Example"]}},bringBack:async()=>{setNotice("Fixture archive question reopened");return null}} as ArchiveApi));
 const navigation=()=>( <QueueTabs phone={phone} active={tab==="queue"} tab={queue} onTab={next=>{setQueue(next);setTab("queue")}} more={<HomeOverflow ideas={ideaCount} active={tab} learn onOpen={p=>setTab(p.id)}/>} counts={{decide:2,later:8,blocks:7,catchup:91}} filter={<ProjectFilterBar compact choices={[{name:"Example",count:2}]} excluded={excluded} hidden={excluded.has("Example")?2:0} onChange={setExcluded}/>}/> );
 return <main className="flex h-dvh min-h-0 flex-col">
 <HomeTabs queue={navigation()}/>
 <PickHoldBar hold={hold}/>
 {tab==="queue"?<>{queue==="decide"?<QueueSurface phone={phone} inbox={{pickHold:hold,asks:{...data,owed:excluded.has("Example")?[]:data.owed} as never,waiting:2,onPick:()=>setNotice("Fixture answer recorded"),onOpen:()=>{},onNote:()=>{},onLater:()=>{},sinceNode:null,sinceCount:91,onIdea:()=>setIdeaOpen(v=>!v),ideaNode:ideaOpen?<IdeaDesk api={api}/>:null}}><AsksPanel data={{...data,owed:excluded.has("Example")?[]:data.owed} as never} part="decide" nowMs={Date.parse("2026-10-08T00:00:00Z")} onPick={()=>setNotice("Fixture answer recorded")} onOpen={()=>setNotice("Fixture thread opened")}/></QueueSurface>:<p className="p-4">{queue==="later"?"Set-aside sample requests":queue==="blocks"?"Sample work waiting on another person":"Sample changes since your last visit"}</p>}</>:tab==="learn"?<LearnPanel cards={[{id:"sample",title:"Reading a decision",body:"Check the evidence, then choose an answer.",mark}]} onMark={(_id,m)=>setMark(m==="clear"?null:m)}/>:tab==="ideas"?<IdeaDesk api={api}/>:tab==="vizier"?<TellVizier threadId={settings.vizierThreadId}/>:tab==="archive"?<ArchivePanel api={archiveApi}/>:<SettingsPanel delegation={{settings,suspended:true} as never} machineOwners={{}} onSave={value=>{setSettings(value);setNotice("Fixture settings saved")}}/>}
 <p role="status" className="px-4">{notice}</p><p className="px-4 text-xs text-muted-foreground">Fixture data; actions stay in this page. Host chat is a fixture attachment.</p></main>
}createRoot(document.getElementById("root")!).render(<Preview/>);
