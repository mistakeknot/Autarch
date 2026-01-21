package storage

import "database/sql"

func HasTasksTable(db *sql.DB) (bool, error) {
    row := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='tasks'`)
    var name string
    if err := row.Scan(&name); err != nil {
        if err == sql.ErrNoRows {
            return false, nil
        }
        return false, err
    }
    return name == "tasks", nil
}

func QuickCheck(db *sql.DB) (string, error) {
    row := db.QueryRow(`PRAGMA quick_check;`)
    var res string
    if err := row.Scan(&res); err != nil {
        return "", err
    }
    return res, nil
}

func CountTasksByStatus(db *sql.DB) (map[string]int, error) {
    rows, err := db.Query(`SELECT status, COUNT(*) FROM tasks GROUP BY status`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    counts := make(map[string]int)
    for rows.Next() {
        var status string
        var cnt int
        if err := rows.Scan(&status, &cnt); err != nil {
            return nil, err
        }
        counts[status] = cnt
    }
    return counts, rows.Err()
}
