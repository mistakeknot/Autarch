package backlogrun

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "modernc.org/sqlite"
)

// Home reads Home's data.db read-only (mode=ro). It never writes.
type Home struct{ DB string }

var stateRank = map[string]int{"open": 4, "display": 3, "ruled": 2, "closed": 1}

// States returns the most live Home card state for each key that has a card.
func (h Home) States(ctx context.Context, keys []string) (map[string]string, error) {
	out := map[string]string{}
	if len(keys) == 0 {
		return out, nil
	}
	dsn := "file:" + h.DB + "?mode=ro&_pragma=busy_timeout(5000)"
	if _, err := url.Parse(dsn); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for i := 0; i < len(keys); i += 200 {
		end := i + 200
		if end > len(keys) {
			end = len(keys)
		}
		chunk := keys[i:end]
		q := "SELECT card_key, state FROM cards WHERE deleted_at IS NULL AND card_key IN (?" + strings.Repeat(",?", len(chunk)-1) + ")"
		args := make([]any, len(chunk))
		for j, k := range chunk {
			args[j] = k
		}
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("home cards: %w", err)
		}
		for rows.Next() {
			var k, st string
			if err := rows.Scan(&k, &st); err != nil {
				rows.Close()
				return nil, err
			}
			if stateRank[st] > stateRank[out[k]] {
				out[k] = st
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}
