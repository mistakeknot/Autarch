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

  const quote = (name: string) => '"' + name.replaceAll('"', '""') + '"';
  function uniqueKeys(db: Database.Database, table: string): string[][] {
    const columns = db.pragma(`table_info(${quote(table)})`) as {name: string; type: string; pk: number}[];
    const primary = columns.filter(c => c.pk > 0).sort((a,b) => a.pk-b.pk).map(c => c.name);
    const keys: string[][] = primary.length ? [primary] : [];
    const indexes = db.pragma(`index_list(${quote(table)})`) as {name: string; unique: number; partial: number; origin: string}[];
    for (const index of indexes.filter(i => i.unique)) {
      if (index.partial) throw new Error(`Unsupported partial unique index: ${index.name}`);
      const info = db.pragma(`index_info(${quote(index.name)})`) as {seqno: number; cid: number; name: string | null}[];
      if (!info.length || info.some(c => c.cid < 0 || c.name === null)) throw new Error(`Unsupported expression unique index: ${index.name}`);
      const key = info.sort((a,b) => a.seqno-b.seqno).map(c => c.name!);
      if (index.origin === 'pk') expect(key).toEqual(primary);
      else keys.push(key);
    }
    const metadata = (db.pragma('table_list') as {schema: string; name: string; wr: number}[])
      .find(t => t.schema === 'main' && t.name === table)!;
    // An INTEGER PRIMARY KEY aliases rowid unless SQLite built a separate PK index
    // (e.g. INTEGER PRIMARY KEY DESC). WITHOUT ROWID tables have no hidden key.
    const rowidAlias = primary.length === 1 && columns.find(c => c.name === primary[0])?.type.toUpperCase() === 'INTEGER'
      && !indexes.some(i => i.origin === 'pk');
    if (metadata.wr === 0 && !rowidAlias) keys.push(['rowid']);
    expect(keys.length).toBeGreaterThan(0);
    return keys;
  }

  // Discover from the final schema, including INTEGER PRIMARY KEYs that index_list omits.
  const inventoryDb = new Database(':memory:');
  migrate(inventoryDb);
  const inventory = Object.fromEntries(['pick_actions','pick_note_fences','pick_handler_control','obligations']
    .map(table => [table, uniqueKeys(inventoryDb, table)]));
  inventoryDb.close();

  for (const [table, keys] of Object.entries(inventory)) {
    const cases = table === 'obligations'
      ? ['pick-relay','pick-receipt'].flatMap(kind => [kind,'wake'].map(replacement => ({kind,replacement})))
      : [{kind:'',replacement:''}];
    for (const {kind,replacement} of cases) for (const key of keys) {
      it(`PRAGMA unique REPLACE ${table} (${key.join(',')}) ${kind} -> ${replacement}`, () => {
        const db = new Database(file); migrate(db);
        try {
          if (table === 'pick_actions') action(db, {disposition:'cancelled',ruling_op:'ruling',relay_op:'relay',
            fallback_op:'fallback',receipt_op:'receipt',status_op:'status',evaluation_json:'{"audit":true}'});
          if (table === 'pick_note_fences') db.exec(`INSERT INTO pick_note_fences
            (decision_id,task_id,generation,note_id,created_at,posting_state,last_error)
            VALUES ('d','task',1,'note','time','failed','evidence')`);
          if (table === 'pick_handler_control') db.exec("UPDATE pick_handler_control SET mode='shadow',epoch=5");
          if (table === 'obligations') db.prepare(`INSERT INTO obligations
            (id,decision_id,kind,state,op,next_try_at,updated_at,allow_reroute,allow_batch,require_message_id,handler_next_try_at,payload)
            VALUES ('protected','d',?,'done','operation','9999-12-31T23:59:59.999Z','audit',0,0,1,'2030-01-01T00:00:00Z','exact')`).run(kind);
          const before = tableRows(db, table);
          const row = {...before[0]};
          // Control's id aliases rowid; bind it only once.
          if (table === 'pick_handler_control') delete row.rowid;
          for (const column of new Set(keys.flat())) {
            const value = row[column];
            if (value === null || value === undefined) throw new Error(`Missing non-null unique fixture: ${table}.${column}`);
            if (!key.includes(column)) {
              if (typeof value !== 'string' && typeof value !== 'number') throw new Error(`Unsupported unique fixture: ${column}`);
              row[column] = typeof value === 'number' ? value + 1 : `replacement-${value}`;
            }
          }
          if (table === 'pick_actions') row.disposition = 'pending';
          if (table === 'pick_handler_control') row.epoch = 0;
          if (table === 'obligations') {
            row.kind = replacement; row.state = 'pending'; row.payload = 'changed';
            if (replacement === 'wake') row.next_try_at = '2000-01-01T00:00:00Z';
          }
          // Prove actual SQLite collision isolation, rather than trusting fixture names.
          for (const other of keys) {
            const collision = db.prepare(`SELECT count(*) n FROM ${quote(table)} WHERE ${other.map(c => `${quote(c)}=?`).join(' AND ')}`)
              .get(...other.map(c => row[c])) as {n: number};
            expect(collision.n, `${table} (${other.join(',')})`).toBe(other === key ? 1 : 0);
          }
          const columns = Object.keys(row);
          const message = table === 'pick_actions' ? 'pick action identity already exists'
            : table === 'pick_note_fences' ? 'pick note fence identity already exists'
            : table === 'pick_handler_control' ? 'pick handler control already exists'
            : 'protected obligation identity already exists';
          expect(() => db.prepare(`INSERT OR REPLACE INTO ${quote(table)} (${columns.map(quote).join(',')})
            VALUES (${columns.map(() => '?').join(',')})`).run(...columns.map(c => row[c]))).toThrow(message);
          expect(tableRows(db, table)).toEqual(before);
        } finally { db.close(); }
      });
    }
  }

  function tableRows(db: Database.Database, table: string) {
    return db.prepare(`SELECT rowid AS rowid, * FROM ${quote(table)} ORDER BY rowid`).all() as Record<string, unknown>[];
  }

  function insertRow(db: Database.Database, table: string, suffix: string, rowid?: number, kind = 'pick-relay') {
    if (table === 'pick_actions') {
      action(db, {pick_id:suffix,decision_id:suffix,...(rowid === undefined ? {} : {rowid})});
      return;
    }
    const row = table === 'pick_note_fences'
      ? {decision_id:suffix,task_id:'task',generation:1,note_id:suffix,created_at:'time'}
      : {id:suffix,decision_id:suffix,kind,next_try_at:'9999-12-31T23:59:59.999Z',
        updated_at:'time',allow_reroute:0,allow_batch:0,require_message_id:1};
    const values = {...row,...(rowid === undefined ? {} : {rowid})};
    const columns = Object.keys(values);
    db.prepare(`INSERT INTO ${quote(table)} (${columns.map(quote).join(',')})
      VALUES (${columns.map(c => '@'+c).join(',')})`).run(values);
  }

  for (const table of ['pick_actions','pick_note_fences','obligations']) {
    it(`ordinary inserts without explicit rowid succeed for ${table}`, () => {
      const db = new Database(file); migrate(db);
      try {
        insertRow(db,table,'first'); insertRow(db,table,'second');
        expect(tableRows(db,table).map(row => row.rowid)).toEqual([1,2]);
      } finally { db.close(); }
    });

  }

  const protectedRows = [
    {table:'pick_actions',kind:''}, {table:'pick_note_fences',kind:''},
    ...['pick-relay','pick-receipt'].map(kind => ({table:'obligations',kind})),
  ];
  for (const {table,kind} of protectedRows) for (const rowid of [-1,0]) {
    it(`protected nonpositive rowid INSERT aborts unchanged ${table} ${kind} (${rowid})`, () => {
      const db = new Database(file); migrate(db);
      try {
        insertRow(db,table,'existing',1,kind);
        const before = tableRows(db,table);
        expect(() => insertRow(db,table,'invalid',rowid,kind)).toThrow('positive rowid');
        expect(tableRows(db,table)).toEqual(before);
      } finally { db.close(); }
    });
  }

  for (const kind of ['pick-relay','pick-receipt']) {
    it(`omitted protected rowid after legacy -1 allocates positive rowid for ${kind}`, () => {
      const db = new Database(file); migrate(db);
      try {
        insertRow(db,'obligations','negative',-1,'wake');
        insertRow(db,'obligations','positive',1,'wake');
        const before = tableRows(db,'obligations');
        insertRow(db,'obligations','ordinary',undefined,kind);
        const after = tableRows(db,'obligations');
        expect(after.slice(0,2)).toEqual(before);
        expect(after).toHaveLength(3);
        expect(after[2]).toMatchObject({rowid:2,kind,id:'ordinary'});
      } finally { db.close(); }
    });

    it(`omitted protected rowid allocating zero aborts unchanged for ${kind}`, () => {
      const db = new Database(file); migrate(db);
      try {
        insertRow(db,'obligations','negative',-1,'wake');
        const before = tableRows(db,'obligations');
        expect(() => insertRow(db,'obligations','ordinary',undefined,kind)).toThrow('positive rowid');
        expect(tableRows(db,'obligations')).toEqual(before);
      } finally { db.close(); }
    });

    it(`protected no-collision rowid move is rejected for obligations ${kind}`, () => {
      const db = new Database(file); migrate(db);
      try {
        insertRow(db,'obligations','protected',1,kind);
        const before = tableRows(db,'obligations');
        for (const rowid of [-1,0,42]) {
          expect(() => db.prepare('UPDATE OR REPLACE obligations SET rowid=?').run(rowid)).toThrow('immutable');
          expect(tableRows(db,'obligations')).toEqual(before);
        }
      } finally { db.close(); }
    });

    for (const rowid of [-1,0]) {
      it(`legacy nonpositive rowid conversion aborts unchanged for ${kind} (${rowid})`, () => {
        const db = new Database(file); migrate(db);
        try {
          insertRow(db,'obligations','legacy',rowid,'wake');
          const before = tableRows(db,'obligations');
          expect(() => db.prepare('UPDATE obligations SET kind=?').run(kind)).toThrow('positive rowid');
          expect(tableRows(db,'obligations')).toEqual(before);
        } finally { db.close(); }
      });

      it(`AFTER INSERT ABORT restores legacy REPLACE victim for ${kind} (${rowid})`, () => {
        const db = new Database(file); migrate(db);
        try {
          insertRow(db,'obligations','victim',rowid,'wake');
          insertRow(db,'obligations','bystander',5,'wake');
          const before = tableRows(db,'obligations');
          expect(() => db.prepare(`INSERT OR REPLACE INTO obligations
            (rowid,id,decision_id,kind,next_try_at,updated_at,allow_reroute,allow_batch,require_message_id)
            VALUES (?,'replacement','d',?,'9999-12-31T23:59:59.999Z','time',0,0,1)`)
            .run(rowid,kind)).toThrow('positive rowid');
          expect(tableRows(db,'obligations')).toEqual(before);
        } finally { db.close(); }
      });
    }
  }

  for (const table of ['pick_actions','pick_note_fences']) {
    it(`protected no-collision rowid move is rejected for ${table}`, () => {
      const db = new Database(file); migrate(db);
      try {
        insertRow(db,table,'first');
        const before = tableRows(db,table);
        expect(() => db.exec(`UPDATE ${quote(table)} SET rowid=42`)).toThrow('immutable');
        expect(tableRows(db,table)).toEqual(before);
      } finally { db.close(); }
    });
  }

  it('legacy no-collision rowid moves remain allowed', () => {
    const db = new Database(file); migrate(db);
    try {
      db.exec("INSERT INTO obligations(id,decision_id,kind,next_try_at,updated_at) VALUES ('legacy','d','wake','time','time')");
      const before = tableRows(db,'obligations');
      db.exec('UPDATE obligations SET rowid=-1');
      expect(tableRows(db,'obligations')).toEqual([{...before[0],rowid:-1}]);
    } finally { db.close(); }
  });

  it('ordinary control initialization without explicit rowid preserves the id alias and singleton', () => {
    const db = new Database(file); migrate(db);
    try {
      expect(tableRows(db,'pick_handler_control')).toHaveLength(1);
      expect(tableRows(db,'pick_handler_control')[0]).toMatchObject({rowid:1,id:1});
      const before = tableRows(db,'pick_handler_control');
      expect(() => db.exec('UPDATE OR REPLACE pick_handler_control SET rowid=2')).toThrow('CHECK');
      expect(tableRows(db,'pick_handler_control')).toEqual(before);
    } finally { db.close(); }
  });

  function isolatedUpdate(db: Database.Database, table: string, keys: string[][], key: string[],
    source: Record<string, unknown>, victim: Record<string, unknown>) {
    const candidate = {...source, ...Object.fromEntries(key.map(c => [c, victim[c]]))};
    for (const other of keys) {
      const collision = db.prepare(`SELECT count(*) n FROM ${quote(table)} WHERE rowid != ? AND
        ${other.map(c => `${quote(c)}=?`).join(' AND ')}`)
        .get(source.rowid, ...other.map(c => candidate[c])) as {n: number};
      expect(collision.n, `${table} (${other.join(',')})`).toBe(other === key ? 1 : 0);
    }
    return () => db.prepare(`UPDATE OR REPLACE ${quote(table)} SET ${key.map(c => `${quote(c)}=?`).join(',')}
      WHERE rowid=?`).run(...key.map(c => victim[c]), source.rowid);
  }

  for (const [table, keys] of Object.entries(inventory)) {
    if (table === 'pick_handler_control') {
      it('PRAGMA unique UPDATE OR REPLACE control collision is unreachable with the singleton CHECK', () => {
        const db = new Database(file); migrate(db);
        try {
          expect(keys).toEqual([['id']]);
          db.exec("UPDATE pick_handler_control SET mode='shadow',epoch=5,reviewed_build='build',updated_at='audit'");
          const before = tableRows(db, table);
          // A second row cannot exist: keep every trigger and CHECK active.
          expect(() => db.exec('INSERT INTO pick_handler_control(id) VALUES (2)')).toThrow();
          expect(tableRows(db, table)).toEqual(before);
          expect(() => db.exec('UPDATE OR REPLACE pick_handler_control SET id=2 WHERE id=1')).toThrow('CHECK');
          expect(tableRows(db, table)).toEqual(before);
          expect(db.prepare('SELECT count(*) n FROM pick_handler_control').get()).toEqual({n:1});
          expect(db.prepare("UPDATE OR REPLACE pick_handler_control SET id=1,updated_at='new-audit' WHERE id=1").run().changes).toBe(1);
          expect(tableRows(db, table)).toEqual([{...before[0],updated_at:'new-audit'}]);
        } finally { db.close(); }
      });
      continue;
    }
    const cases = table === 'obligations'
      ? ['pick-relay','pick-receipt'].flatMap(kind => [kind,'wake'].map(sourceKind => ({kind,sourceKind})))
      : [{kind:'',sourceKind:''}];
    for (const {kind,sourceKind} of cases) for (const key of keys) {
      it(`PRAGMA unique UPDATE OR REPLACE ${table} (${key.join(',')}) ${kind} <- ${sourceKind}`, () => {
        const db = new Database(file); migrate(db);
        try {
          if (table === 'pick_actions') action(db, {disposition:'cancelled',ruling_op:'ruling',relay_op:'relay',
            fallback_op:'fallback',receipt_op:'receipt',status_op:'status',evaluation_json:'{"audit":true}',
            relay_text:'Exact payload',relay_sha256:'digest',updated_at:'victim-audit'});
          if (table === 'pick_note_fences') db.exec(`INSERT INTO pick_note_fences
            (decision_id,task_id,generation,note_id,created_at,posting_state,posting_attempted_at,posted_at,comment_id,last_error)
            VALUES ('d','task',1,'note','created','failed','attempt','posted','comment','evidence')`);
          if (table === 'obligations') db.prepare(`INSERT INTO obligations
            (id,decision_id,kind,state,op,next_try_at,updated_at,allow_reroute,allow_batch,require_message_id,handler_next_try_at,payload)
            VALUES ('protected','d',?,'done','operation','9999-12-31T23:59:59.999Z','audit',0,0,1,'2030-01-01T00:00:00Z','Exact payload')`).run(kind);
          const victim = tableRows(db, table)[0];
          const sourceValues = {...victim};
          for (const column of new Set(keys.flat())) {
            const value = sourceValues[column];
            if (typeof value !== 'string' && typeof value !== 'number') throw new Error(`Missing unique fixture: ${table}.${column}`);
            sourceValues[column] = typeof value === 'number' ? value + 1 : `source-${value}`;
          }
          if (table === 'pick_actions') {
            sourceValues.disposition = 'pending';
            // A non-NULL operation is already frozen. Exercise the allowed NULL -> value
            // assignment that previously deleted the other row through REPLACE.
            for (const column of key.filter(c => c.endsWith('_op'))) sourceValues[column] = null;
          }
          if (table === 'pick_note_fences') sourceValues.posting_state = 'recorded';
          if (table === 'obligations') {
            sourceValues.kind = sourceKind; sourceValues.state = 'pending'; sourceValues.payload = 'source payload';
            if (sourceKind === 'wake') Object.assign(sourceValues, {
              next_try_at:'2000-01-01T00:00:00Z',allow_reroute:1,allow_batch:1,require_message_id:0});
          }
          const columns = Object.keys(sourceValues);
          db.prepare(`INSERT INTO ${quote(table)} (${columns.map(quote).join(',')})
            VALUES (${columns.map(() => '?').join(',')})`).run(...columns.map(c => sourceValues[c]));
          const before = tableRows(db, table);
          expect(before).toHaveLength(2);
          const update = isolatedUpdate(db, table, keys, key, before[1], before[0]);
          // Identity keys may legitimately abort in the existing immutable trigger.
          expect(update).toThrow();
          expect(db.prepare(`SELECT count(*) n FROM ${quote(table)}`).get()).toEqual({n:2});
          expect(tableRows(db, table)).toEqual(before);
        } finally { db.close(); }
      });
    }
  }

  for (const key of inventory.obligations) {
    it(`PRAGMA unique UPDATE OR REPLACE legacy wake (${key.join(',')}) keeps SQLite replacement behavior`, () => {
      const db = new Database(file); migrate(db);
      try {
        db.exec(`INSERT INTO obligations(id,decision_id,kind,op,next_try_at,updated_at,payload) VALUES
          ('victim','d','wake','victim-op','time','time','victim payload'),
          ('source','d','wake','source-op','time','time','source payload')`);
        const before = tableRows(db, 'obligations');
        expect(isolatedUpdate(db, 'obligations', inventory.obligations, key, before[1], before[0])().changes).toBe(1);
        const expected = {...before[1], ...Object.fromEntries(key.map(c => [c,before[0][c]]))};
        expect(tableRows(db, 'obligations')).toEqual([expected]);
        expect(db.prepare('SELECT count(*) n FROM obligations').get()).toEqual({n:1});
        expect(db.prepare("UPDATE obligations SET payload='plain update'").run().changes).toBe(1);
        expect(tableRows(db, 'obligations')).toEqual([{...expected,payload:'plain update'}]);
        expect(db.prepare('SELECT allow_reroute,allow_batch,require_message_id,handler_next_try_at FROM obligations').get())
          .toEqual({allow_reroute:1,allow_batch:1,require_message_id:0,handler_next_try_at:null});
      } finally { db.close(); }
    });
  }

  it('PRAGMA unique UPDATE OR REPLACE permits literal self keys and audit/state changes', () => {
    const db = new Database(file); migrate(db);
    try {
      action(db, {ruling_op:'ruling',relay_op:'relay',fallback_op:'fallback',receipt_op:'receipt',status_op:'status'});
      db.exec(`INSERT INTO pick_note_fences(decision_id,task_id,generation,note_id,created_at)
        VALUES ('d','task',1,'note','created')`);
      for (const kind of ['pick-relay','pick-receipt']) db.prepare(`INSERT INTO obligations
        (id,decision_id,kind,op,next_try_at,updated_at,allow_reroute,allow_batch,require_message_id,payload)
        VALUES (?,'d',?,?,'9999-12-31T23:59:59.999Z','time',0,0,1,'Exact payload')`).run(kind,kind,kind+'-op');
      for (const [table,keys] of Object.entries(inventory)) {
        const audit = table === 'pick_actions' ? {evaluation_json:'{"self":true}',disposition:'automatic'}
          : table === 'pick_note_fences' ? {posting_state:'recorded',comment_id:'comment'}
          : table === 'obligations' ? {state:'done',updated_at:'audit'} : {updated_at:'audit'};
        const before = tableRows(db, table);
        for (const row of before) {
          const changes: Record<string, unknown> = {...Object.fromEntries([...new Set(keys.flat())].map(c => [c,row[c]])), ...audit};
          const columns = Object.keys(changes);
          expect(db.prepare(`UPDATE OR REPLACE ${quote(table)} SET ${columns.map(c => `${quote(c)}=?`).join(',')}
            WHERE rowid=?`).run(...columns.map(c => changes[c]),row.rowid).changes).toBe(1);
        }
        expect(tableRows(db, table)).toEqual(before.map(row => ({...row,...audit})));
      }
    } finally { db.close(); }
  });

  it('NULL unique operations do not collide in actions or protected obligations', () => {
    const db = new Database(file); migrate(db);
    action(db); action(db,{pick_id:'p2',decision_id:'d2'});
    expect(db.prepare('SELECT ruling_op,relay_op,fallback_op,receipt_op,status_op FROM pick_actions').all())
      .toEqual(Array(2).fill({ruling_op:null,relay_op:null,fallback_op:null,receipt_op:null,status_op:null}));
    for (const kind of ['pick-relay','pick-receipt']) for (const suffix of ['a','b']) {
      db.prepare(`INSERT INTO obligations(id,decision_id,kind,next_try_at,updated_at,allow_reroute,allow_batch,require_message_id)
        VALUES (?,'d',?,'9999-12-31T23:59:59.999Z','time',0,0,1)`).run(kind+suffix,kind);
    }
    expect(db.prepare('SELECT op FROM obligations').all()).toEqual(Array(4).fill({op:null}));
    const actionsBefore = tableRows(db, 'pick_actions');
    expect(db.prepare(`UPDATE OR REPLACE pick_actions SET pick_id=pick_id,decision_id=decision_id,
      ruling_op=NULL,relay_op=NULL,fallback_op=NULL,receipt_op=NULL,status_op=NULL,updated_at='null-audit'`).run().changes).toBe(2);
    expect(tableRows(db, 'pick_actions')).toEqual(actionsBefore.map(row => ({...row,updated_at:'null-audit'})));
    const obligationsBefore = tableRows(db, 'obligations');
    expect(db.prepare("UPDATE OR REPLACE obligations SET id=id,op=NULL,state='done'").run().changes).toBe(4);
    expect(tableRows(db, 'obligations')).toEqual(obligationsBefore.map(row => ({...row,state:'done'})));
    db.close();
  });

  for (const key of inventory.obligations) {
    it(`legacy unique collision (${key.join(',')}) retains IGNORE UPSERT REPLACE semantics`, () => {
      const db = new Database(file); migrate(db);
      db.exec("INSERT INTO obligations(id,decision_id,kind,op,next_try_at,updated_at,payload) VALUES ('old','d','wake','old-op','time','time','original')");
      const id = key.includes('id') ? 'old' : 'new';
      const op = key.includes('op') ? 'old-op' : 'new-op';
      const rowid = key.includes('rowid') ? 1 : 2;
      const values = [rowid,id,op];
      expect(db.prepare(`INSERT OR IGNORE INTO obligations(rowid,id,decision_id,kind,op,next_try_at,updated_at)
        VALUES (?,?,'d','wake',?,'time','time')`).run(...values).changes).toBe(0);
      db.prepare(`INSERT INTO obligations(rowid,id,decision_id,kind,op,next_try_at,updated_at) VALUES (?,?,'d','wake',?,'time','time')
        ON CONFLICT (${key.map(quote).join(',')}) DO UPDATE SET payload='upsert'`).run(...values);
      expect(db.prepare('SELECT payload FROM obligations').all()).toEqual([{payload:'upsert'}]);
      db.prepare(`INSERT OR REPLACE INTO obligations(rowid,id,decision_id,kind,op,next_try_at,updated_at,payload)
        VALUES (?,?,'d','wake',?,'time','time','replacement')`).run(...values);
      expect(db.prepare('SELECT id,op,payload,allow_reroute,allow_batch,require_message_id FROM obligations').all())
        .toEqual([{id,op,payload:'replacement',allow_reroute:1,allow_batch:1,require_message_id:0}]);
      db.close();
    });
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
    ['2030-01-01T', '2030-01-01T00:02:00Z'],
    ['2029-12-31T23:58:00Z', '2030-01-01T'],
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
