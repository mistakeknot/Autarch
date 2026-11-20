import {createRequire} from 'node:module';
import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
const root=process.cwd(),out=process.env.OUT;if(!out||!process.env.BB_BUILD)throw Error('OUT and BB_BUILD required');mkdirSync(out,{recursive:true});
const nm=resolve(root,'integrations/bb-plugin-autarch/node_modules');
const {build}=createRequire(nm+'/x.js')('esbuild');
await build({entryPoints:['docs/plans/screenshots/header-layouts/preview.tsx'],bundle:true,minify:true,outfile:out+'/preview.js',jsx:'automatic',define:{'process.env.NODE_ENV':'"production"'},plugins:[{name:'fixture-chat',setup(b){b.onResolve({filter:/^@get-bb\/plugin-sdk\/app$/},()=>({path:'chat',namespace:'fixture'}));b.onLoad({filter:/.*/,namespace:'fixture'},()=>({contents:'export function useRpc(){return {call:async()=>({ok:true})}} export function ThreadChat(){return <div className="p-4">Host thread chat attachment (fixture). Messages are not sent from this preview.</div>}',loader:'jsx',resolveDir:nm}));}}],nodePaths:[nm]});
const req=createRequire(process.env.BB_BUILD+'/package.json');const{compile}=req('@tailwindcss/node');const{Scanner}=req('@tailwindcss/oxide');const compiler=await compile(readFileSync('docs/plans/screenshots/home-ux/harness/theme.css','utf8'),{base:process.env.BB_BUILD,onDependency(){}});const scanner=new Scanner({sources:[{base:root+'/integrations/bb-plugin-autarch/ui',pattern:'**/*',negated:false},{base:root+'/docs/plans/screenshots/header-layouts',pattern:'**/*',negated:false}]});writeFileSync(out+'/theme.css',compiler.build(scanner.scan()));writeFileSync(out+'/index.html','<!doctype html><meta name="viewport" content="width=device-width, initial-scale=1"><meta charset="utf-8"><title>Citadel header options</title><link rel="stylesheet" href="theme.css"><div id="root"></div><script src="preview.js"></script>');

// Render the removed baseline component too, so its empty behavior is reviewable.
writeFileSync(out+'/map-before.tsx',execFileSync('git',['show',(process.env.BASELINE_REF??'78b77986')+':integrations/bb-plugin-autarch/ui/map-placeholder.tsx']));
writeFileSync(out+'/map-entry.tsx','import{createRoot}from"react-dom/client";import{useState}from"react";import{MapPlaceholder}from"./map-before";function App(){const[l,set]=useState<any>("attention");return <MapPlaceholder lens={l} onLens={set}/>};createRoot(document.getElementById("root")!).render(<App/>);');
await build({entryPoints:[out+'/map-entry.tsx'],bundle:true,minify:true,outfile:out+'/map.js',nodePaths:[nm],jsx:'automatic'});
writeFileSync(out+'/map-before.html','<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="theme.css"><div id="root"></div><script src="map.js"></script>');
