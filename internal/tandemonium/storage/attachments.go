package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Attachment struct {
	MessageID string
	Path      string
	Note      string
	CreatedAt string
	BlobHash  string
	ByteSize  int64
	MimeType  string
}

func AddAttachmentsWithStore(db *sql.DB, storeDir, messageID string, attachments []Attachment) error {
	if len(attachments) == 0 {
		return nil
	}
	for i := range attachments {
		if strings.TrimSpace(attachments[i].Path) == "" {
			return errors.New("attachment path required")
		}
		hash, size, mime, err := StoreAttachment(storeDir, attachments[i].Path)
		if err != nil {
			return err
		}
		attachments[i].BlobHash = hash
		attachments[i].ByteSize = size
		attachments[i].MimeType = mime
	}
	return AddAttachments(db, messageID, attachments)
}

func AddAttachments(db *sql.DB, messageID string, attachments []Attachment) error {
	if len(attachments) == 0 {
		return nil
	}
	if strings.TrimSpace(messageID) == "" {
		return errors.New("message id required")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, att := range attachments {
		path := strings.TrimSpace(att.Path)
		if path == "" {
			_ = tx.Rollback()
			return errors.New("attachment path required")
		}
		msgID := att.MessageID
		if strings.TrimSpace(msgID) == "" {
			msgID = messageID
		}
		created := att.CreatedAt
		if strings.TrimSpace(created) == "" {
			created = nowTimestamp()
		}
		if _, err := tx.Exec(`INSERT INTO attachments (message_id, path, note, created_ts, blob_hash, byte_size, mime_type) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			msgID, path, att.Note, created, att.BlobHash, att.ByteSize, att.MimeType); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func ListAttachments(db *sql.DB, messageID string) ([]Attachment, error) {
	rows, err := db.Query(`SELECT message_id, path, note, created_ts, blob_hash, byte_size, mime_type FROM attachments WHERE message_id = ? ORDER BY id`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := []Attachment{}
	for rows.Next() {
		var att Attachment
		if err := rows.Scan(&att.MessageID, &att.Path, &att.Note, &att.CreatedAt, &att.BlobHash, &att.ByteSize, &att.MimeType); err != nil {
			return nil, err
		}
		attachments = append(attachments, att)
	}
	return attachments, rows.Err()
}

func ListAttachmentsForMessages(db *sql.DB, messageIDs []string) (map[string][]Attachment, error) {
	if len(messageIDs) == 0 {
		return map[string][]Attachment{}, nil
	}
	placeholders := make([]string, len(messageIDs))
	args := make([]interface{}, 0, len(messageIDs))
	for i, id := range messageIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`SELECT message_id, path, note, created_ts, blob_hash, byte_size, mime_type FROM attachments WHERE message_id IN (%s) ORDER BY id`, strings.Join(placeholders, ","))
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]Attachment{}
	for rows.Next() {
		var att Attachment
		if err := rows.Scan(&att.MessageID, &att.Path, &att.Note, &att.CreatedAt, &att.BlobHash, &att.ByteSize, &att.MimeType); err != nil {
			return nil, err
		}
		out[att.MessageID] = append(out[att.MessageID], att)
	}
	return out, rows.Err()
}

func ReadAttachmentData(storeDir, blobHash string, maxBytes int64) ([]byte, error) {
	blobHash = strings.TrimSpace(blobHash)
	if blobHash == "" {
		return nil, errors.New("blob hash required")
	}
	if len(blobHash) < 2 {
		return nil, errors.New("invalid blob hash")
	}
	path := filepath.Join(storeDir, blobHash[:2], blobHash)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && info.Size() > maxBytes {
		return nil, fmt.Errorf("attachment exceeds limit")
	}
	return os.ReadFile(path)
}
