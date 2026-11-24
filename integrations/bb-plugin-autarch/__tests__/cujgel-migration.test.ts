import Database from "better-sqlite3";
import { copyFileSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vitest";
import { migrate, readSchemaState } from "../migrations.js";
import { tmpDir, decision } from "./helpers.js";
import { Store } from "../store.js";

it("migrates a copied v7 database with a verified backup, no backfill or reader bump",()=>{
  const t=tmpDir();
  try {
    const oldFile=join(t.dir,"previous.db"),file=join(t.dir,"copy.db");
    const old=new Database(oldFile);migrate(old,{codeVersion:7});
    old.prepare("INSERT INTO cards(task_id,state) VALUES ('fixture','open')").run();old.close();
    const before=readFileSync(oldFile);copyFileSync(oldFile,file);
    const db=new Database(file);
    try {
      expect(migrate(db)).toEqual({schemaVersion:8,minReaderVersion:3});
      expect(db.prepare("SELECT task_id,state FROM cards").all()).toEqual([{task_id:"fixture",state:"open"}]);
      expect(db.prepare("SELECT * FROM decision_cujgel").all()).toEqual([]);
      const log=db.prepare("SELECT backup_path,digest FROM migration_log WHERE version=8").get() as {backup_path:string;digest:string};
      expect(log.digest).toMatch(/^[0-9a-f]{64}$/);
      const backup=new Database(log.backup_path,{readonly:true});
      expect(readSchemaState(backup)).toEqual({schemaVersion:7,minReaderVersion:3});backup.close();
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
    expect(store.db.prepare("SELECT * FROM decision_cujgel").all()).toEqual([{decision_id:"fixture",body_json:"{}"}]);
  } finally {store.close();t.cleanup();}
});
