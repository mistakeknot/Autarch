import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { cardFingerprint, parseCard } from "../cards.js";

const fixture = (name: string) => readFileSync(new URL(`../../../internal/homeask/testdata/cujgel/${name}`,import.meta.url),"utf8");
const example=fixture("citadel-link.json");
const negatives=JSON.parse(fixture("negative-cases.json")) as {name:string;body:string}[];
const options=[{id:"option-a",label:"First variant",kind:"ruling-only"},{id:"option-b",label:"Second variant",kind:"ruling-only"}];
function desc(body=example) { return 'Which variant?\n\nRequest: fixture sha256:aaaaaaaaaaaaaaaa\n\n```home-ask\n'+JSON.stringify({schema:"home-ask/v2",project:"fixture",project_root:"/fixture",question:"Which variant?",options})+'\n```\n\n```cujgel/v1\n'+body+'\n```\n'; }

describe("cujgel/v1 card metadata parity",()=>{
  it("reads the vendored example as metadata, never prose",()=>{
    const card=parseCard(desc());
    expect((card as any).cujgel).toEqual(JSON.parse(example));
    expect(card.question).toBe("Which variant?");
  });
  for(const tc of negatives) it(`refuses ${tc.name}`,()=>expect(()=>parseCard(desc(tc.body))).toThrow());
  it("refuses duplicate or unterminated blocks",()=>{
    expect(()=>parseCard(desc()+"\n```cujgel/v1\n"+example+"\n```\n")).toThrow(/more than one cujgel/);
    expect(()=>parseCard(desc().slice(0,-5))).toThrow(/unterminated cujgel/);
  });
  it("refuses a secret form combined with a comparison",()=>{
    const metadata={thread:"thr_23456789ab",names:["FAKE_TEST_TOKEN"],path:"/fixture/credentials.env",project:"fixture"};
    expect(()=>parseCard(desc()+"\n```home-secret\n"+JSON.stringify(metadata)+"\n```\n")).toThrow(/combined with.*secret/);
  });
  it("accepts uppercase IDs and four variants",()=>{
    const m=JSON.parse(example);m.comparison_id=m.question_id="Upper_ID";
    m.variants.push({variant_id:"Variant_C",option_id:"option-a"},{variant_id:"Variant_D",option_id:"option-b"});
    expect((parseCard(desc(JSON.stringify(m))) as any).cujgel).toEqual(m);
  });
  it("fingerprints link-only edits without changing unlinked fingerprints",()=>{
    const card=parseCard(desc());const data={title:"Compare",card,thread:"thr_fixture",tasksProject:"fixture"};
    const first=cardFingerprint(data);
    const m=JSON.parse(example);m.comparison_rev.sha256="b".repeat(64);
    expect(cardFingerprint({...data,card:parseCard(desc(JSON.stringify(m)))})).not.toBe(first);
  });
});
