package coordination

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

const maxInlineAttachmentBytes int64 = 64 * 1024

type SendMessageRequest struct {
	MessageID     string
	ThreadID      string
	Sender        string
	Subject       string
	Body          string
	To            []string
	Cc            []string
	Bcc           []string
	Importance    string
	AckRequired   bool
	Metadata      map[string]string
	Attachments   []AttachmentRef
	AttachmentDir string
	CreatedAt     string
}

type SendMessageResponse struct {
	MessageID string
	ThreadID  string
}

type AttachmentRef struct {
	Path string
	Note string
}

type AttachmentPayload struct {
	MessageID string `json:"message_id"`
	Path      string `json:"path"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_ts"`
	BlobHash  string `json:"blob_hash"`
	ByteSize  int64  `json:"byte_size"`
	MimeType  string `json:"mime_type"`
	Data      string `json:"data,omitempty"`
}

type FetchInboxRequest struct {
	Recipient          string
	Limit              int
	SinceTS            string
	UrgentOnly         bool
	PageToken          string
	IncludeAttachments bool
	AttachmentDir      string
}

type InboxMessage struct {
	ID          string
	ThreadID    string
	Sender      string
	Subject     string
	Body        string
	CreatedAt   string
	Importance  string
	AckRequired bool
	ReadAt      string
	AckAt       string
	Recipient   string
	Attachments []AttachmentPayload
	To          []string
	Cc          []string
	Bcc         []string
}

type FetchInboxResponse struct {
	Messages  []InboxMessage
	NextToken string
}

type AckMessageRequest struct {
	MessageID string
	Recipient string
	AckTS     string
}

type AckMessageResponse struct {
	MessageID string
	Recipient string
	AckAt     string
}

type MarkReadRequest struct {
	MessageID string
	Recipient string
	ReadTS    string
}

type MarkReadResponse struct {
	MessageID string
	Recipient string
	ReadAt    string
}

type ReservePathsRequest struct {
	Owner     string
	Paths     []string
	Exclusive bool
	Reason    string
	TTL       time.Duration
}

type ReservePathsResponse struct {
	Granted   []storage.Reservation
	Conflicts []storage.ReservationConflict
}

type ReleasePathsRequest struct {
	Owner string
	Paths []string
}

type ReleasePathsResponse struct {
	Released int
}

type SearchMessagesRequest struct {
	Query     string
	Limit     int
	PageToken string
}

type MessageSummary struct {
	ID         string
	ThreadID   string
	Sender     string
	Subject    string
	CreatedAt  string
	Importance string
}

type SearchMessagesResponse struct {
	Messages  []MessageSummary
	NextToken string
}

type SummarizeThreadRequest struct {
	ThreadID        string
	IncludeExamples bool
	LLMMode         bool
	LLMConfig       config.LLMSummaryConfig
}

type SummarizeThreadResponse struct {
	ThreadID     string
	Participants []string
	MessageCount int
	KeyPoints    []string
	ActionItems  []string
	Examples     []LLMSummaryExample
}

func SendMessage(db *sql.DB, req SendMessageRequest) (SendMessageResponse, error) {
	recipients := append([]string{}, req.To...)
	recipients = append(recipients, req.Cc...)
	recipients = append(recipients, req.Bcc...)
	for _, r := range recipients {
		policy, err := storage.GetContactPolicy(db, r)
		if err != nil {
			return SendMessageResponse{}, err
		}
		switch policy {
		case "block_all":
			return SendMessageResponse{}, errors.New("recipient blocks all contacts")
		case "contacts_only":
			ok, err := storage.HasAcceptedContact(db, req.Sender, r)
			if err != nil {
				return SendMessageResponse{}, err
			}
			if !ok {
				return SendMessageResponse{}, errors.New("contact required")
			}
		}
	}
	metadataJSON := ""
	metadata := map[string]interface{}{}
	for k, v := range req.Metadata {
		metadata[k] = v
	}
	metadata["to"] = req.To
	metadata["cc"] = req.Cc
	metadata["bcc"] = req.Bcc
	if len(metadata) > 0 {
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return SendMessageResponse{}, err
		}
		metadataJSON = string(encoded)
	}
	msg := storage.Message{
		ID:          req.MessageID,
		ThreadID:    req.ThreadID,
		Sender:      req.Sender,
		Subject:     req.Subject,
		Body:        req.Body,
		CreatedAt:   req.CreatedAt,
		Importance:  req.Importance,
		AckRequired: req.AckRequired,
		Metadata:    metadataJSON,
	}
	if err := storage.SendMessage(db, msg, recipients); err != nil {
		return SendMessageResponse{}, err
	}
	if len(req.Attachments) > 0 {
		if strings.TrimSpace(req.AttachmentDir) == "" {
			return SendMessageResponse{}, errors.New("attachment dir required")
		}
		attachments := make([]storage.Attachment, 0, len(req.Attachments))
		for _, att := range req.Attachments {
			attachments = append(attachments, storage.Attachment{
				MessageID: msg.ID,
				Path:      att.Path,
				Note:      att.Note,
			})
		}
		if err := storage.AddAttachmentsWithStore(db, req.AttachmentDir, msg.ID, attachments); err != nil {
			return SendMessageResponse{}, err
		}
	}
	threadID := req.ThreadID
	if strings.TrimSpace(threadID) == "" {
		threadID = msg.ID
	}
	return SendMessageResponse{MessageID: msg.ID, ThreadID: threadID}, nil
}

func FetchInbox(db *sql.DB, req FetchInboxRequest) (FetchInboxResponse, error) {
	deliveries, nextToken, err := storage.FetchInboxPage(db, req.Recipient, req.Limit, req.SinceTS, req.UrgentOnly, req.PageToken)
	if err != nil {
		return FetchInboxResponse{}, err
	}
	if req.IncludeAttachments && strings.TrimSpace(req.AttachmentDir) == "" {
		return FetchInboxResponse{}, errors.New("attachment dir required")
	}
	messageIDs := make([]string, 0, len(deliveries))
	for _, d := range deliveries {
		messageIDs = append(messageIDs, d.Message.ID)
	}
	attachmentsByMessage, err := storage.ListAttachmentsForMessages(db, messageIDs)
	if err != nil {
		return FetchInboxResponse{}, err
	}
	messages := make([]InboxMessage, 0, len(deliveries))
	for _, d := range deliveries {
		to, cc, bcc := storage.ParseRecipientMetadata(d.Message.Metadata)
		payloads := make([]AttachmentPayload, 0, len(attachmentsByMessage[d.Message.ID]))
		for _, att := range attachmentsByMessage[d.Message.ID] {
			payload := AttachmentPayload{
				MessageID: att.MessageID,
				Path:      att.Path,
				Note:      att.Note,
				CreatedAt: att.CreatedAt,
				BlobHash:  att.BlobHash,
				ByteSize:  att.ByteSize,
				MimeType:  att.MimeType,
			}
			if req.IncludeAttachments {
				data, err := storage.ReadAttachmentData(req.AttachmentDir, att.BlobHash, maxInlineAttachmentBytes)
				if err != nil {
					return FetchInboxResponse{}, err
				}
				payload.Data = base64.StdEncoding.EncodeToString(data)
			}
			payloads = append(payloads, payload)
		}
		messages = append(messages, InboxMessage{
			ID:          d.Message.ID,
			ThreadID:    d.Message.ThreadID,
			Sender:      d.Message.Sender,
			Subject:     d.Message.Subject,
			Body:        d.Message.Body,
			CreatedAt:   d.Message.CreatedAt,
			Importance:  d.Message.Importance,
			AckRequired: d.Message.AckRequired,
			ReadAt:      d.ReadAt,
			AckAt:       d.AckAt,
			Recipient:   d.Recipient,
			Attachments: payloads,
			To:          to,
			Cc:          cc,
			Bcc:         bcc,
		})
	}
	return FetchInboxResponse{Messages: messages, NextToken: nextToken}, nil
}

func AckMessage(db *sql.DB, req AckMessageRequest) (AckMessageResponse, error) {
	if err := storage.AckMessage(db, req.MessageID, req.Recipient, req.AckTS); err != nil {
		return AckMessageResponse{}, err
	}
	ackAt := req.AckTS
	if strings.TrimSpace(ackAt) == "" {
		ackAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return AckMessageResponse{MessageID: req.MessageID, Recipient: req.Recipient, AckAt: ackAt}, nil
}

func MarkRead(db *sql.DB, req MarkReadRequest) (MarkReadResponse, error) {
	if err := storage.MarkMessageRead(db, req.MessageID, req.Recipient, req.ReadTS); err != nil {
		return MarkReadResponse{}, err
	}
	readAt := req.ReadTS
	if strings.TrimSpace(readAt) == "" {
		readAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return MarkReadResponse{MessageID: req.MessageID, Recipient: req.Recipient, ReadAt: readAt}, nil
}

func ReservePaths(db *sql.DB, req ReservePathsRequest) (ReservePathsResponse, error) {
	res, err := storage.ReservePaths(db, req.Owner, req.Paths, req.Exclusive, req.Reason, req.TTL)
	if err != nil {
		return ReservePathsResponse{}, err
	}
	return ReservePathsResponse{Granted: res.Granted, Conflicts: res.Conflicts}, nil
}

func ReleasePaths(db *sql.DB, req ReleasePathsRequest) (ReleasePathsResponse, error) {
	released, err := storage.ReleasePaths(db, req.Owner, req.Paths)
	if err != nil {
		return ReleasePathsResponse{}, err
	}
	return ReleasePathsResponse{Released: released}, nil
}

func SearchMessages(db *sql.DB, req SearchMessagesRequest) (SearchMessagesResponse, error) {
	msgs, nextToken, err := storage.SearchMessagesPage(db, req.Query, req.Limit, req.PageToken)
	if err != nil {
		return SearchMessagesResponse{}, err
	}
	resp := SearchMessagesResponse{Messages: make([]MessageSummary, 0, len(msgs)), NextToken: nextToken}
	for _, msg := range msgs {
		resp.Messages = append(resp.Messages, MessageSummary{
			ID:         msg.ID,
			ThreadID:   msg.ThreadID,
			Sender:     msg.Sender,
			Subject:    msg.Subject,
			CreatedAt:  msg.CreatedAt,
			Importance: msg.Importance,
		})
	}
	return resp, nil
}

func SummarizeThread(db *sql.DB, req SummarizeThreadRequest) (SummarizeThreadResponse, error) {
	summary, err := storage.SummarizeThread(db, req.ThreadID)
	if err != nil {
		return SummarizeThreadResponse{}, err
	}
	resp := SummarizeThreadResponse{
		ThreadID:     summary.ThreadID,
		Participants: summary.Participants,
		MessageCount: summary.MessageCount,
	}
	if !req.LLMMode {
		return resp, nil
	}
	messages, err := storage.ListThreadMessages(db, req.ThreadID, 200)
	if err != nil {
		return SummarizeThreadResponse{}, err
	}
	llmMessages := make([]LLMMessage, 0, len(messages))
	for _, msg := range messages {
		llmMessages = append(llmMessages, LLMMessage{
			ID:      msg.ID,
			Sender:  msg.Sender,
			Subject: msg.Subject,
			Body:    msg.Body,
		})
	}
	output, err := RunLLMSummaryCommand(context.Background(), req.LLMConfig, LLMSummaryInput{
		ThreadID:        req.ThreadID,
		Messages:        llmMessages,
		IncludeExamples: req.IncludeExamples,
	})
	if err != nil {
		return SummarizeThreadResponse{}, err
	}
	if len(output.Summary.Participants) > 0 {
		resp.Participants = output.Summary.Participants
	}
	resp.KeyPoints = output.Summary.KeyPoints
	resp.ActionItems = output.Summary.ActionItems
	if req.IncludeExamples {
		resp.Examples = output.Examples
	}
	return resp, nil
}

type SetContactPolicyRequest struct {
	Owner  string
	Policy string
}

type SetContactPolicyResponse struct {
	Owner  string
	Policy string
}

type GetContactPolicyRequest struct {
	Owner string
}

type GetContactPolicyResponse struct {
	Owner  string
	Policy string
}

type RequestContactRequest struct {
	Requester string
	Recipient string
}

type RequestContactResponse struct {
	Requester string
	Recipient string
	Status    string
}

type RespondContactRequest struct {
	Requester string
	Recipient string
	Accept    bool
}

type RespondContactResponse struct {
	Requester string
	Recipient string
	Status    string
}

type ListContactsRequest struct {
	Owner string
}

type ListContactsResponse struct {
	Owner    string
	Contacts []string
}

func SetContactPolicy(db *sql.DB, req SetContactPolicyRequest) (SetContactPolicyResponse, error) {
	if err := storage.SetContactPolicy(db, req.Owner, req.Policy); err != nil {
		return SetContactPolicyResponse{}, err
	}
	return SetContactPolicyResponse{Owner: req.Owner, Policy: req.Policy}, nil
}

func GetContactPolicy(db *sql.DB, req GetContactPolicyRequest) (GetContactPolicyResponse, error) {
	policy, err := storage.GetContactPolicy(db, req.Owner)
	if err != nil {
		return GetContactPolicyResponse{}, err
	}
	return GetContactPolicyResponse{Owner: req.Owner, Policy: policy}, nil
}

func RequestContact(db *sql.DB, req RequestContactRequest) (RequestContactResponse, error) {
	if err := storage.RequestContact(db, req.Requester, req.Recipient); err != nil {
		return RequestContactResponse{}, err
	}
	return RequestContactResponse{Requester: req.Requester, Recipient: req.Recipient, Status: "pending"}, nil
}

func RespondContact(db *sql.DB, req RespondContactRequest) (RespondContactResponse, error) {
	if err := storage.RespondContact(db, req.Requester, req.Recipient, req.Accept); err != nil {
		return RespondContactResponse{}, err
	}
	status := "denied"
	if req.Accept {
		status = "accepted"
	}
	return RespondContactResponse{Requester: req.Requester, Recipient: req.Recipient, Status: status}, nil
}

func ListContacts(db *sql.DB, req ListContactsRequest) (ListContactsResponse, error) {
	contacts, err := storage.ListContacts(db, req.Owner)
	if err != nil {
		return ListContactsResponse{}, err
	}
	return ListContactsResponse{Owner: req.Owner, Contacts: contacts}, nil
}
