// @vitest-environment jsdom
import { expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { HomeTabs, HomeOverflow, QueueTabs } from "../ui/tabs.js";
import { ProjectFilterBar } from "../ui/projectfilter.js";
it("merges Queue navigation with all other destinations in More",()=>{
 const html=renderToStaticMarkup(<HomeTabs extra={<span>Update available</span>} queue={<QueueTabs tab="decide" counts={{decide:2,later:8,blocks:7,catchup:91}} onTab={()=>{}} more={<HomeOverflow learn onOpen={()=>{}} definition="Counts follow the project filter."/>}/>}/>);
 expect(html).not.toContain("data-home-tabs");
 expect(html).toContain("data-home-extra");
 expect(html).toContain("Update available");
 expect(html.match(/role="tab"/g)).toHaveLength(4);
 expect(html).toContain("data-home-overflow");
 for(const title of ["Ideas","Learn","Archive","Vizier","Settings"])expect(html).toContain(`>${title}</button>`);
 expect(html).toContain("Counts follow the project filter.");
});
it("the compact filter keeps its current selection accessible",()=>{
 const html=renderToStaticMarkup(<ProjectFilterBar compact choices={[{name:"Example",count:2}]} excluded={new Set()} hidden={0} onChange={()=>{}}/>);
 expect(html).toContain('aria-label="Project filter: All projects"');
 expect(html).toContain(">Projects</span>");
});
it("does not label a Queue tab selected while another destination is open",()=>{
 const html=renderToStaticMarkup(<QueueTabs active={false} tab="decide" counts={null} onTab={()=>{}}/>);
 expect(html).not.toContain('aria-selected="true"');
 expect(html).not.toContain('aria-controls="queue-tabpanel"');
});

it("keeps a filtered compact header short while marking the active filter",()=>{
 const html=renderToStaticMarkup(<ProjectFilterBar compact choices={[{name:"Example",count:2}]} excluded={new Set(["Example"])} hidden={2} onChange={()=>{}}/>);
 expect(html).toContain("Projects · 2 hidden");
 expect(html).not.toContain("data-project-filter-hidden");
 expect(html).toContain('aria-label="Project filter: 0 of 1 projects"');
});

it("names and marks the active More destination", () => {
 const host = document.createElement("div");
 host.innerHTML = renderToStaticMarkup(<HomeOverflow learn active="archive" onOpen={()=>{}}/>);
 expect(host.querySelector("summary")?.textContent).toBe("More · Archive");
 expect(host.querySelector("summary")?.className).toContain("font-semibold");
 expect(host.querySelector('[aria-current="page"]')?.textContent).toBe("Archive");
 expect(host.querySelectorAll('[aria-current="page"]')).toHaveLength(1);
});

it.each([false,true])("visible and accessible Queue names match at phone=%s",phone=>{
 const host=document.createElement("div");
 host.innerHTML=renderToStaticMarkup(<QueueTabs phone={phone} tab="decide" counts={{decide:2,later:3,blocks:4,catchup:5}} onTab={()=>{}}/>);
 const buttons=Array.from(host.querySelectorAll('[role="tab"]'));
 expect(buttons.map(b=>b.textContent)).toEqual(phone?["Now (2)","Later (3)","Blocked (4)","Since left (5)"]:["Needs you now (2)","Later (3)","Blocked on others (4)","Since you left (5)"]);
 for(const button of buttons)expect(button.getAttribute("aria-label")).toBe(button.textContent);
});
it.each([0,3,undefined])("Ideas shows its readable open count (%s)",ideas=>{
 const html=renderToStaticMarkup(<HomeOverflow ideas={ideas} onOpen={()=>{}}/>);
 expect(html).toContain(ideas===undefined?">Ideas</button>":`>Ideas (${ideas})</button>`);
});

it("keeps compact header spacing, keyboard-only rings and deterministic active colors",()=>{
 const host=document.createElement("div");
 host.innerHTML=renderToStaticMarkup(<QueueTabs tab="decide" counts={null} onTab={()=>{}} filter={<ProjectFilterBar compact choices={[{name:"Example",count:1}]} excluded={new Set()} hidden={0} onChange={()=>{}}/>} more={<HomeOverflow active="archive" onOpen={()=>{}}/>}/>);
 const projects=host.querySelector('[data-project-filter-trigger]')!;
 const more=host.querySelector('summary')!;
 expect(projects.textContent).not.toContain("▾");
 for(const control of [...Array.from(host.querySelectorAll('[role="tab"]')),projects,more]){
  expect(control.className).toContain("focus-visible:ring-2");
  expect(control.className).not.toMatch(/(?:^|\s)focus:ring-/);
  expect(control.className).toContain("min-h-11");
 }
 for(const control of [projects,more])expect(control.className.split(" ")).toContain("px-2");
 const selected=host.querySelector('[aria-selected="true"]')!;
 expect(selected.className).toContain("bg-state-active");
 expect(more.className).toContain("bg-muted");
 expect(more.className.split(" ")).toContain("text-foreground");
 for(const control of [selected,more])expect(control.className.split(" ")).not.toContain("bg-transparent");
});
