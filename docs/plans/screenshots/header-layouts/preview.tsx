import { createRoot } from "react-dom/client";
import { useState } from "react";
import { HomeTabs, HomeOverflow, QueueTabs, type QueueTab } from "../../../../integrations/bb-plugin-autarch/ui/tabs";
import { AsksPanel } from "../../../../integrations/bb-plugin-autarch/ui/asks";
import { ProjectFilterBar } from "../../../../integrations/bb-plugin-autarch/ui/projectfilter";
import { LearnPanel } from "../../../../integrations/bb-plugin-autarch/ui/learn";
import { IdeaDesk, type IdeaApi } from "../../../../integrations/bb-plugin-autarch/ui/ideas";
import { TellVizier } from "../../../../integrations/bb-plugin-autarch/ui/vizier";
import { asksData } from "../home-ux/harness/fixtures";
const data={...asksData,owed:asksData.owed.slice(0,2).map((o,i)=>({...o,id:`sample-${i}`,key:`EX-${i+1}`,project:"Example",subject:i?"Choose the next review":"Playtest: sample page",ask:{...o.ask,options:[{id:"a",label:"Ready to continue",kind:"instruction",instruction:"Continue with the reviewed sample."},{id:"b",label:"Needs changes",kind:"instruction",instruction:"Revise the sample labels before continuing."}],question:"What this is: a sample page ready for review.\n\nOpen https://sample.example.test and check whether the labels are readable."}}))};
function Preview(){
 const [tab,setTab]=useState("queue"),[queue,setQueue]=useState<QueueTab>("decide"),[mark,setMark]=useState<"got"|"later"|null>(null),[notice,setNotice]=useState("");
 const [excluded,setExcluded]=useState<ReadonlySet<string>>(new Set());
 const [api]=useState(()=>{let ideas:any[]=[];return {projects:async()=>[{id:"sample",name:"Example",prefix:"EX"}],digest:async()=>({ideas,show:ideas.length>0}),file:async(_p:string,text:string)=>{ideas=[...ideas,{task_id:"sample",project:"Example",key:"EX-3",words:text}];return {ok:true}},act:async()=>{ideas=[];return {ok:true}},clear:async()=>{ideas=[];}} as IdeaApi;});
 const layout=new URLSearchParams(location.search).get("headerLayout")??"wrap";
 const navigation=(merged=false)=>( <QueueTabs active={tab==="queue"} tab={queue} onTab={next=>{setQueue(next);setTab("queue")}} more={merged?<HomeOverflow learn onOpen={p=>setTab(p.id)}/>:null} counts={{decide:excluded.has("Example")?0:2,later:8,blocks:7,catchup:91}} filter={<ProjectFilterBar compact={merged} choices={[{name:"Example",count:2}]} excluded={excluded} hidden={excluded.has("Example")?2:0} onChange={setExcluded}/>}/> );
 return <main>
 <aside className="flex flex-wrap items-center gap-3 border-b border-border p-3 text-sm" aria-label="Preview layouts"><strong>Layout</strong>{[["compact","A · Compact"],["merged","B · Merged"],["wrap","C · Wrapped"]].map(([value,label])=><button aria-label={label} title={label} aria-pressed={(new URLSearchParams(location.search).get("headerLayout")??"wrap")===value} className="rounded border border-border px-3 py-2 aria-pressed:bg-muted" key={value} onClick={()=>{const u=new URL(location.href);u.searchParams.set("headerLayout",value!);location.href=u.href}}>{label?.slice(0,1)}</button>)}</aside>
 <HomeTabs learn mergedQueue={navigation(true)} onOpen={p=>setTab(p.id)}/>
 {tab==="queue"?<>{layout!=="merged"?navigation():null}{queue==="decide"?<AsksPanel data={{...data,owed:excluded.has("Example")?[]:data.owed} as never} part="decide" nowMs={Date.parse("2026-10-08T00:00:00Z")} onPick={()=>setNotice("Fixture answer recorded")} onOpen={()=>setNotice("Fixture thread opened")}/>:<p className="p-4">{queue==="later"?"Set-aside sample requests":queue==="blocks"?"Sample work waiting on another person":"Sample changes since your last visit"}</p>}</>:tab==="learn"?<LearnPanel cards={[{id:"sample",title:"Reading a decision",body:"Check the evidence, then choose an answer.",mark}]} onMark={(_id,m)=>setMark(m==="clear"?null:m)}/>:tab==="ideas"?<IdeaDesk api={api}/>:tab==="vizier"?<TellVizier threadId="sample-thread"/>:<p className="p-4">{tab==="archive"?"No archived sample cards.":"Preview settings. Learn is enabled for this fixture."}</p>}
 <p role="status" className="px-4">{notice}</p><p className="px-4 text-xs text-muted-foreground">Fixture data; actions stay in this page. Host chat is a fixture attachment.</p></main>
}createRoot(document.getElementById("root")!).render(<Preview/>);
