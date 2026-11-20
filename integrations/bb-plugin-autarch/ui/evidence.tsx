import { useCallback, useEffect, useState } from "react";
import { EvidenceLightbox, type LightboxImage } from "./evidence-lightbox.js";
import { useRpc } from "@get-bb/plugin-sdk/app";
import type { rpcContract } from "../server.js";
import type { Evidence, EvidenceImage, EvidenceSide } from "../evidence.js";

function LocalImage({ image, caption, id, register, open }: { image: EvidenceImage; caption: string; id: string; register: (id: string, url: string | null) => void; open: (id: string) => void }) {
 const rpc = useRpc<typeof rpcContract>();
 const [loaded, setLoaded] = useState<{ key: string; url: string } | null>(null);
 const [failed, setFailed] = useState<string | null>(null);
 const key = `${image.sha256}:${image.mime}`;
 const src = failed !== key && loaded?.key === key ? loaded.url : null;
 useEffect(() => {
  let active = true;
  let url: string | undefined;
  rpc.call("evidenceImage", { sha256: image.sha256 }).then(raw => {
   const r = raw as { ok: boolean; mime?: string; data_base64?: string };
   if (!active || !r.ok || r.mime !== image.mime || !r.data_base64) return;
   const bytes = Uint8Array.from(atob(r.data_base64), c => c.charCodeAt(0));
   url = URL.createObjectURL(new Blob([bytes], { type: image.mime }));
   register(id, url);
   setLoaded({ key: `${image.sha256}:${image.mime}`, url });
  }).catch(() => { /* An unavailable image stays a quiet placeholder. */ });
  return () => { active = false; if (url) { register(id, null); URL.revokeObjectURL(url); } };
 }, [rpc, image.sha256, image.mime, id, register]);
 return src ? <button type="button" aria-label={`Enlarge: ${caption}`} className="block max-w-full cursor-zoom-in rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" onClick={() => open(id)}><img src={src} alt={caption} onError={() => { setFailed(key); register(id, null); }} className="max-h-48 max-w-full rounded-md object-contain" /></button> : <p className="m-0 text-xs text-muted-foreground">image unavailable</p>;
}
export function EvidenceBlock({ items }: { items?: Evidence[] }) {
 const [urls,setUrls]=useState<Record<string,string>>({}), [current,setCurrent]=useState<string | null>(null);
 const register=useCallback((id:string,url:string|null)=>{
  if (!url) setCurrent(previous => previous === id ? null : previous);
  setUrls(prev=>{const next={...prev};if(url)next[id]=url;else delete next[id];return next;});
 },[]);
 const images: LightboxImage[]=[];
 const source=(value:EvidenceSide,caption:string,path:string)=>{
  if(value.url)return <a href={value.url} target="_blank" rel="noopener noreferrer" className="underline [overflow-wrap:anywhere]">{caption}<span className="ml-2 text-xs text-muted-foreground">{new URL(value.url).host}</span></a>;
  if(!value.image)return null;
  const id=`${path}:${value.image.sha256}:${value.image.mime}`;
  if(urls[id])images.push({id,caption,url:urls[id]});
  return <LocalImage image={value.image} caption={caption} id={id} register={register} open={setCurrent}/>;
 };
 if (!items?.length) return null;
 const figures=items.map((e,i)=><figure key={i} className="m-0 space-y-1">
  {e.url?source(e,e.caption,`${i}`):<><figcaption>{e.caption}</figcaption>{e.before&&e.after?<div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,14rem),1fr))] gap-2"><div><p className="m-0 text-xs text-muted-foreground">Before</p>{source(e.before,`${e.caption}: Before`,`${i}:before`)}</div><div><p className="m-0 text-xs text-muted-foreground">After</p>{source(e.after,`${e.caption}: After`,`${i}:after`)}</div></div>:source(e,e.caption,`${i}`)}</>}
 </figure>);
 return <section className="mt-3 space-y-2 text-sm" aria-label="Evidence" data-evidence>
  <h4 className="m-0 text-xs font-medium text-muted-foreground">Evidence</h4>{figures}
  {current&&images.some(i=>i.id===current)?<EvidenceLightbox images={images} current={current} onChange={setCurrent} onClose={()=>setCurrent(null)}/>:null}
 </section>;
}
