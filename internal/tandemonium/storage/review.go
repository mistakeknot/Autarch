package storage

import "database/sql"

func AddToReviewQueue(db *sql.DB, taskID string) error {
	_, err := db.Exec(`INSERT INTO review_queue (task_id) VALUES (?)`, taskID)
	return err
}

func ListReviewQueue(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT task_id FROM review_queue ORDER BY rowid ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func RemoveFromReviewQueue(db *sql.DB, taskID string) error {
	_, err := db.Exec(`DELETE FROM review_queue WHERE task_id = ?`, taskID)
	return err
}

func ApproveTask(db *sql.DB, id string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, "done", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM review_queue WHERE task_id = ?`, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func RejectTask(db *sql.DB, id string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, "rejected", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, "ready", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM review_queue WHERE task_id = ?`, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
