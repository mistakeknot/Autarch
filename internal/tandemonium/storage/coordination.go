package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Message struct {
	ID          string
	ThreadID    string
	Sender      string
	Subject     string
	Body        string
	CreatedAt   string
	Importance  string
	AckRequired bool
	Metadata    string
}

type MessageDelivery struct {
	Message   Message
	Recipient string
	ReadAt    string
	AckAt     string
}

type Reservation struct {
	ID         int64
	Path       string
	Owner      string
	Exclusive  bool
	Reason     string
	CreatedAt  string
	ExpiresAt  string
	ReleasedAt string
}

type ReservationConflict struct {
	Path   string
	Holder string
}

type ReservationResult struct {
	Granted   []Reservation
	Conflicts []ReservationConflict
}

type ThreadSummary struct {
	ThreadID     string
	Participants []string
	MessageCount int
}

func SendMessage(db *sql.DB, msg Message, recipients []string) error {
	if msg.ID == "" {
		return errors.New("message id required")
	}
	if len(recipients) == 0 {
		return errors.New("recipients required")
	}
	if msg.ThreadID == "" {
		msg.ThreadID = msg.ID
	}
	if msg.CreatedAt == "" {
		msg.CreatedAt = nowTimestamp()
	}
	if msg.Importance == "" {
		msg.Importance = "normal"
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec(`INSERT INTO messages (id, thread_id, sender, subject, body, created_ts, importance, ack_required, metadata)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ThreadID, msg.Sender, msg.Subject, msg.Body, msg.CreatedAt, msg.Importance, boolToInt(msg.AckRequired), msg.Metadata)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	for _, r := range recipients {
		if strings.TrimSpace(r) == "" {
			_ = tx.Rollback()
			return errors.New("recipient required")
		}
		_, err = tx.Exec(`INSERT INTO mailboxes (message_id, recipient, created_ts) VALUES (?, ?, ?)`,
			msg.ID, r, msg.CreatedAt)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func GetMessageByID(db *sql.DB, messageID string) (Message, error) {
	if strings.TrimSpace(messageID) == "" {
		return Message{}, errors.New("message id required")
	}
	row := db.QueryRow(`SELECT id, thread_id, sender, subject, body, created_ts, importance, ack_required, metadata
FROM messages
WHERE id = ?`, messageID)
	var msg Message
	var ackRequired int
	if err := row.Scan(&msg.ID, &msg.ThreadID, &msg.Sender, &msg.Subject, &msg.Body, &msg.CreatedAt, &msg.Importance, &ackRequired, &msg.Metadata); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Message{}, fmt.Errorf("message not found")
		}
		return Message{}, err
	}
	msg.AckRequired = ackRequired != 0
	return msg, nil
}

func FetchInbox(db *sql.DB, recipient string, limit int) ([]MessageDelivery, error) {
	return FetchInboxWithFilters(db, recipient, limit, "", false)
}

func FetchInboxWithFilters(db *sql.DB, recipient string, limit int, sinceTs string, urgentOnly bool) ([]MessageDelivery, error) {
	deliveries, _, err := FetchInboxPage(db, recipient, limit, sinceTs, urgentOnly, "")
	return deliveries, err
}

func FetchInboxPage(db *sql.DB, recipient string, limit int, sinceTs string, urgentOnly bool, pageToken string) ([]MessageDelivery, string, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
SELECT m.id, m.thread_id, m.sender, m.subject, m.body, m.created_ts, m.importance, m.ack_required, m.metadata,
       mb.recipient, mb.read_ts, mb.ack_ts
FROM mailboxes mb
JOIN messages m ON m.id = mb.message_id
WHERE mb.recipient = ?`
	args := []interface{}{recipient}
	if strings.TrimSpace(sinceTs) != "" {
		query += " AND m.created_ts >= ?"
		args = append(args, sinceTs)
	}
	if urgentOnly {
		query += " AND m.importance = ?"
		args = append(args, "urgent")
	}
	if strings.TrimSpace(pageToken) != "" {
		tokenTs, tokenID, err := decodePageToken(pageToken)
		if err != nil {
			return nil, "", err
		}
		query += " AND (m.created_ts < ? OR (m.created_ts = ? AND m.id < ?))"
		args = append(args, tokenTs, tokenTs, tokenID)
	}
	query += `
ORDER BY m.created_ts DESC, m.id DESC
LIMIT ?`
	args = append(args, limit+1)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	deliveries := []MessageDelivery{}
	for rows.Next() {
		var msg Message
		var recipient string
		var readAt sql.NullString
		var ackAt sql.NullString
		var ackRequired int
		if err := rows.Scan(&msg.ID, &msg.ThreadID, &msg.Sender, &msg.Subject, &msg.Body, &msg.CreatedAt, &msg.Importance, &ackRequired, &msg.Metadata, &recipient, &readAt, &ackAt); err != nil {
			return nil, "", err
		}
		msg.AckRequired = ackRequired != 0
		deliveries = append(deliveries, MessageDelivery{
			Message:   msg,
			Recipient: recipient,
			ReadAt:    readAt.String,
			AckAt:     ackAt.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	nextToken := ""
	if len(deliveries) > limit {
		last := deliveries[limit-1]
		nextToken = encodePageToken(last.Message.CreatedAt, last.Message.ID)
		deliveries = deliveries[:limit]
	}
	return deliveries, nextToken, nil
}

func AckMessage(db *sql.DB, messageID, recipient, ackTs string) error {
	if ackTs == "" {
		ackTs = nowTimestamp()
	}
	res, err := db.Exec(`UPDATE mailboxes SET ack_ts = ? WHERE message_id = ? AND recipient = ?`, ackTs, messageID, recipient)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("message not found")
	}
	return nil
}

func MarkMessageRead(db *sql.DB, messageID, recipient, readTs string) error {
	if readTs == "" {
		readTs = nowTimestamp()
	}
	res, err := db.Exec(`UPDATE mailboxes SET read_ts = ? WHERE message_id = ? AND recipient = ?`, readTs, messageID, recipient)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("message not found")
	}
	return nil
}

func ReservePaths(db *sql.DB, owner string, paths []string, exclusive bool, reason string, ttl time.Duration) (ReservationResult, error) {
	result := ReservationResult{}
	if ttl <= 0 {
		ttl = time.Hour
	}
	now := time.Now().UTC()
	nowTs := now.Format(time.RFC3339Nano)
	expiresTs := now.Add(ttl).Format(time.RFC3339Nano)

	tx, err := db.Begin()
	if err != nil {
		return result, err
	}

	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			_ = tx.Rollback()
			return result, errors.New("path required")
		}
		rows, err := tx.Query(`SELECT owner, exclusive FROM reservations WHERE path = ? AND released_ts IS NULL AND expires_ts > ?`, path, nowTs)
		if err != nil {
			_ = tx.Rollback()
			return result, err
		}
		conflict := false
		var holder string
		for rows.Next() {
			var existingOwner string
			var existingExclusive int
			if err := rows.Scan(&existingOwner, &existingExclusive); err != nil {
				rows.Close()
				_ = tx.Rollback()
				return result, err
			}
			if exclusive || existingExclusive != 0 {
				conflict = true
				holder = existingOwner
				break
			}
		}
		rows.Close()
		if conflict {
			result.Conflicts = append(result.Conflicts, ReservationConflict{Path: path, Holder: holder})
			continue
		}
		res, err := tx.Exec(`INSERT INTO reservations (path, owner, exclusive, reason, created_ts, expires_ts) VALUES (?, ?, ?, ?, ?, ?)`,
			path, owner, boolToInt(exclusive), reason, nowTs, expiresTs)
		if err != nil {
			_ = tx.Rollback()
			return result, err
		}
		id, _ := res.LastInsertId()
		result.Granted = append(result.Granted, Reservation{
			ID:        id,
			Path:      path,
			Owner:     owner,
			Exclusive: exclusive,
			Reason:    reason,
			CreatedAt: nowTs,
			ExpiresAt: expiresTs,
		})
	}

	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func ListActiveReservations(db *sql.DB, limit int) ([]Reservation, error) {
	if limit <= 0 {
		limit = 50
	}
	nowTs := nowTimestamp()
	rows, err := db.Query(`SELECT id, path, owner, exclusive, reason, created_ts, expires_ts, released_ts
FROM reservations
WHERE released_ts IS NULL AND expires_ts > ?
ORDER BY created_ts DESC
LIMIT ?`, nowTs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Reservation{}
	for rows.Next() {
		var res Reservation
		var exclusive int
		var released sql.NullString
		if err := rows.Scan(&res.ID, &res.Path, &res.Owner, &exclusive, &res.Reason, &res.CreatedAt, &res.ExpiresAt, &released); err != nil {
			return nil, err
		}
		res.Exclusive = exclusive != 0
		res.ReleasedAt = released.String
		out = append(out, res)
	}
	return out, rows.Err()
}

func ReleasePaths(db *sql.DB, owner string, paths []string) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(paths))
	args := []interface{}{nowTimestamp(), owner}
	for i, p := range paths {
		placeholders[i] = "?"
		args = append(args, p)
	}
	query := fmt.Sprintf(`UPDATE reservations SET released_ts = ? WHERE owner = ? AND released_ts IS NULL AND path IN (%s)`, strings.Join(placeholders, ","))
	res, err := db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(rows), nil
}

func ReleaseReservationsByID(db *sql.DB, owner string, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := []interface{}{nowTimestamp(), owner}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`UPDATE reservations SET released_ts = ? WHERE owner = ? AND released_ts IS NULL AND id IN (%s)`, strings.Join(placeholders, ","))
	res, err := db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(rows), nil
}

type ReservationRenewal struct {
	ID           int64
	Path         string
	OldExpiresAt string
	NewExpiresAt string
}

func RenewReservations(db *sql.DB, owner string, paths []string, extend time.Duration) ([]ReservationRenewal, error) {
	if extend <= 0 {
		extend = time.Hour
	}
	now := time.Now().UTC()
	nowTs := now.Format(time.RFC3339Nano)

	clauses := []string{"owner = ?", "released_ts IS NULL", "expires_ts > ?"}
	args := []interface{}{owner, nowTs}
	if len(paths) > 0 {
		placeholders := make([]string, len(paths))
		for i, p := range paths {
			placeholders[i] = "?"
			args = append(args, p)
		}
		clauses = append(clauses, fmt.Sprintf("path IN (%s)", strings.Join(placeholders, ",")))
	}

	query := fmt.Sprintf(`SELECT id, path, expires_ts FROM reservations WHERE %s`, strings.Join(clauses, " AND "))

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(query, args...)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	defer rows.Close()

	renewals := []ReservationRenewal{}
	for rows.Next() {
		var id int64
		var path string
		var expiresRaw string
		if err := rows.Scan(&id, &path, &expiresRaw); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		expiresAt, parseErr := time.Parse(time.RFC3339Nano, expiresRaw)
		if parseErr != nil {
			expiresAt = now
		}
		base := now
		if expiresAt.After(base) {
			base = expiresAt
		}
		newExpires := base.Add(extend)
		newExpiresTs := newExpires.Format(time.RFC3339Nano)
		if _, err := tx.Exec(`UPDATE reservations SET expires_ts = ? WHERE id = ?`, newExpiresTs, id); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		renewals = append(renewals, ReservationRenewal{
			ID:           id,
			Path:         path,
			OldExpiresAt: expiresRaw,
			NewExpiresAt: newExpiresTs,
		})
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return renewals, nil
}

func ForceReleaseReservation(db *sql.DB, id int64) error {
	if id <= 0 {
		return errors.New("reservation id required")
	}
	nowTs := nowTimestamp()
	res, err := db.Exec(`UPDATE reservations SET released_ts = ? WHERE id = ? AND released_ts IS NULL`, nowTs, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("reservation not found")
	}
	return nil
}

func SearchMessages(db *sql.DB, query string, limit int) ([]Message, error) {
	msgs, _, err := SearchMessagesPage(db, query, limit, "")
	return msgs, err
}

func SearchMessagesPage(db *sql.DB, query string, limit int, pageToken string) ([]Message, string, error) {
	if strings.TrimSpace(query) == "" {
		return []Message{}, "", nil
	}
	if limit <= 0 {
		limit = 50
	}
	term := "%" + query + "%"
	q := `SELECT id, thread_id, sender, subject, body, created_ts, importance, ack_required, metadata
FROM messages
WHERE (subject LIKE ? OR body LIKE ? OR sender LIKE ?)`
	args := []interface{}{term, term, term}
	if strings.TrimSpace(pageToken) != "" {
		ts, id, err := decodePageToken(pageToken)
		if err != nil {
			return nil, "", err
		}
		q += " AND (created_ts < ? OR (created_ts = ? AND id < ?))"
		args = append(args, ts, ts, id)
	}
	q += " ORDER BY created_ts DESC, id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	msgs := []Message{}
	for rows.Next() {
		var msg Message
		var ackRequired int
		if err := rows.Scan(&msg.ID, &msg.ThreadID, &msg.Sender, &msg.Subject, &msg.Body, &msg.CreatedAt, &msg.Importance, &ackRequired, &msg.Metadata); err != nil {
			return nil, "", err
		}
		msg.AckRequired = ackRequired != 0
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	nextToken := ""
	if len(msgs) > limit {
		last := msgs[limit-1]
		nextToken = encodePageToken(last.CreatedAt, last.ID)
		msgs = msgs[:limit]
	}
	return msgs, nextToken, nil
}

func SummarizeThread(db *sql.DB, threadID string) (ThreadSummary, error) {
	summary := ThreadSummary{ThreadID: threadID}
	row := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE thread_id = ?`, threadID)
	if err := row.Scan(&summary.MessageCount); err != nil {
		return summary, err
	}

	rows, err := db.Query(`
SELECT sender FROM messages WHERE thread_id = ?
UNION
SELECT mb.recipient FROM mailboxes mb
JOIN messages m ON m.id = mb.message_id
WHERE m.thread_id = ?`, threadID, threadID)
	if err != nil {
		return summary, err
	}
	defer rows.Close()

	participants := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return summary, err
		}
		participants = append(participants, p)
	}
	summary.Participants = participants
	return summary, rows.Err()
}

func ListThreadMessages(db *sql.DB, threadID string, limit int) ([]Message, error) {
	if strings.TrimSpace(threadID) == "" {
		return nil, errors.New("thread id required")
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.Query(`SELECT id, thread_id, sender, subject, body, created_ts, importance, ack_required, metadata
FROM messages
WHERE thread_id = ?
ORDER BY created_ts ASC, id ASC
LIMIT ?`, threadID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Message{}
	for rows.Next() {
		var msg Message
		var ackRequired int
		if err := rows.Scan(&msg.ID, &msg.ThreadID, &msg.Sender, &msg.Subject, &msg.Body, &msg.CreatedAt, &msg.Importance, &ackRequired, &msg.Metadata); err != nil {
			return nil, err
		}
		msg.AckRequired = ackRequired != 0
		out = append(out, msg)
	}
	return out, rows.Err()
}

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func encodePageToken(ts, id string) string {
	return fmt.Sprintf("%s|%s", ts, id)
}

func decodePageToken(token string) (string, string, error) {
	parts := strings.SplitN(token, "|", 2)
	if len(parts) != 2 {
		return "", "", errors.New("invalid page token")
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", errors.New("invalid page token")
	}
	return parts[0], parts[1], nil
}

func MergeRecipientMetadata(existing string, to, cc, bcc []string) (string, error) {
	payload := map[string]interface{}{}
	if strings.TrimSpace(existing) != "" {
		if err := json.Unmarshal([]byte(existing), &payload); err != nil {
			payload = map[string]interface{}{"_raw_metadata": existing}
		}
	}
	payload["to"] = to
	payload["cc"] = cc
	payload["bcc"] = bcc
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func ParseRecipientMetadata(metadata string) ([]string, []string, []string) {
	if strings.TrimSpace(metadata) == "" {
		return nil, nil, nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &payload); err != nil {
		return nil, nil, nil
	}
	return parseRecipientField(payload["to"]), parseRecipientField(payload["cc"]), parseRecipientField(payload["bcc"])
}

func parseRecipientField(raw interface{}) []string {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
