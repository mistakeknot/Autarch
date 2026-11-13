import { useEffect, useState } from "react";
import { useRpc } from "@get-bb/plugin-sdk/app";
import type { rpcContract } from "../server.js";
import type { Evidence, EvidenceImage, EvidenceSide } from "../evidence.js";

function LocalImage({ image, caption }: { image: EvidenceImage; caption: string }) {
 const rpc = useRpc<typeof rpcContract>();
 const [loaded, setLoaded] = useState<{ key: string; url: string } | null>(null);
 const key = `${image.sha256}:${image.mime}`;
 const src = loaded?.key === key ? loaded.url : null;
 useEffect(() => {
  let active = true;
  let url: string | undefined;
  rpc.call("evidenceImage", { sha256: image.sha256 }).then(raw => {
   const r = raw as { ok: boolean; mime?: string; data_base64?: string };
   if (!active || !r.ok || r.mime !== image.mime || !r.data_base64) return;
   const bytes = Uint8Array.from(atob(r.data_base64), c => c.charCodeAt(0));
   url = URL.createObjectURL(new Blob([bytes], { type: image.mime }));
   setLoaded({ key: `${image.sha256}:${image.mime}`, url });
  }).catch(() => { /* An unavailable image stays a quiet placeholder. */ });
  return () => { active = false; if (url) URL.revokeObjectURL(url); };
 }, [rpc, image.sha256, image.mime]);
 return src ? <a href={src} target="_blank" rel="noopener noreferrer" aria-label={`Open full size: ${caption}`}><img src={src} alt={caption} className="max-h-48 max-w-full rounded object-contain" /></a> : <p className="m-0 text-xs text-muted-foreground">image unavailable</p>;
}
function Source({ source, caption }: { source: EvidenceSide; caption: string }) {
 if (source.url) return <a href={source.url} target="_blank" rel="noopener noreferrer" className="underline [overflow-wrap:anywhere]">{caption}<span className="ml-2 text-xs text-muted-foreground">{new URL(source.url).host}</span></a>;
 return source.image ? <LocalImage image={source.image} caption={caption} /> : null;
}
export function EvidenceBlock({ items }: { items?: Evidence[] }) {
 if (!items?.length) return null;
 return <section className="mt-3 space-y-2 text-sm" aria-label="Evidence" data-evidence>
  <h4 className="m-0 text-xs font-medium text-muted-foreground">Evidence</h4>
  {items.map((e,i) => <figure key={i} className="m-0 space-y-1">
   {e.url ? <Source source={e} caption={e.caption} /> : <><figcaption>{e.caption}</figcaption>{e.before && e.after ? <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,14rem),1fr))] gap-2"><div><p className="m-0 text-xs text-muted-foreground">Before</p><Source source={e.before} caption={`${e.caption}: before`} /></div><div><p className="m-0 text-xs text-muted-foreground">After</p><Source source={e.after} caption={`${e.caption}: after`} /></div></div> : <Source source={e} caption={e.caption} />}</>}
  </figure>)}
 </section>;
}
