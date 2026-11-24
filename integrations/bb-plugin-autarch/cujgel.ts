// Metadata only. Keep validation in parity with internal/homeask/cujgel.go.
export interface CujgelLink {
  project: string;
  comparison_id: string;
  question_id: string;
  comparison_rev: { revision: number; sha256: string };
  variants: { variant_id: string; option_id: string }[];
}

// JS $ also matches before a final newline. These patterns require end of input.
const ID = /^[A-Za-z0-9][A-Za-z0-9_-]{0,127}(?![\s\S])/;
const HASH = /^[a-f0-9]{64}(?![\s\S])/;
const OPTION = /^[a-z0-9][a-z0-9._-]{0,127}(?![\s\S])/;
const CARD_OPTION = /^[a-z0-9-]{1,32}(?![\s\S])/;
const fail = (field: string): never => { throw new Error(`invalid cujgel/v1 metadata: ${field}`); };
class NumberToken { constructor(readonly text: string) {} }

// Syntax is checked first. Retain original number spellings and decoded object
// keys to detect duplicate keys and fractions that JSON.parse would round away.
function strictJSON(raw: string): unknown {
  try { JSON.parse(raw); } catch { return fail("JSON"); }
  const tokens = raw.match(/"(?:[^"\\]|\\.)*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|true|false|null|[{}\[\]:,]/g)!;
  let i=0;
  const value = (): unknown => {
    const token=tokens[i++]!;
    if(token==="{") {
      const out:Record<string,unknown>=Object.create(null);
      while(tokens[i]!=="}") {
        const key=JSON.parse(tokens[i++]!) as string;i++;
        if(Object.hasOwn(out,key)) fail("duplicate key");
        out[key]=value();
        if(tokens[i]!==",") break;i++;
      }
      i++;return out;
    }
    if(token==="[") {
      const out:unknown[]=[];
      while(tokens[i]!=="]") { out.push(value());if(tokens[i]!==",") break;i++; }
      i++;return out;
    }
    return /^-?\d/.test(token) ? new NumberToken(token) : JSON.parse(token);
  };
  return value();
}

function object(v:unknown,required:string[],field:string):Record<string,unknown> {
  if(!v || typeof v!=="object" || Array.isArray(v) || v instanceof NumberToken) return fail(field);
  const m=v as Record<string,unknown>;
  if(required.some(k=>!Object.hasOwn(m,k)) || Object.keys(m).some(k=>!required.includes(k))) fail(field);
  return m;
}
function text(v:unknown,pattern:RegExp,field:string):string {
  if(typeof v!=="string" || !pattern.test(v)) return fail(field);
  return v;
}
function integer(v:unknown):NumberToken {
  const field="comparison_rev.revision";
  if(!(v instanceof NumberToken) || !Number.isFinite(Number(v.text)) || Number(v.text)<1) return fail(field);
  const m=/^(\d+)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/.exec(v.text);
  if(!m) return fail(field);
  const digits=m[1]!+(m[2]??"");
  const shift=Number(m[3]??0)-(m[2]?.length??0);
  if(shift<0 && (-shift>digits.length || /[^0]/.test(digits.slice(shift)))) return fail(field);
  const whole=(shift>=0 ? digits+"0".repeat(shift) : digits.slice(0,shift)).replace(/^0+/,"");
  if(!whole) return fail(field);
  return new NumberToken(whole);
}
function canonical(v:unknown):string {
  if(v instanceof NumberToken) return v.text;
  if(Array.isArray(v)) return "["+v.map(canonical).join(",")+"]";
  if(v!==null && typeof v==="object") return "{"+Object.keys(v).sort().map(k=>JSON.stringify(k)+":"+canonical((v as Record<string,unknown>)[k])).join(",")+"}";
  return JSON.stringify(v);
}

/** Canonical bytes retain exact integral revisions; the object is for JS display. */
export function parseCujgel(raw:string,options:unknown):{link:CujgelLink;json:string} {
  if(Buffer.byteLength(raw,"utf8")>16384) fail("16 KiB limit");
  const m=object(strictJSON(raw),["project","comparison_id","question_id","comparison_rev","variants"],"block");
  for(const key of ["project","comparison_id","question_id"]) text(m[key],ID,key);
  if(m.question_id!==m.comparison_id) fail("question_id");
  const rev=object(m.comparison_rev,["revision","sha256"],"comparison_rev");
  rev.revision=integer(rev.revision);text(rev.sha256,HASH,"comparison_rev.sha256");
  if(!Array.isArray(m.variants) || m.variants.length<2 || m.variants.length>4) fail("variants");
  const ids=new Set<string>();
  const cardOptions=new Set(Array.isArray(options) ? options.map(v=>v?.id) : []);
  for(const raw of m.variants as unknown[]) {
    const variant=object(raw,["variant_id","option_id"],"variants");
    const id=text(variant.variant_id,ID,"variant_id"),option=text(variant.option_id,OPTION,"option_id");
    if(ids.has(id)) fail("variant_id");ids.add(id);
    if(!CARD_OPTION.test(option) || !cardOptions.has(option)) fail("option_id");
  }
  const json=canonical(m);
  return {link:JSON.parse(json) as CujgelLink,json};
}
