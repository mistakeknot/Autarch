package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func OpenTemp() (*sql.DB, error) {
	dir, err := os.MkdirTemp("", "tandemonium-db-")
	if err != nil {
		return nil, err
	}
	return Open(filepath.Join(dir, "state.db"))
}

var sharedMu sync.Mutex
var sharedDBs = map[string]*sql.DB{}

func OpenShared(path string) (*sql.DB, error) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if db, ok := sharedDBs[path]; ok {
		return db, nil
	}
	db, err := Open(path)
	if err != nil {
		return nil, err
	}
	sharedDBs[path] = db
	return db, nil
}

func Migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS review_queue (
  task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  state TEXT NOT NULL,
  offset INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL,
  sender TEXT NOT NULL,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  created_ts TEXT NOT NULL,
  importance TEXT NOT NULL,
  ack_required INTEGER NOT NULL DEFAULT 0,
  metadata TEXT
);
CREATE TABLE IF NOT EXISTS mailboxes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  message_id TEXT NOT NULL,
  recipient TEXT NOT NULL,
  created_ts TEXT NOT NULL,
  read_ts TEXT,
  ack_ts TEXT
);
CREATE TABLE IF NOT EXISTS reservations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  path TEXT NOT NULL,
  owner TEXT NOT NULL,
  exclusive INTEGER NOT NULL DEFAULT 0,
  reason TEXT,
  created_ts TEXT NOT NULL,
  expires_ts TEXT NOT NULL,
  released_ts TEXT
);
CREATE TABLE IF NOT EXISTS agents (
  name TEXT PRIMARY KEY,
  program TEXT,
  model TEXT,
  task_description TEXT,
  created_ts TEXT NOT NULL,
  updated_ts TEXT NOT NULL,
  last_active_ts TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  payload TEXT NOT NULL,
  created_ts TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS contact_policies (
  owner TEXT PRIMARY KEY,
  policy TEXT NOT NULL,
  updated_ts TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS contact_requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  requester TEXT NOT NULL,
  recipient TEXT NOT NULL,
  status TEXT NOT NULL,
  created_ts TEXT NOT NULL,
  responded_ts TEXT
);
CREATE TABLE IF NOT EXISTS attachments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  message_id TEXT NOT NULL,
  path TEXT NOT NULL,
  note TEXT,
  created_ts TEXT NOT NULL,
  blob_hash TEXT,
  byte_size INTEGER,
  mime_type TEXT
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_sessions_task_id ON sessions(task_id);
CREATE INDEX IF NOT EXISTS idx_messages_thread_id ON messages(thread_id);
CREATE INDEX IF NOT EXISTS idx_mailboxes_recipient ON mailboxes(recipient);
CREATE INDEX IF NOT EXISTS idx_mailboxes_message_id ON mailboxes(message_id);
CREATE INDEX IF NOT EXISTS idx_reservations_path ON reservations(path);
CREATE INDEX IF NOT EXISTS idx_reservations_owner ON reservations(owner);
CREATE INDEX IF NOT EXISTS idx_events_kind ON events(kind);
CREATE UNIQUE INDEX IF NOT EXISTS idx_contact_requests_pair ON contact_requests(requester, recipient);
CREATE INDEX IF NOT EXISTS idx_contact_requests_recipient ON contact_requests(recipient);
CREATE INDEX IF NOT EXISTS idx_attachments_message_id ON attachments(message_id);
`)
	if err != nil {
		return err
	}
	return addAttachmentColumns(db)
}

func addAttachmentColumns(db *sql.DB) error {
	columns := []string{
		"ALTER TABLE attachments ADD COLUMN blob_hash TEXT",
		"ALTER TABLE attachments ADD COLUMN byte_size INTEGER",
		"ALTER TABLE attachments ADD COLUMN mime_type TEXT",
	}
	for _, stmt := range columns {
		if _, err := db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return err
		}
	}
	return nil
}
