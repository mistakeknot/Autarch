import Database from "better-sqlite3";
import { createHash } from "node:crypto";
import { copyFileSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vitest";
import { MIGRATIONS, migrate, readSchemaState } from "../migrations.js";
import { contentDigest } from "../backup.js";
import { tmpDir, decision, pickOf } from "./helpers.js";
import { Store } from "../store.js";
import { loadV6 } from "./v6build.js";

// All pre-existing content is invariant except the schema-version marker and
// newly appended migration-log rows, which the migration assertions check.
function oldTableDigests(db:Database.Database,names:string[]):Record<string,string> {
  const quote=(s:string)=>'"'+s.replaceAll('"','""')+'"';
  return Object.fromEntries(names.map(name=>{
    const schema=db.prepare("SELECT sql FROM sqlite_master WHERE type='table' AND name=?").get(name) as {sql:string};
    const columns=db.pragma(`table_info(${quote(name)})`) as {name:string;pk:number}[];
    const order=/\bWITHOUT\s+ROWID\b/i.test(schema.sql) ? columns.filter(c=>c.pk).sort((a,b)=>a.pk-b.pk).map(c=>quote(c.name)).join(',') : 'rowid';
    const where=name==='schema_meta' ? " WHERE key<>'schema_version'" : name==='migration_log' ? ' WHERE version<=7' : '';
    const rows=db.prepare(`SELECT ${columns.map(c=>quote(c.name)).join(',')} FROM ${quote(name)}${where} ORDER BY ${order}`).raw().all();
    return [name,createHash('sha256').update(JSON.stringify({schema:schema.sql,columns,rows})).digest('hex')];
  }));
}

it("keeps the shipped V1 migration identical to the pinned reader",async()=>{
  const old=await loadV6();
  expect(MIGRATIONS.find(m=>m.version===1)!.sql).toBe(old.MIGRATIONS.find(m=>m.version===1)!.sql);
});

it("uses byte-identical migration-8 DDL in the Go export fixture",()=>{
  const source=readFileSync(new URL("../../../internal/homefooter/export_cujgel_test.go",import.meta.url),"utf8");
  const match=/const cujgelV8DDL = `([\s\S]*?)`/.exec(source);
  expect(match,"Go export must use the verbatim migration-8 fixture").not.toBeNull();
  expect(match![1]).toBe(MIGRATIONS.find(m=>m.version===8)!.sql);
});

it("migrates a copied v7 database with a verified backup, no backfill or reader bump",()=>{
  const t=tmpDir();
  try {
    const oldFile=join(t.dir,"previous.db"),file=join(t.dir,"copy.db");
    const old=new Database(oldFile);migrate(old,{codeVersion:7});
    old.prepare("INSERT INTO cards(task_id,state) VALUES ('fixture','open')").run();old.close();
    const before=readFileSync(oldFile);copyFileSync(oldFile,file);
    const db=new Database(file);
    try {
      for(const name of ['decisions','picks','events','obligations'])
        expect((db.prepare(`SELECT count(*) n FROM ${name}`).get() as {n:number}).n,`v7 fixture must populate ${name}`).toBeGreaterThan(0);
      const names=(db.prepare("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name").all() as {name:string}[]).map(r=>r.name);
      const digests=oldTableDigests(db,names),preimage=contentDigest(db);
      expect(migrate(db)).toEqual({schemaVersion:8,minReaderVersion:3});
      expect(oldTableDigests(db,names)).toEqual(digests);
      expect(db.prepare("SELECT task_id,state FROM cards").all()).toEqual([{task_id:"fixture",state:"open"}]);
      expect(db.prepare("SELECT * FROM decision_cujgel").all()).toEqual([]);
      const log=db.prepare("SELECT backup_path,digest FROM migration_log WHERE version=8").get() as {backup_path:string;digest:string};
      expect(log.digest).toMatch(/^[0-9a-f]{64}$/);
      const backup=new Database(log.backup_path,{readonly:true});
      expect(readSchemaState(backup)).toEqual({schemaVersion:7,minReaderVersion:3});
      expect(backup.pragma("integrity_check",{simple:true})).toBe("ok");
      expect(contentDigest(backup)).toEqual(preimage);backup.close();
      expect(log.digest).toBe(preimage.digest);
      expect(()=>migrate(db,{codeVersion:7})).not.toThrow();
      expect(db.prepare("SELECT count(*) n FROM migration_log WHERE version=8").get()).toEqual({n:1});
      expect(readFileSync(oldFile)).toEqual(before);
    } finally { db.close(); }
  } finally { t.cleanup(); }
});

it("keeps generation snapshots immutable, including REPLACE",()=>{
  const t=tmpDir();const store=new Store(new Database(join(t.dir,"data.db")));
  try {
    store.insertDecision(decision({id:"fixture",request_id:"fixture"}));
    expect(store.db.prepare("SELECT name FROM sqlite_master WHERE name='decision_cujgel'").get()).toBeTruthy();
    store.db.prepare("INSERT INTO decision_cujgel(decision_id,body_json) VALUES ('fixture','{}')").run();
    for(const sql of ["UPDATE decision_cujgel SET body_json='[]'", "DELETE FROM decision_cujgel", "INSERT OR REPLACE INTO decision_cujgel VALUES ('fixture','[]')"])
      expect(()=>store.db.exec(sql)).toThrow();
    expect(()=>store.db.exec("INSERT OR REPLACE INTO decision_cujgel(rowid,decision_id,body_json) VALUES (1,'other','[]')")).toThrow();
    expect(store.db.prepare("SELECT * FROM decision_cujgel").all()).toEqual([{decision_id:"fixture",body_json:"{}"}]);
  } finally {store.close();t.cleanup();}
});

it.each(["bringback","override"] as const)("copies the original snapshot on %s",mode=>{
  const t=tmpDir();const store=new Store(new Database(join(t.dir,"data.db")));
  try {
    const first=decision({id:"first",request_id:"first",source:"card",task_id:"fixture",generation:1,tasks_project_id:"fixture",card_fp:"fingerprint"});
    store.insertDecision(first);
    store.db.prepare("INSERT INTO decision_cujgel VALUES ('first','{}')").run();
    expect(store.recordPick(pickOf("first",{revision:first.revision,by:mode==="override" ? "vizier" : "mk"}))).toMatchObject({ok:true});
    expect(store.insertReplacement({...first,id:"next",request_id:"next",generation:2,supersedes:"first"},mode)).toMatchObject({ok:true});
    expect(store.db.prepare("SELECT * FROM decision_cujgel ORDER BY decision_id").all()).toEqual([{decision_id:"first",body_json:"{}"},{decision_id:"next",body_json:"{}"}]);
  } finally { store.close();t.cleanup(); }
});
