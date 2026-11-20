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
 expect(html).toContain("Projects •");
 expect(html).not.toContain("data-project-filter-hidden");
 expect(html).toContain('aria-label="Project filter: 0 of 1 projects"');
});
