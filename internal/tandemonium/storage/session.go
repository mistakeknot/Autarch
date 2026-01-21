package storage

import "database/sql"

type Session struct {
	ID     string
	TaskID string
	State  string
	Offset int64
}

func InsertSession(db *sql.DB, s Session) error {
	_, err := db.Exec(`INSERT INTO sessions (id, task_id, state, offset) VALUES (?, ?, ?, ?)`, s.ID, s.TaskID, s.State, s.Offset)
	return err
}

func UpdateSessionOffset(db *sql.DB, id string, offset int64) error {
	_, err := db.Exec(`UPDATE sessions SET offset = ? WHERE id = ?`, offset, id)
	return err
}

func UpdateSessionState(db *sql.DB, id, state string) error {
	_, err := db.Exec(`UPDATE sessions SET state = ? WHERE id = ?`, state, id)
	return err
}

func GetSession(db *sql.DB, id string) (Session, error) {
	row := db.QueryRow(`SELECT id, task_id, state, offset FROM sessions WHERE id = ?`, id)
	var s Session
	if err := row.Scan(&s.ID, &s.TaskID, &s.State, &s.Offset); err != nil {
		return Session{}, err
	}
	return s, nil
}

func FindSessionByTask(db *sql.DB, taskID string) (Session, error) {
	row := db.QueryRow(`SELECT id, task_id, state, offset FROM sessions WHERE task_id = ? ORDER BY rowid DESC LIMIT 1`, taskID)
	var s Session
	if err := row.Scan(&s.ID, &s.TaskID, &s.State, &s.Offset); err != nil {
		return Session{}, err
	}
	return s, nil
}
