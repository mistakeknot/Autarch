package storage

import (
	"database/sql"
	"errors"
	"strings"
)

var allowedContactPolicies = map[string]bool{
	"open":          true,
	"auto":          true,
	"contacts_only": true,
	"block_all":     true,
}

func GetContactPolicy(db *sql.DB, owner string) (string, error) {
	if strings.TrimSpace(owner) == "" {
		return "", errors.New("owner required")
	}
	row := db.QueryRow(`SELECT policy FROM contact_policies WHERE owner = ?`, owner)
	var policy string
	if err := row.Scan(&policy); err != nil {
		if err == sql.ErrNoRows {
			return "open", nil
		}
		return "", err
	}
	if policy == "" {
		return "open", nil
	}
	return policy, nil
}

func SetContactPolicy(db *sql.DB, owner, policy string) error {
	if strings.TrimSpace(owner) == "" {
		return errors.New("owner required")
	}
	policy = strings.TrimSpace(policy)
	if !allowedContactPolicies[policy] {
		return errors.New("invalid policy")
	}
	_, err := db.Exec(`INSERT INTO contact_policies (owner, policy, updated_ts) VALUES (?, ?, ?)
ON CONFLICT(owner) DO UPDATE SET policy = excluded.policy, updated_ts = excluded.updated_ts`, owner, policy, nowTimestamp())
	return err
}

func RequestContact(db *sql.DB, requester, recipient string) error {
	if strings.TrimSpace(requester) == "" || strings.TrimSpace(recipient) == "" {
		return errors.New("requester and recipient required")
	}
	_, err := db.Exec(`INSERT INTO contact_requests (requester, recipient, status, created_ts) VALUES (?, ?, ?, ?)
ON CONFLICT(requester, recipient) DO UPDATE SET status = excluded.status, created_ts = excluded.created_ts`, requester, recipient, "pending", nowTimestamp())
	return err
}

func RespondContact(db *sql.DB, requester, recipient string, accept bool) error {
	if strings.TrimSpace(requester) == "" || strings.TrimSpace(recipient) == "" {
		return errors.New("requester and recipient required")
	}
	status := "denied"
	if accept {
		status = "accepted"
	}
	res, err := db.Exec(`UPDATE contact_requests SET status = ?, responded_ts = ? WHERE requester = ? AND recipient = ?`, status, nowTimestamp(), requester, recipient)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("request not found")
	}
	return nil
}

func HasAcceptedContact(db *sql.DB, a, b string) (bool, error) {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false, errors.New("participants required")
	}
	row := db.QueryRow(`SELECT 1 FROM contact_requests WHERE ((requester = ? AND recipient = ?) OR (requester = ? AND recipient = ?)) AND status = 'accepted' LIMIT 1`, a, b, b, a)
	var val int
	if err := row.Scan(&val); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func ListContacts(db *sql.DB, owner string) ([]string, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, errors.New("owner required")
	}
	rows, err := db.Query(`SELECT requester, recipient FROM contact_requests WHERE status = 'accepted' AND (requester = ? OR recipient = ?)`, owner, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var requester string
		var recipient string
		if err := rows.Scan(&requester, &recipient); err != nil {
			return nil, err
		}
		if requester != owner {
			seen[requester] = true
		}
		if recipient != owner {
			seen[recipient] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	contacts := make([]string, 0, len(seen))
	for contact := range seen {
		contacts = append(contacts, contact)
	}
	return contacts, nil
}
