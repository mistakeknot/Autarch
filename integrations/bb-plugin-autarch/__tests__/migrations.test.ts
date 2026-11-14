import Database from "better-sqlite3";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { createHash } from "node:crypto";
import { existsSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { CODE_VERSION, MIGRATIONS, migrate, readSchemaState, SchemaTooNewError, type Migration } from "../migrations.js";
import { createStoreHandle, Store } from "../store.js";
import { QuiesceRequiredError } from "../backup.js";
import { decision, openStore, pickOf, tmpDir } from "./helpers.js";
import { loadV2 } from "./v2build.js";
import { loadV6 } from "./v6build.js";
import { BackupNotVerifiedError, contentDigest } from "../backup.js";
import { FakeSdk, econnrefused } from "./wakes-helpers.js";

let t: ReturnType<typeof tmpDir>;
let file: string;
beforeEach(() => {
  t = tmpDir();
  file = join(t.dir, "data.db");
});
afterEach(() => t.cleanup());

const V2: Migration = { version: 2, sql: "ALTER TABLE decisions ADD COLUMN triage TEXT; CREATE INDEX decisions_triage ON decisions(triage);" };

describe("v7 pick handler persistence", () => {
  function action(db: Database.Database, overrides: Record<string, unknown> = {}) {
    const row = { pick_id:"p", decision_id:"d", task_id:"task", generation:1, revision:"r", card_fp:"fp",
      asker:"thread-a", blocks_decision_id:"d", capture_mode:"shadow", capture_epoch:0, policy_version:1,
      captured_at:"2030-01-01T00:00:00.000Z", evaluation_deadline_at:"2030-01-01T00:02:00.000Z", ...overrides };
    const keys = Object.keys(row);
    db.prepare(`INSERT INTO pick_actions(${keys.join(',')}) VALUES (${keys.map(k=>'@'+k).join(',')})`).run(row);
  }

  function rejectsReplacement(db: Database.Database, table: string, changes: Record<string, unknown>) {
    const before = db.prepare(`SELECT * FROM ${table} ORDER BY rowid`).all() as Record<string, unknown>[];
    const row = {...before[0], ...changes};
    const keys = Object.keys(row);
    expect(()=>db.prepare(`INSERT OR REPLACE INTO ${table}(${keys.join(',')}) VALUES (${keys.map(k=>'@'+k).join(',')})`).run(row)).toThrow();
    expect(db.prepare(`SELECT * FROM ${table} ORDER BY rowid`).all()).toEqual(before);
  }

  for (const disposition of ['pending', 'fallback', 'cancelled']) {
    for (const key of ['pick_id', 'decision_id']) {
      it(`REPLACE preserves ${disposition} action by ${key}`, () => {
        const db = new Database(file); migrate(db);
        action(db,{disposition,relay_text:'Exact\ntext',relay_sha256:'digest',evaluation_json:'{ "audit": true }',
          ruling_op:'ruling',relay_op:'relay',fallback_op:'fallback',receipt_op:'receipt',status_op:'status',updated_at:'audit'});
        rejectsReplacement(db,'pick_actions',{[key === 'pick_id' ? 'decision_id' : 'pick_id']:'replacement',
          revision:'changed',disposition:'pending',evaluation_json:null,ruling_op:null,relay_op:null,
          fallback_op:null,receipt_op:null,status_op:null,relay_text:'changed',relay_sha256:'changed'});
        db.close();
      });
    }
  }

  for (const state of ['pending','failed','recorded']) {
    it(`REPLACE preserves ${state} note fence identity and evidence`, () => {
      const db = new Database(file); migrate(db);
      db.prepare(`INSERT INTO pick_note_fences(decision_id,task_id,generation,note_id,created_at,posting_state,
        posting_attempted_at,posted_at,comment_id,last_error) VALUES ('d','task',1,'note','created',?,'attempt','posted','comment','error')`).run(state);
      rejectsReplacement(db,'pick_note_fences',{task_id:'changed',generation:2,posting_state:'pending',
        posting_attempted_at:null,posted_at:null,comment_id:null,last_error:null});
      db.close();
    });
  }

  it("REPLACE preserves control epoch and owner scope evidence", () => {
    const db = new Database(file); migrate(db);
    db.exec(`UPDATE pick_handler_control SET mode='live',epoch=5,owner_enablement_json='{ "decision": "synthetic" }',
      reviewed_build='build',reviewed_scope_json='{ "scope": true }',enabled_cells_json='["instruction:none"]',updated_at='audit'`);
    rejectsReplacement(db,'pick_handler_control',{mode:'off',epoch:0,enabled_cells_json:'[]',reviewed_build:'changed',updated_at:null});
    db.close();
  });

  for (const kind of ['pick-relay','pick-receipt']) {
    for (const state of ['pending','done']) {
      for (const replacementKind of ['wake',kind]) {
        it(`REPLACE preserves ${state} ${kind} against ${replacementKind}`, () => {
          const db = new Database(file); migrate(db);
          db.prepare(`INSERT INTO obligations(id,decision_id,kind,state,next_try_at,updated_at,allow_reroute,allow_batch,require_message_id,
            handler_next_try_at,payload) VALUES ('protected','d',?,?,'9999-12-31T23:59:59.999Z','audit',0,0,1,'2030-01-01T00:00:00Z','Exact payload')`).run(kind,state);
          rejectsReplacement(db,'obligations',{kind:replacementKind,state:'pending',decision_id:'changed',payload:'changed',
            next_try_at:replacementKind === 'wake' ? '2000-01-01T00:00:00Z' : '9999-12-31T23:59:59.999Z',updated_at:'changed'});
          db.close();
        });
      }
    }
  }

  it("legacy obligation UPSERT preserves defaults and updates existing rows", () => {
    const db = new Database(file); migrate(db);
    const upsert = db.prepare(`INSERT INTO obligations(id,decision_id,kind,next_try_at,updated_at,payload)
      VALUES ('legacy','d','wake',?,'time',?) ON CONFLICT(id) DO UPDATE SET next_try_at=excluded.next_try_at,payload=excluded.payload`);
    upsert.run('first','original');
    upsert.run('second','updated');
    expect(db.prepare('SELECT kind,next_try_at,payload,allow_reroute,allow_batch,require_message_id,handler_next_try_at FROM obligations').all())
      .toEqual([{kind:'wake',next_try_at:'second',payload:'updated',allow_reroute:1,allow_batch:1,require_message_id:0,handler_next_try_at:null}]);
    db.close();
  });

  for (const kind of ['pick-relay', 'pick-receipt']) {
    const invalidPolicies = kind === 'pick-relay'
      ? [{}, {allow_reroute:1,allow_batch:0,require_message_id:1}, {allow_reroute:0,allow_batch:1,require_message_id:1}, {allow_reroute:0,allow_batch:0,require_message_id:0}]
      : [{}, {allow_batch:1}];
    for (const [index, policy] of invalidPolicies.entries()) {
      it(`protected policy rejects ${kind} invalid policy ${index} on insert and update`, () => {
        const db = new Database(file); migrate(db);
        const insert = (id: string, values: Record<string, unknown>) => {
          const row = {id, decision_id:'d', kind, next_try_at:'9999-12-31T23:59:59.999Z', updated_at:'time', ...values};
          const keys = Object.keys(row);
          return db.prepare(`INSERT INTO obligations(${keys.join(',')}) VALUES (${keys.map(k=>'@'+k).join(',')})`).run(row);
        };
        expect(()=>insert('invalid',policy)).toThrow();
        const valid = kind === 'pick-relay' ? {allow_reroute:0,allow_batch:0,require_message_id:1} : {allow_batch:0};
        insert('valid', valid);
        const changes = Object.keys(policy).length ? policy : {allow_reroute:1,allow_batch:1,require_message_id:0};
        expect(()=>db.prepare(`UPDATE obligations SET ${Object.keys(changes).map(k=>`${k}=@${k}`).join(',')} WHERE id='valid'`).run(changes)).toThrow();
        expect(db.prepare("SELECT kind FROM obligations WHERE id='valid'").get()).toEqual({kind});
        db.close();
      });
    }
  }

  it("action constraints freeze snapshots, preserve absent evidence and allow evaluation audit updates", () => {
    const db = new Database(file); migrate(db);
    action(db);
    expect(db.prepare("SELECT disposition,declaration_json,declaration_sha256,relay_text,relay_sha256,target_thread,target_title,target_project_id,target_project_name FROM pick_actions").get()).toEqual({disposition:"pending",declaration_json:null,declaration_sha256:null,relay_text:null,relay_sha256:null,target_thread:null,target_title:null,target_project_id:null,target_project_name:null});
    expect(()=>action(db,{decision_id:"d2"})).toThrow();
    expect(()=>action(db,{pick_id:"p2"})).toThrow();
    for (const [key,value] of Object.entries({pick_id:"other",decision_id:"other",task_id:"other",generation:2,revision:"other",card_fp:"other",asker:"other",blocks_decision_id:"other",capture_mode:"live",capture_epoch:1,policy_version:2,captured_at:"later",evaluation_deadline_at:"later",declaration_json:"{}",declaration_sha256:"digest",relay_text:"changed",relay_sha256:"digest",target_thread:"other",target_title:"other",target_project_id:"other",target_project_name:"other"})) {
      expect(()=>db.prepare(`UPDATE pick_actions SET ${key}=?`).run(value),key).toThrow();
    }
    db.exec("UPDATE pick_actions SET evaluation_json='{}', evaluation_started_at='start', evaluated_at='end', disposition='fallback', ruling_op='ruling', relay_op='relay', fallback_op='fallback', receipt_op='receipt', status_op='status', updated_at='end'");
    for (const op of ['ruling_op','relay_op','fallback_op','receipt_op','status_op']) {
      expect(()=>db.prepare(`UPDATE pick_actions SET ${op}='changed'`).run()).toThrow();
      expect(()=>action(db,{pick_id:'p2',decision_id:'d2',[op]:op.replace('_op','')})).toThrow();
    }
    expect(()=>db.exec('DELETE FROM pick_actions')).toThrow();
    action(db,{pick_id:'p2',decision_id:'d2',declaration_json:'{}',declaration_sha256:'synthetic-digest',relay_text:'Exact\ntext',relay_sha256:'synthetic-digest',target_thread:'target',target_title:'Title',target_project_id:'project',target_project_name:'Project'});
    expect(db.prepare("SELECT relay_text FROM pick_actions WHERE pick_id='p2'").get()).toEqual({relay_text:'Exact\ntext'});
    for (const disposition of ['automatic','cancelled','pending']) expect(()=>db.prepare("UPDATE pick_actions SET disposition=? WHERE pick_id='p'").run(disposition)).toThrow();
    expect(()=>db.exec("UPDATE pick_actions SET disposition='sent'")).toThrow();
    expect(()=>db.exec("UPDATE pick_actions SET evaluation_json='broken'")).toThrow();
    expect(()=>action(db,{pick_id:"p2",decision_id:"d2",evaluation_deadline_at:null})).toThrow();
    expect(()=>action(db,{pick_id:"p2",decision_id:"d2",capture_mode:"invalid"})).toThrow();
    db.close();
  });

  for (const [captured, deadline] of [
    ['5', '6'],
    ['5', '2030-01-01T00:00:00Z'],
    ['0001-01-01T00:00:00Z', '2000000'],
    ['garbage', 'zzzz'],
    ['2000-99-99T00:00:00Z', '2030-01-01T00:00:00Z'],
    ['2030-01-01T00:00:00Z', 'zzzz'],
    ['2030-01-01T00:00:00Z', '2030-99-99T00:00:00Z'],
    ['2030-01-01T00:00:00Z', '2030-01-01T01:00:00+02:00'],
    ['2030-01-01T00:00:00Z', '2030-01-01T01:00:00+01:00'],
  ]) {
    it(`action time rejects ${captured} to ${deadline}`, () => {
      const db = new Database(file); migrate(db);
      expect(()=>action(db,{captured_at:captured,evaluation_deadline_at:deadline})).toThrow();
      db.close();
    });
  }

  it("action time accepts chronological deadlines despite timezone and string ordering", () => {
    const db = new Database(file); migrate(db);
    const captured = '2030-01-01T01:00:00+01:00';
    const deadline = '2030-01-01T00:05:00Z';
    action(db,{captured_at:captured,evaluation_deadline_at:deadline});
    expect(db.prepare("SELECT captured_at,evaluation_deadline_at FROM pick_actions").get()).toEqual({captured_at:captured,evaluation_deadline_at:deadline});
    expect(()=>db.prepare("UPDATE pick_actions SET captured_at='2030-01-01T00:00:00Z' WHERE pick_id='p'").run()).toThrow();
    expect(()=>db.prepare("UPDATE pick_actions SET evaluation_deadline_at='2030-01-01T00:06:00Z' WHERE pick_id='p'").run()).toThrow();
    db.close();
  });

  it("note fences retain pending and failed evidence with immutable generation identity", () => {
    const db = new Database(file); migrate(db);
    db.exec("INSERT INTO pick_note_fences(decision_id,task_id,generation,note_id,created_at) VALUES ('d','task',1,'n','time')");
    expect(db.prepare("SELECT posting_state FROM pick_note_fences").get()).toEqual({posting_state:"pending"});
    db.exec("UPDATE pick_note_fences SET posting_state='failed',last_error='unavailable',posting_attempted_at='time'");
    expect(db.prepare("SELECT count(*) n FROM pick_note_fences WHERE task_id='task' AND generation=1").get()).toEqual({n:1});
    expect(()=>db.exec("DELETE FROM pick_note_fences")).toThrow();
    expect(()=>db.exec("UPDATE pick_note_fences SET note_id='other'")).toThrow();
    expect(()=>db.exec("INSERT INTO pick_note_fences(decision_id,task_id,generation,note_id,created_at) VALUES ('d','task',1,'n','time')")).toThrow();
    db.exec("UPDATE pick_note_fences SET posting_state='recorded',posted_at='time',comment_id='comment'");
    expect(()=>db.exec("UPDATE pick_note_fences SET posting_state='ignored'")).toThrow();
    db.close();
  });

  for (const terminal of ['fallback','cancelled']) {
    for (const next of ['pending','automatic','fallback','cancelled'].filter(value=>value !== terminal)) {
      it(`terminal disposition ${terminal} cannot become ${next}`, () => {
        const db = new Database(file); migrate(db);
        action(db);
        if (terminal === 'fallback') db.exec("UPDATE pick_actions SET disposition='automatic' WHERE pick_id='p'");
        db.prepare("UPDATE pick_actions SET disposition=? WHERE pick_id='p'").run(terminal);
        action(db,{pick_id:'independent',decision_id:'independent'});
        expect(()=>db.prepare("UPDATE pick_actions SET disposition=? WHERE pick_id='p'").run(next)).toThrow();
        db.prepare("UPDATE pick_actions SET disposition=?,evaluation_json='{}',updated_at='audit' WHERE pick_id='p'").run(terminal);
        expect(db.prepare("SELECT disposition,updated_at FROM pick_actions WHERE pick_id='p'").get()).toEqual({disposition:terminal,updated_at:'audit'});
        expect(db.prepare("SELECT disposition FROM pick_actions WHERE pick_id='independent'").get()).toEqual({disposition:'pending'});
        db.close();
      });
    }
  }

  for (const field of ['owner_enablement_json', 'reviewed_scope_json']) {
    for (const value of ['null', 'true', '123', '"text"', '[1,2]']) {
      it(`live control rejects ${field} ${value} on update and accepts objects`, () => {
        const db = new Database(file); migrate(db);
        const row = {owner_enablement_json:'{"decision":"synthetic"}',reviewed_scope_json:'{}',[field]:value};
        const update = db.prepare(`UPDATE pick_handler_control SET mode='live',epoch=1,owner_enablement_json=@owner_enablement_json,
          reviewed_build='build',reviewed_scope_json=@reviewed_scope_json,enabled_cells_json='["instruction:none"]'`);
        expect(()=>update.run(row)).toThrow(/CHECK constraint failed/);
        update.run({owner_enablement_json:'{}',reviewed_scope_json:'{}'});
        expect(db.prepare('SELECT mode,owner_enablement_json,reviewed_scope_json FROM pick_handler_control').get())
          .toEqual({mode:'live',owner_enablement_json:'{}',reviewed_scope_json:'{}'});
        db.close();
      });
    }
  }

  it("live control rejects duplicate cells on update and accepts distinct cells", () => {
    const db = new Database(file); migrate(db);
    const enable = db.prepare(`UPDATE pick_handler_control SET mode='live',epoch=1,owner_enablement_json='{"decision":"synthetic"}',
      reviewed_build='build',reviewed_scope_json='{}',enabled_cells_json=?`);
    expect(()=>enable.run('["instruction:none","instruction:none"]')).toThrow('unsupported pick handler cell');
    const update = db.prepare(`UPDATE pick_handler_control SET mode='live',epoch=2,owner_enablement_json='{"decision":"next"}',
      reviewed_build='build',reviewed_scope_json='{}',enabled_cells_json=?`);
    expect(()=>update.run('["ruling-only:none","ruling-only:none"]')).toThrow('unsupported pick handler cell');
    enable.run('["instruction:none","ruling-only:none"]');
    update.run('["instruction:github-pr","ruling-only:github-pr"]');
    expect(db.prepare("SELECT mode,epoch FROM pick_handler_control").get()).toEqual({mode:'live',epoch:2});
    db.close();
  });

  it("control defaults off and requires monotonic epochs and owner reviewed cell scope for live", () => {
    const db = new Database(file); migrate(db);
    expect(db.prepare("SELECT * FROM pick_handler_control").get()).toMatchObject({id:1,mode:"off",epoch:0,enabled_cells_json:"[]",owner_enablement_json:null,reviewed_build:null,reviewed_scope_json:null});
    expect(()=>db.exec("INSERT INTO pick_handler_control(id) VALUES (2)")).toThrow();
    expect(()=>db.exec("UPDATE pick_handler_control SET mode='live',epoch=1")).toThrow();
    expect(()=>db.exec("UPDATE pick_handler_control SET mode='shadow'")).toThrow();
    db.exec("UPDATE pick_handler_control SET mode='shadow',epoch=1");
    expect(()=>db.exec("UPDATE pick_handler_control SET epoch=0")).toThrow();
    const enable = db.prepare("UPDATE pick_handler_control SET mode='live',epoch=2,owner_enablement_json=?,reviewed_build='build',reviewed_scope_json='{}',enabled_cells_json=?");
    expect(()=>enable.run('{}','["merge:none"]')).toThrow();
    enable.run('{"decision":"synthetic"}','["instruction:none"]');
    expect(()=>db.exec("UPDATE pick_handler_control SET enabled_cells_json='[\"instruction:github-pr\"]'")).toThrow();
    db.exec("UPDATE pick_handler_control SET mode='off',epoch=3");
    expect(()=>db.exec("UPDATE pick_handler_control SET mode='live',epoch=4")).toThrow();
    expect(()=>db.exec("DELETE FROM pick_handler_control")).toThrow();
    db.close();
  });

  it("protected schedule constraints and handler lookup indexes preserve legacy defaults", () => {
    const db = new Database(file); migrate(db);
    db.exec("INSERT INTO obligations(id,decision_id,kind,next_try_at,updated_at) VALUES ('o','d','wake','due','time')");
    expect(db.prepare("SELECT allow_reroute,allow_batch,require_message_id,handler_next_try_at FROM obligations").get()).toEqual({allow_reroute:1,allow_batch:1,require_message_id:0,handler_next_try_at:null});
    for (const col of ['allow_reroute','allow_batch','require_message_id']) expect(()=>db.exec(`UPDATE obligations SET ${col}=2`)).toThrow();
    for (const kind of ['pick-relay','pick-receipt']) {
      expect(()=>db.prepare("UPDATE obligations SET kind=?").run(kind)).toThrow();
      db.prepare("INSERT INTO obligations(id,decision_id,kind,next_try_at,updated_at,allow_reroute,allow_batch,require_message_id) VALUES (?,'d',?,'9999-12-31T23:59:59.999Z','time',0,0,1)").run(kind,kind);
      expect(()=>db.prepare("UPDATE obligations SET next_try_at='due' WHERE id=?").run(kind)).toThrow();
      expect(()=>db.prepare("UPDATE obligations SET kind='wake' WHERE id=?").run(kind)).toThrow();
    }
    for (const [index,columns] of Object.entries({pick_actions_deadline:['disposition','evaluation_deadline_at'],pick_actions_generation:['task_id','generation'],pick_note_fences_generation:['task_id','generation'],obligations_handler_due:['state','handler_next_try_at']})) {
      expect((db.pragma(`index_info(${index})`) as {name:string}[]).map(r=>r.name)).toEqual(columns);
    }
    db.close();
  });
  async function populatedV6() {
    const v6 = await loadV6();
    expect(v6.CODE_VERSION).toBe(6);
    const s = new v6.Store(new Database(file));
    const d = decision({ root_fsid: "synthetic-filesystem" });
    s.insertDecision(d);
    s.recordPick(pickOf(d.id), [{ id: "legacy", kind: "wake", recipient: "thread-a", payload: "legacy", op: "legacy" }]);
    s.claim("legacy", 0);
    s.updateAttempt("legacy", 1, "not-delivered", { error: "synthetic retry", evidence: "preserved" });
    s.transition("legacy", "sending", "pending", 1, { next_try_at: "2000-01-01T00:00:00.000Z" });
    s.recordEvent("fixture", d.id, { evidence: "preserved" });
    s.close();
    return v6;
  }

  it("populated v6 upgrade preserves every old column, scheduling and verified preimage", async () => {
    await populatedV6();
    const db = new Database(file);
    const before = contentDigest(db);
    const indexes = db.prepare("SELECT name,sql FROM sqlite_master WHERE type='index' ORDER BY name").all();
    const names = (db.prepare("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").all() as {name: string}[]).map(r => r.name);
    const snapshots = names.map(name => {
      const columns = (db.pragma(`table_info(${name})`) as {name: string}[]).map(r => r.name);
      const sql = `SELECT ${columns.map(c => `"${c}"`).join(",")} FROM ${name} ORDER BY rowid`;
      return { name, sql, rows: db.prepare(sql).all() };
    });
    migrate(db);
    for (const index of indexes as {name:string;sql:string|null}[]) expect(db.prepare("SELECT name,sql FROM sqlite_master WHERE name=?").get(index.name)).toEqual(index);
    expect(()=>db.exec("INSERT INTO picks SELECT * FROM picks")).toThrow();
    for (const snap of snapshots) {
      if (snap.name === "schema_meta") continue;
      const rows = db.prepare(snap.sql).all();
      expect(snap.name === "migration_log" ? rows.slice(0, -1) : rows, snap.name).toEqual(snap.rows);
    }
    expect(db.prepare("SELECT allow_reroute,allow_batch,require_message_id,handler_next_try_at FROM obligations").get()).toEqual({allow_reroute:1,allow_batch:1,require_message_id:0,handler_next_try_at:null});
    const log = db.prepare("SELECT * FROM migration_log WHERE version=7").get() as {backup_path:string;digest:string;table_counts_json:string};
    expect(log.digest).toBe(before.digest);
    const backup = new Database(log.backup_path, {readonly:true});
    expect(backup.pragma("integrity_check", {simple:true})).toBe("ok");
    expect(readSchemaState(backup)).toEqual({schemaVersion:6,minReaderVersion:3});
    expect(contentDigest(backup)).toEqual(before);
    expect(JSON.parse(log.table_counts_json)).toEqual(before.tableCounts);
    backup.close();
    const after = contentDigest(db);
    migrate(db);
    expect(contentDigest(db)).toEqual(after);
    db.close();
  });

  for (const failure of ["quiesce", "backup", "transaction"] as const) {
    it(`v6 upgrade ${failure} failure preserves schema, data and log`, async () => {
      const v6 = await populatedV6();
      const old = failure === "quiesce" ? new v6.Store(new Database(file)) : null;
      const db = new Database(file, {timeout:50});
      const before = contentDigest(db);
      const schema = db.prepare("SELECT * FROM sqlite_master ORDER BY name").all();
      const run = () => migrate(db, {test:{hook:(step, ctx) => {
        if (failure === "backup" && step === "backup-written") {
          const b = new Database(ctx.backupPath!);
          b.exec("UPDATE settings_kv SET value='changed' WHERE key='store_id'");
          b.close();
        }
        if (failure === "transaction" && step === "ddl-applied") throw new Error("synthetic transaction failure");
      }}});
      if (failure === "quiesce") expect(run).toThrow(QuiesceRequiredError);
      else if (failure === "backup") expect(run).toThrow(BackupNotVerifiedError);
      else expect(run).toThrow("synthetic transaction failure");
      expect(contentDigest(db)).toEqual(before);
      expect(db.prepare("SELECT * FROM sqlite_master ORDER BY name").all()).toEqual(schema);
      db.close(); old?.close();
    });
  }

  it("v6 loop over expanded schema", async () => {
    const v6 = await populatedV6();
    const db = new Database(file); migrate(db); db.close();
    let now = "2030-01-01T00:00:00.000Z";
    const s = new v6.Store(new Database(file), {now:()=>now});
    const sdk = new FakeSdk();
    let routes = 0;
    const loop = new v6.WakeLoop({store:s,time:()=>now,imagesFor:()=>({state:"none"})} as unknown as ConstructorParameters<typeof v6.WakeLoop>[0], sdk, async()=>{routes++;return "successor";});
    const cardWrites = s.db.prepare("SELECT * FROM card_writes").all();
    action(s.db);
    s.db.exec("INSERT INTO pick_note_fences(decision_id,task_id,generation,note_id,created_at) VALUES ('d','task',1,'n','time')");
    const actionBefore = s.db.prepare("SELECT * FROM pick_actions").all();
    for (const kind of ["pick-relay","pick-receipt"]) {
      s.db.prepare(`INSERT INTO obligations(id,decision_id,kind,recipient,payload,op,next_try_at,handler_next_try_at,updated_at,allow_reroute,allow_batch,require_message_id)
        VALUES (?, 'synthetic', ?, 'thread-target','protected',?,'9999-12-31T23:59:59.999Z','2000-01-01T00:00:00.000Z',?,0,0,1)`).run(kind,kind,kind,now);
    }
    sdk.script = [{kind:"throw",err:econnrefused()}];
    await loop.drain();
    expect(s.obligation("legacy")?.state).toBe("pending");
    now = "2030-01-02T00:00:00.000Z";
    await loop.drain();
    expect(s.obligation("legacy")?.state).toBe("done");
    expect(s.attempts("legacy").map(r=>r.state)).toEqual(["not-delivered","not-delivered","delivered"]);
    for (const kind of ["pick-relay","pick-receipt"]) {
      s.db.prepare("UPDATE obligations SET state='uncertain' WHERE id=?").run(kind);
      expect(s.resend(kind,0,`resend-${kind}`)).toEqual({ok:true,replay:false});
      await loop.drain();
      expect(s.obligation(kind)).toMatchObject({state:"pending",next_try_at:"9999-12-31T23:59:59.999Z",attempt:0});
      s.db.prepare("UPDATE obligations SET state='undeliverable',last_error='thread archived' WHERE id=?").run(kind);
    }
    expect(routes).toBe(0);
    expect(sdk.sent).toHaveLength(2);
    expect(sdk.sent.every(r=>r.threadId === "thread-a")).toBe(true);
    // Exercise the real v6 stranded sweep alongside the archived protected rows.
    s.db.prepare(`INSERT INTO obligations(id,decision_id,kind,recipient,payload,op,state,last_error,next_try_at,updated_at)
      VALUES ('stranded-control','synthetic-control','wake','archived-thread','legacy control','stranded-control',
        'undeliverable','thread archived','2000-01-01T00:00:00.000Z',?)`).run(now);
    await loop.drain();
    expect(routes).toBe(1);
    expect(s.obligation('stranded-control')).toMatchObject({state:'done',recipient:'successor',attempt:1});
    expect(s.attempts('stranded-control').map(r=>r.state)).toEqual(['delivered']);
    expect(sdk.sent.map(r=>r.threadId)).toEqual(['thread-a','thread-a','successor']);
    expect(s.db.prepare("SELECT detail_json FROM events WHERE type='rerouted'").all()).toEqual([
      {detail_json:JSON.stringify({obligation:'stranded-control',from:'archived-thread',to:'successor'})},
    ]);
    expect(s.db.prepare("SELECT * FROM card_writes").all()).toEqual(cardWrites);
    expect(s.db.prepare("SELECT * FROM pick_actions").all()).toEqual(actionBefore);
    for (const kind of ["pick-relay","pick-receipt"]) {
      expect(s.attempts(kind)).toEqual([]);
      expect(s.obligation(kind)).toMatchObject({state:"undeliverable",recipient:"thread-target",next_try_at:"9999-12-31T23:59:59.999Z"});
    }
    s.close();
  });

  it("actual pre-PR1 reader accepts legacy bodies and rejects on_pick", async () => {
    const v6 = await loadV6();
    const body = {v:1,kind:"decide",subject:"synthetic",project:"example",project_root:"/synthetic",asker:"thread",thread:"thread-a",question:"Which?",recommendation:"a",options:[{id:"a",label:"Continue",kind:"instruction",instruction:"Continue inspection."},{id:"b",label:"Wait",kind:"needs-context"}]};
    expect(()=>v6.parseExistingAsk(body)).not.toThrow();
    const extended = structuredClone(body) as any;
    extended.options[0].on_pick = {schema:1,target_thread:"thread-a",reversible:true,subject:{kind:"none"},ruling:{bead:null,under:[]}};
    expect(()=>v6.parseExistingAsk(extended)).toThrow(/on_pick|unknown/i);
  });

  it("fresh schema is additive, inert and keeps reader version 3", () => {
    const db = new Database(file);
    expect(migrate(db)).toEqual({ schemaVersion: 7, minReaderVersion: 3 });
    for (const table of ["pick_actions", "pick_note_fences"]) {
      expect(db.prepare(`SELECT count(*) n FROM ${table}`).get()).toEqual({ n: 0 });
    }
    expect(db.prepare("SELECT mode, epoch FROM pick_handler_control").get()).toEqual({ mode: "off", epoch: 0 });
    expect(migrate(db)).toEqual({ schemaVersion: 7, minReaderVersion: 3 });
    expect(db.prepare("SELECT count(*) n FROM migration_log WHERE version=7").get()).toEqual({ n: 1 });
    db.close();
  });
});

describe("staged migrations", () => {
  it("a fresh database gets the current schema and a store_id", () => {
    const db = new Database(file);
    expect(migrate(db)).toEqual({ schemaVersion: CODE_VERSION, minReaderVersion: 3 });
    const id = (db.prepare("SELECT value FROM settings_kv WHERE key='store_id'").get() as { value: string }).value;
    expect(id).toMatch(/^[0-9a-f]{32}$/);
    // idempotent, and keeps the store_id
    migrate(db);
    expect((db.prepare("SELECT value FROM settings_kv WHERE key='store_id'").get() as { value: string }).value).toBe(id);
  });

  it("a database at an older schema version migrates and keeps its data", () => {
    const db = new Database(file);
    migrate(db, { codeVersion: 1, migrations: [MIGRATIONS[0]!] });
    db.prepare("INSERT INTO notes(id, at, text) VALUES ('n','t','keep me')").run();
    expect(readSchemaState(db).schemaVersion).toBe(1);
    expect(migrate(db, { codeVersion: 2, migrations: [MIGRATIONS[0]!, V2] })).toEqual({ schemaVersion: 2, minReaderVersion: 0 });
    expect(db.prepare("SELECT text FROM notes").get()).toEqual({ text: "keep me" });
    expect(db.prepare("SELECT triage FROM decisions").all()).toEqual([]);
  });

  it("the shipped v1 to v2 step adds the approvals tables and keeps data", () => {
    const db = new Database(file);
    migrate(db, { codeVersion: 1, migrations: [MIGRATIONS[0]!] });
    db.prepare("INSERT INTO notes(id, at, text) VALUES ('n','t','keep me')").run();
    expect(migrate(db, { codeVersion: 2 })).toEqual({ schemaVersion: 2, minReaderVersion: 0 });
    expect(db.prepare("SELECT text FROM notes").get()).toEqual({ text: "keep me" });
    expect(db.prepare("SELECT COUNT(*) n FROM approvals").get()).toEqual({ n: 0 });
    expect(db.prepare("SELECT COUNT(*) n FROM approval_events").get()).toEqual({ n: 0 });
  });

  it("refuses a database whose min_reader_version exceeds the code, and the handle stays not-ready", () => {
    const db = new Database(file);
    migrate(db);
    db.prepare("UPDATE schema_meta SET value = 99 WHERE key = 'min_reader_version'").run();
    expect(() => migrate(db)).toThrow(SchemaTooNewError);
    db.close();
    const handle = createStoreHandle(() => new Database(file), { closeOnFailure: true });
    expect(handle.ready()).toBe(false);
    expect(handle.error()).toMatch(/requires reader version 99/);
    handle.dispose();
  });

  it("a failed migration rolls back completely", () => {
    const db = new Database(file);
    migrate(db);
    const bad: Migration = { version: CODE_VERSION + 1, sql: "ALTER TABLE decisions ADD COLUMN ok TEXT; THIS IS NOT SQL;" };
    expect(() => migrate(db, { codeVersion: CODE_VERSION + 1, migrations: [...MIGRATIONS, bad] })).toThrow();
    expect(readSchemaState(db).schemaVersion).toBe(CODE_VERSION);
    expect(() => db.prepare("SELECT ok FROM decisions").all()).toThrow();
  });

  it("a candidate that cannot quiesce is refused, and version N keeps filing, picking and reconciling on its unchanged file", () => {
    const n = openStore(file);
    // candidate N+1 tries to migrate the same file while N is open: refused, nothing written
    const cand = new Database(file);
    expect(() => migrate(cand, { codeVersion: CODE_VERSION + 1, migrations: [...MIGRATIONS, { ...V2, version: CODE_VERSION + 1 }] })).toThrow(QuiesceRequiredError);
    cand.close();
    expect(readSchemaState(n.db).schemaVersion).toBe(CODE_VERSION);

    const d = decision();
    expect(n.insertDecision(d)).toMatchObject({ inserted: true });
    expect(n.recordPick(pickOf(d.id), [{ id: "o", kind: "notify" }])).toMatchObject({ ok: true });
    expect(n.claim("o", 0)).toEqual({ ok: true, attempt: 1 });
    n.updateAttempt("o", 1, "delivered", { evidence: "e" });
    expect(n.transition("o", "sending", "done", 1)).toEqual({ ok: true });

    // and a fresh version-N instance still starts on it
    const again = openStore(file);
    expect(again.obligation("o")?.state).toBe("done");
  });
});

// ---- v3 schema (Task 2.3 tests 1-5, 7) ----------------------------------------

function insDecision(db: Database.Database, id: string, extra: Record<string, unknown> = {}): void {
  const base: Record<string, unknown> = {
    id, request_id: `req-${id}`, identity: "i", revision: "rev-1", semantic_key: "s", subject: "subj", kind: "decide",
    project: "autarch", asker: "a", thread: "t", body_json: "{}", filed_at: "x", updated_at: "x", ...extra,
  };
  const cols = Object.keys(base);
  db.prepare(`INSERT INTO decisions(${cols.join(",")}) VALUES (${cols.map((c) => "@" + c).join(",")})`).run(base);
}
const CARD = { source: "card", task_id: "T1", generation: 1, tasks_project_id: "P1", card_fp: "fp1" };
const v3db = () => {
  const db = new Database(file);
  db.pragma("journal_mode = WAL");
  migrate(db);
  return db;
};
const abort = (fn: () => unknown) => expect(fn).toThrow(/constraint|abort|immutable|never carries|fixed at insert|append-only|frozen|insert-only|UNIQUE/i);

describe("v3 migration", () => {
  it("1. a populated v2 database migrates with every row and column preserved", async () => {
    const v2 = await loadV2();
    const old = openV2(v2);
    const d = decision();
    old.insertDecision(d);
    old.insertDecision(decision({ kind: "decide" }));
    old.recordPick(pickOf(d.id), [{ id: "o1", kind: "notify", recipient: "x" }, { id: "o2", kind: "ruling-file" }]);
    old.markSeen("acct", "item");
    old.setSetting("k", "v");
    old.close();
    const raw = new Database(file, { readonly: true });
    const tables = (raw.prepare("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name").all() as { name: string }[]).map((r) => r.name);
    const dump = (db: Database.Database, name: string, cols?: string[]) =>
      db.prepare(`SELECT ${cols ? cols.map((c) => `"${c}"`).join(",") : "*"} FROM "${name}" ORDER BY rowid`).all();
    const before = new Map(tables.map((n) => [n, dump(raw, n)]));
    const colsOf = new Map(tables.map((n) => [n, (raw.pragma(`table_info("${n}")`) as { name: string }[]).map((c) => c.name)]));
    expect(before.get("decisions")).toHaveLength(2);
    raw.close();

    const s = new Store(new Database(file), { busyTimeoutMs: 50 });
    for (const n of tables) {
      if (n === "schema_meta") continue;
      expect(dump(s.db, n, colsOf.get(n)), n).toEqual(before.get(n));
    }
    expect(s.db.prepare("SELECT source, task_id, generation, tasks_project_id, card_fp FROM decisions").all()).toEqual([
      { source: "home", task_id: null, generation: null, tasks_project_id: null, card_fp: null },
      { source: "home", task_id: null, generation: null, tasks_project_id: null, card_fp: null },
    ]);
    for (const t3 of ["cards", "card_requests", "decision_blocks", "card_writes", "project_bindings", "migration_log"]) {
      expect(s.db.prepare(`SELECT COUNT(*) c FROM ${t3}`).get(), t3).toBeDefined();
    }
    s.close();
  });

  it("2. (task_id, generation) is unique where task_id is set", () => {
    const db = v3db();
    insDecision(db, "g1", CARD);
    abort(() => insDecision(db, "g1dup", CARD));
    insDecision(db, "g2", { ...CARD, generation: 2 });
    insDecision(db, "h1");
    insDecision(db, "h2"); // NULL task_id rows do not collide
  });

  it("3. decisions_ask_immutable rejects body_json and card_fp updates, allows updated_at and resolved_at", () => {
    const db = v3db();
    insDecision(db, "g1", CARD);
    insDecision(db, "h1");
    abort(() => db.prepare("UPDATE decisions SET body_json = '{\"x\":1}' WHERE id = 'g1'").run());
    abort(() => db.prepare("UPDATE decisions SET card_fp = 'other' WHERE id = 'g1'").run());
    abort(() => db.prepare("UPDATE decisions SET card_fp = 'new' WHERE id = 'h1'").run()); // NULL -> value
    for (const col of ["revision", "identity", "subject", "semantic_key"]) {
      abort(() => db.prepare(`UPDATE decisions SET ${col} = 'changed' WHERE id = 'g1'`).run());
    }
    expect(db.prepare("UPDATE decisions SET updated_at = 'y', resolved_at = 'y', withdrawn_at = 'y' WHERE id = 'g1'").run().changes).toBe(1);
  });

  it("4. decisions_card_link and decisions_home_insert", () => {
    const db = v3db();
    insDecision(db, "card1", CARD);
    insDecision(db, "home1");
    // refused: source flips, both ways
    abort(() => db.prepare("UPDATE decisions SET source = 'home' WHERE id = 'card1'").run());
    abort(() => db.prepare("UPDATE decisions SET source = 'card' WHERE id = 'home1'").run());
    // refused: each link column, NULL -> value and value -> other
    for (const [col, val] of [["task_id", "T9"], ["generation", 9], ["tasks_project_id", "P9"]] as const) {
      abort(() => db.prepare(`UPDATE decisions SET ${col} = ? WHERE id = 'home1'`).run(val));
      abort(() => db.prepare(`UPDATE decisions SET ${col} = ? WHERE id = 'card1'`).run(val));
      abort(() => db.prepare(`UPDATE decisions SET ${col} = NULL WHERE id = 'card1'`).run());
    }
    // refused: a 'home' row inserted with any link column
    abort(() => insDecision(db, "bad1", { task_id: "T2" }));
    abort(() => insDecision(db, "bad2", { generation: 1 }));
    abort(() => insDecision(db, "bad3", { tasks_project_id: "P1" }));
    abort(() => insDecision(db, "bad4", { source: "other" }));
    // allowed: a card insert with all link columns, and updates of other columns
    insDecision(db, "card2", { ...CARD, task_id: "T2" });
    expect(db.prepare("UPDATE decisions SET updated_at = 'z', delegable = 0 WHERE id = 'card1'").run().changes).toBe(1);
    // a no-op write of a link column is not a change
    expect(db.prepare("UPDATE decisions SET source = 'card' WHERE id = 'card1'").run().changes).toBe(1);
  });

  it("5. card_requests, decision_blocks and cards_routing_frozen triggers", () => {
    const db = v3db();
    insDecision(db, "card1", CARD);
    db.prepare("INSERT INTO card_requests(request_key, task_id, identity, registered_at) VALUES ('k','T1','id1','t')").run();
    abort(() => db.prepare("UPDATE card_requests SET identity = 'x' WHERE request_key = 'k'").run());
    abort(() => db.prepare("DELETE FROM card_requests WHERE request_key = 'k'").run());
    db.prepare("INSERT INTO decision_blocks(decision_id, ref) VALUES ('card1','bead:mk-1')").run();
    abort(() => db.prepare("INSERT INTO decision_blocks(decision_id, ref) VALUES ('card1','bead:mk-1')").run()); // PK
    abort(() => db.prepare("UPDATE decision_blocks SET ref = 'bead:mk-2' WHERE decision_id = 'card1'").run());
    abort(() => db.prepare("DELETE FROM decision_blocks WHERE decision_id = 'card1'").run());

    db.prepare("INSERT INTO cards(task_id, state) VALUES ('T1','observed')").run();
    const up = (sql: string) => db.prepare(sql).run();
    up("UPDATE cards SET routing_mode = 'thread', routed_thread = 'thr_a' WHERE task_id = 'T1'"); // NULL -> value
    abort(() => up("UPDATE cards SET routing_mode = 'pull' WHERE task_id = 'T1'"));
    abort(() => up("UPDATE cards SET routed_thread = 'thr_b' WHERE task_id = 'T1'"));
    abort(() => up("UPDATE cards SET routing_mode = NULL WHERE task_id = 'T1'"));
    abort(() => up("UPDATE cards SET routed_thread = NULL WHERE task_id = 'T1'"));
    expect(up("UPDATE cards SET state = 'open', asking_thread = 'thr_c' WHERE task_id = 'T1'").changes).toBe(1);
    abort(() => up("UPDATE cards SET state = 'bogus' WHERE task_id = 'T1'"));
    abort(() => up("INSERT INTO cards(task_id, state, routing_mode) VALUES ('T2','open','sideways')"));
    // card_writes and project_bindings accept their documented values only
    db.prepare("INSERT INTO card_writes(id, task_id, decision_id, kind, payload, state, next_try_at, updated_at) VALUES ('w1','T1','card1','comment','{}','pending','t','t')").run();
    abort(() => db.prepare("INSERT INTO card_writes(id, task_id, decision_id, kind, payload, state, next_try_at, updated_at) VALUES ('w2','T1','card1','comment','{}','pending','t','t')").run()); // UNIQUE(decision_id, kind)
    abort(() => db.prepare("INSERT INTO card_writes(id, task_id, decision_id, kind, payload, state, next_try_at, updated_at) VALUES ('w3','T1','card1','nope','{}','pending','t','t')").run());
    db.prepare("INSERT INTO project_bindings(tasks_project_id, home_project, state, suggested_at) VALUES ('P1','autarch','suggested','t')").run();
    abort(() => db.prepare("INSERT INTO project_bindings(tasks_project_id, home_project, state) VALUES ('P2','autarch','trusted')").run());
  });

  it("7. reader fence: a v2 migrate on a v3 database throws SchemaTooNewError and writes nothing", async () => {
    const v2 = await loadV2();
    const s = new Store(new Database(file), { busyTimeoutMs: 50 });
    s.setSetting("k", "v");
    s.close(); // wal_checkpoint(TRUNCATE) on a clean stop
    const sha = () => createHash("sha256").update(readFileSync(file)).digest("hex");
    const meta = () => {
      const r = new Database(file, { readonly: true });
      try {
        return r.prepare("SELECT key, value FROM schema_meta ORDER BY key").all();
      } finally {
        r.close();
      }
    };
    const shaBefore = sha();
    const metaBefore = meta();
    expect(metaBefore).toContainEqual({ key: "min_reader_version", value: 3 });
    const db = new Database(file);
    expect(() => migrate(db, { codeVersion: 2, migrations: v2.MIGRATIONS })).toThrow(SchemaTooNewError);
    db.close();
    expect(sha()).toBe(shaBefore);
    expect(meta()).toEqual(metaBefore);
    const wal = `${file}-wal`;
    expect(!existsSync(wal) || statSync(wal).size === 0).toBe(true);
  });
});

function openV2(v2: Awaited<ReturnType<typeof loadV2>>) {
  return new v2.Store(new Database(file));
}

describe("v4 migration (moves)", () => {
  const v3Only = MIGRATIONS.filter((m) => m.version <= 3);

  it("migrates a real v3 database: backup verified, moves table added, no min_reader bump, data kept", () => {
    const old = new Database(file);
    migrate(old, { codeVersion: 3, migrations: v3Only });
    old.prepare("INSERT INTO cards(task_id, state) VALUES ('T1','open')").run();
    old.close();

    const logs: string[] = [];
    const db = new Database(file);
    expect(migrate(db, { log: { info: (m) => logs.push(m) } })).toEqual({ schemaVersion: 7, minReaderVersion: 3 });
    expect(logs.some((l) => l.startsWith("autarch: schema 3 → 7; backup ") && l.includes("verified"))).toBe(true);
    expect(db.prepare("SELECT COUNT(*) c FROM cards").get()).toEqual({ c: 1 });
    expect(db.prepare("SELECT COUNT(*) c FROM moves").get()).toEqual({ c: 0 });
    expect(db.prepare("SELECT version FROM migration_log ORDER BY version").all()).toEqual([{ version: 3 }, { version: 4 }, { version: 5 }, { version: 6 }, { version: 7 }]);
    db.close();
  });

  it("a v3 reader still opens an expanded database, and a database that demands a newer reader is refused", () => {
    const db = new Database(file);
    migrate(db);
    expect(() => migrate(db, { codeVersion: 3, migrations: v3Only })).not.toThrow();
    db.prepare("UPDATE schema_meta SET value = ? WHERE key = 'min_reader_version'").run(CODE_VERSION + 1);
    expect(() => migrate(db)).toThrow(SchemaTooNewError);
    db.close();
  });

  it("moves rejects an unknown kind or state, and a duplicate (task, generation)", () => {
    const db = new Database(file);
    migrate(db);
    const ins = (kind: string, state: string, gen = 1) =>
      db.prepare("INSERT INTO moves(task_id, generation, kind, payload_json, state, opened_by, opened_at) VALUES ('T1', ?, ?, '{}', ?, 'card', 't')").run(gen, kind, state);
    expect(() => ins("nope", "open")).toThrow();
    expect(() => ins("pr", "done")).toThrow();
    ins("pr", "open");
    expect(() => ins("pr", "open")).toThrow();
    ins("pr", "claimed", 2);
    db.close();
  });
});
