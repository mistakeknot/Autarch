import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import * as links from "../ui/cardlink.js";
import { AskCard, type OwedAsk } from "../ui/asks.js";
import * as asksUI from "../ui/asks.js";
import { BlocksRow } from "../ui/blocks.js";

export const fakeAsk:OwedAsk={id:"fixture-decision",key:"FAKE-1",project:"fixture",thread:"thr_fixture",subject:"Compare options",asker:"thread",filed_at:"2030-01-01T00:00:00Z",revision:"fixture-revision",ask:{question:"Which variant?",options:[{id:"first",label:"First variant",kind:"ruling-only"},{id:"second",label:"Second variant",kind:"ruling-only"}]}};
const metadata={project:"proj_fixture",comparison_id:"Comparison_fixture",question_id:"Comparison_fixture",comparison_rev:{revision:1,sha256:"a".repeat(64)},variants:[{variant_id:"first",option_id:"first"},{variant_id:"second",option_id:"second"}]};

describe("card option targets",()=>{
  it("keeps every legacy cardKeyFrom result",()=>{
    for(const [path,key] of [[undefined,null],["",null],["fake-1","FAKE-1"],["/FAKE-1/","FAKE-1"],["%2FFAKE-1%2F","FAKE-1"],["FAKE-1/extra",null],["FAKE-1/option/first",null],["FAKE-1%0A",null],["%ZZ",null]] as const)
      expect(links.cardKeyFrom(path)).toBe(key);
  });
  it("exports a validated helper next to cardHref",()=>{
    const href=(links as any).cardOptionHref;
    expect(typeof href).toBe("function");
    expect(href("FAKE-1","first")).toBe("/plugins/autarch/home/FAKE-1/option/first");
    for(const bad of ["First","first\n","../first","a".repeat(33),""]) expect(()=>href("FAKE-1",bad)).toThrow();
  });
  it("opens the card even if the requested option has invalid syntax",()=>{
    const parse=(links as any).cardOpenTargetFrom;
    expect(typeof parse).toBe("function");
    expect(parse("FAKE-1")).toEqual({key:"FAKE-1"});
    expect(parse("/fake-1/option/first/")).toEqual({key:"FAKE-1",option:"first"});
    for(const option of ["unknown","First","first%0A","%ZZ",""]) {
      const target=parse(`FAKE-1/option/${option}`);expect(target?.key).toBe("FAKE-1");
      expect(target).toHaveProperty("option");
    }
    expect(parse("../option/first")).toBeNull();
  });
  it("does not normalize an encoded slash into a valid option id",()=>{
    const parse=(links as any).cardOpenTargetFrom,choose=(links as any).cardOptionSelection;
    expect(typeof parse).toBe("function");expect(typeof choose).toBe("function");
    const target=parse("FAKE-1/option/first%2F");expect(target?.key).toBe("FAKE-1");
    expect(choose(target.option,fakeAsk).option).toBeNull();
  });
  it("preselects only an ordinary option of an open card",()=>{
    const choose=(links as any).cardOptionSelection;
    expect(typeof choose).toBe("function");
    expect(choose("first",fakeAsk)).toEqual({option:"first",notice:null});
    for(const [option,card] of [["unknown",fakeAsk],["first\n",fakeAsk],["first",null],["first",{...fakeAsk,held:{reason:"fixture",by:"fixture",at:"fixture"}}],["first",{...fakeAsk,unreadable:true}],["first",{...fakeAsk,ask:{...fakeAsk.ask,options:[{...fakeAsk.ask.options[0],approval:{kind:"merge"}}]}}],["first",{...fakeAsk,ask:{...fakeAsk.ask,options:[{...fakeAsk.ask.options[0],on_pick:{}}]}}]] as const) {
      const result=choose(option,card);expect(result.option).toBeNull();expect(result.notice).toBeTruthy();
    }
  });
});

describe("Cujgel open adapter",()=>{
  async function adapter() {
    expect(existsSync(new URL("../ui/cujgelopen.ts",import.meta.url)),"one explicit navigation adapter").toBe(true);
    return import("../ui/cujgelopen.js");
  }
  it("defaults to missing, with no route or probe",async()=>{
    const mod=await adapter();expect(mod.CUJGEL_PLUGIN_ID).toBeNull();
    expect(mod.openTarget(metadata)).toEqual({status:"missing",href:null});
  });
  it("derives the pinned route only from ids and an injected plugin id",async()=>{
    const mod=await adapter();
    expect(mod.openTarget(metadata,"fixture plugin")).toEqual({status:"available",href:"/plugins/fixture%20plugin/journeys/proj_fixture/Comparison_fixture"});
    expect(mod.openTarget({...metadata,project:"../fixture"},"fixture-plugin")).toEqual({status:"missing",href:null});
  });
  it("shows the missing state with the normal answer controls",()=>{
    const pick=vi.fn();const html=renderToStaticMarkup(<AskCard ask={{...fakeAsk,cujgel:metadata} as OwedAsk} onPick={pick} onOpen={()=>{}} />);
    expect(html).toContain("Open comparison in Cujgel");expect(html).toContain("Cujgel is not available yet; answer here.");
    expect(html).not.toContain("/journeys/");expect(html).toContain('data-option="first"');expect(pick).not.toHaveBeenCalled();
  });
  it("renders an injected available target as a plain link",()=>{
    const Open=(asksUI as any).CujgelOpen;expect(typeof Open).toBe("function");
    const html=renderToStaticMarkup(<Open link={metadata} pluginId="fixture-plugin" />);
    expect(html).toContain('href="/plugins/fixture-plugin/journeys/proj_fixture/Comparison_fixture"');
    expect(html).toContain("Open comparison in Cujgel");expect(html).not.toContain("<button");
  });
  it("carries the generation snapshot from a queue row to its card metadata",()=>{
    const row={id:"fixture",decision_id:"fixture-decision",task_id:"fixture-task",card_key:"FAKE-1",binding_state:"confirmed",project:"fixture",title:"Compare",refs:[],blocks_count:0,created_at:"2030-01-01",thread:"thr_fixture",pinned:false,ask:fakeAsk.ask,revision:"fixture",mentions:0,display_reason:null,display_only:false,overrides_generation:null,changed_after_ruling:false,root:{state:"verified",reason:null},cujgel:metadata};
    const html=renderToStaticMarkup(<BlocksRow row={row as any} nowMs={Date.parse("2030-01-01")} onPick={()=>{}} onOpen={()=>{}} />);
    expect(html).toContain("Cujgel is not available yet; answer here.");
  });
  it("leaves an unlinked card's rendered bytes unchanged",()=>{
    const html=renderToStaticMarkup(<AskCard ask={fakeAsk} onPick={()=>{}} onOpen={()=>{}} />);
    expect(createHash("sha256").update(html).digest("hex")).toBe("b9178192114edeaa7a69d5535d7daccc5a59e604d9fcbeae0e5587da5c3511a8");
  });
  it("keeps the older card text free of the metadata fence",async()=>{
    const {parseCard}=await import("../cards.js");
    const desc='Which variant?\nRequest: fixture sha256:aaaaaaaaaaaaaaaa\n```home-ask\n'+JSON.stringify({schema:"home-ask/v2",project:"fixture",project_root:"/fixture",question:"Which variant?",options:fakeAsk.ask.options})+'\n```\n```cujgel/v1\n'+JSON.stringify(metadata)+'\n```\n';
    expect(parseCard(desc).question).toBe("Which variant?");
  });
});
