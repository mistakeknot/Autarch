package storage

import "database/sql"

type Task struct {
    ID     string
    Title  string
    Status string
}

func InsertTask(db *sql.DB, t Task) error {
    _, err := db.Exec(`INSERT INTO tasks (id, title, status) VALUES (?, ?, ?)`, t.ID, t.Title, t.Status)
    return err
}

func GetTask(db *sql.DB, id string) (Task, error) {
	row := db.QueryRow(`SELECT id, title, status FROM tasks WHERE id = ?`, id)
	var t Task
	if err := row.Scan(&t.ID, &t.Title, &t.Status); err != nil {
		return Task{}, err
	}
	return t, nil
}

func UpdateTaskStatus(db *sql.DB, id, status string) error {
	_, err := db.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, status, id)
	return err
}
