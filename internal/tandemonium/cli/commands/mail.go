package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/coordination"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
	"github.com/spf13/cobra"
)

func MailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mail",
		Short: "Send and receive coordination messages",
	}
	cmd.AddCommand(
		mailSendCmd(),
		mailReplyCmd(),
		mailInboxCmd(),
		mailAckCmd(),
		mailReadCmd(),
		mailSearchCmd(),
		mailSummarizeCmd(),
		mailPolicyCmd(),
		mailContactCmd(),
	)
	return cmd
}

func mailReplyCmd() *cobra.Command {
	var replyTo string
	var messageID string
	var to []string
	var cc []string
	var bcc []string
	var from string
	var subject string
	var subjectPrefix string
	var body string
	var threadID string
	var importance string
	var metadata string
	var ackRequired bool
	var attach []string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "reply",
		Short: "Reply to a message",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("reply", err)
				}
			}()
			if strings.TrimSpace(replyTo) == "" {
				return fmt.Errorf("message id required")
			}
			if strings.TrimSpace(body) == "" {
				return fmt.Errorf("body required")
			}
			if strings.TrimSpace(from) == "" {
				from = "unknown"
			}
			if strings.TrimSpace(messageID) == "" {
				messageID = fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano())
			}

			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			original, err := storage.GetMessageByID(db, replyTo)
			if err != nil {
				return err
			}

			recipients := append([]string{}, to...)
			recipients = append(recipients, cc...)
			recipients = append(recipients, bcc...)
			if len(recipients) == 0 {
				if strings.TrimSpace(original.Sender) == "" {
					return fmt.Errorf("recipient required")
				}
				recipients = []string{original.Sender}
				to = []string{original.Sender}
			}

			if strings.TrimSpace(threadID) == "" {
				if strings.TrimSpace(original.ThreadID) != "" {
					threadID = original.ThreadID
				} else {
					threadID = original.ID
				}
			}

			if strings.TrimSpace(subject) == "" {
				subject = replySubject(subjectPrefix, original.Subject)
			}

			if strings.TrimSpace(importance) == "" {
				importance = original.Importance
			}
			if !cmd.Flags().Changed("ack") {
				ackRequired = original.AckRequired
			}

			if err := enforceContactPolicies(db, from, recipients); err != nil {
				return err
			}

			mergedMetadata, err := storage.MergeRecipientMetadata(metadata, to, cc, bcc)
			if err != nil {
				return err
			}

			msg := storage.Message{
				ID:          messageID,
				ThreadID:    threadID,
				Sender:      from,
				Subject:     subject,
				Body:        body,
				CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
				Importance:  importance,
				AckRequired: ackRequired,
				Metadata:    mergedMetadata,
			}

			if err := storage.SendMessage(db, msg, recipients); err != nil {
				return err
			}
			if len(attach) > 0 {
				root, err := project.FindRoot(".")
				if err != nil {
					return err
				}
				storeDir := project.AttachmentsDir(root)
				attachments := make([]storage.Attachment, 0, len(attach))
				for _, item := range attach {
					path, note := parseAttachment(item)
					if _, err := os.Stat(path); err != nil {
						return err
					}
					attachments = append(attachments, storage.Attachment{
						MessageID: msg.ID,
						Path:      path,
						Note:      note,
					})
				}
				if err := storage.AddAttachmentsWithStore(db, storeDir, msg.ID, attachments); err != nil {
					return err
				}
			}

			if jsonOut {
				payload := map[string]interface{}{
					"id":         msg.ID,
					"thread_id":  msg.ThreadID,
					"recipients": recipients,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Sent %s to %s\n", msg.ID, strings.Join(recipients, ","))
			return nil
		},
	}

	cmd.Flags().StringVar(&replyTo, "id", "", "Message id to reply to")
	cmd.Flags().StringVar(&messageID, "message-id", "", "New message id")
	cmd.Flags().StringSliceVar(&to, "to", nil, "Recipients")
	cmd.Flags().StringSliceVar(&cc, "cc", nil, "Cc recipients")
	cmd.Flags().StringSliceVar(&bcc, "bcc", nil, "Bcc recipients")
	cmd.Flags().StringVar(&from, "from", "", "Sender name")
	cmd.Flags().StringVar(&subject, "subject", "", "Message subject")
	cmd.Flags().StringVar(&subjectPrefix, "subject-prefix", "Re:", "Subject prefix when auto-generating")
	cmd.Flags().StringVar(&body, "body", "", "Message body")
	cmd.Flags().StringVar(&threadID, "thread", "", "Thread id override")
	cmd.Flags().StringVar(&importance, "importance", "", "Importance (low|normal|high|urgent)")
	cmd.Flags().StringVar(&metadata, "metadata", "", "Metadata JSON")
	cmd.Flags().BoolVar(&ackRequired, "ack", false, "Require acknowledgement")
	cmd.Flags().StringSliceVar(&attach, "attach", nil, "Attachment path (use path::note for note)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func mailSendCmd() *cobra.Command {
	var to []string
	var cc []string
	var bcc []string
	var from string
	var subject string
	var body string
	var threadID string
	var messageID string
	var importance string
	var metadata string
	var ackRequired bool
	var attach []string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message to recipients",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("send", err)
				}
			}()
			if len(to) == 0 {
				return fmt.Errorf("recipient required")
			}
			if strings.TrimSpace(subject) == "" {
				return fmt.Errorf("subject required")
			}
			if strings.TrimSpace(body) == "" {
				return fmt.Errorf("body required")
			}
			if strings.TrimSpace(from) == "" {
				from = "unknown"
			}
			if strings.TrimSpace(messageID) == "" {
				messageID = fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano())
			}

			recipients := append([]string{}, to...)
			recipients = append(recipients, cc...)
			recipients = append(recipients, bcc...)

			mergedMetadata, err := storage.MergeRecipientMetadata(metadata, to, cc, bcc)
			if err != nil {
				return err
			}

			msg := storage.Message{
				ID:          messageID,
				ThreadID:    threadID,
				Sender:      from,
				Subject:     subject,
				Body:        body,
				CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
				Importance:  importance,
				AckRequired: ackRequired,
				Metadata:    mergedMetadata,
			}

			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			if err := enforceContactPolicies(db, from, recipients); err != nil {
				return err
			}
			if err := storage.SendMessage(db, msg, recipients); err != nil {
				return err
			}
			if len(attach) > 0 {
				root, err := project.FindRoot(".")
				if err != nil {
					return err
				}
				storeDir := project.AttachmentsDir(root)
				attachments := make([]storage.Attachment, 0, len(attach))
				for _, item := range attach {
					path, note := parseAttachment(item)
					if _, err := os.Stat(path); err != nil {
						return err
					}
					attachments = append(attachments, storage.Attachment{
						MessageID: msg.ID,
						Path:      path,
						Note:      note,
					})
				}
				if err := storage.AddAttachmentsWithStore(db, storeDir, msg.ID, attachments); err != nil {
					return err
				}
			}
			if jsonOut {
				payload := map[string]interface{}{
					"id":         msg.ID,
					"thread_id":  msg.ThreadID,
					"recipients": recipients,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Sent %s to %s\n", msg.ID, strings.Join(recipients, ","))
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&to, "to", nil, "Recipients")
	cmd.Flags().StringSliceVar(&cc, "cc", nil, "Cc recipients")
	cmd.Flags().StringSliceVar(&bcc, "bcc", nil, "Bcc recipients")
	cmd.Flags().StringVar(&from, "from", "", "Sender name")
	cmd.Flags().StringVar(&subject, "subject", "", "Message subject")
	cmd.Flags().StringVar(&body, "body", "", "Message body")
	cmd.Flags().StringVar(&threadID, "thread", "", "Thread id")
	cmd.Flags().StringVar(&messageID, "id", "", "Message id")
	cmd.Flags().StringVar(&importance, "importance", "normal", "Importance (low|normal|high|urgent)")
	cmd.Flags().StringVar(&metadata, "metadata", "", "Metadata JSON")
	cmd.Flags().BoolVar(&ackRequired, "ack", false, "Require acknowledgement")
	cmd.Flags().StringSliceVar(&attach, "attach", nil, "Attachment path (use path::note for note)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func mailPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Manage contact policies",
	}
	cmd.AddCommand(mailPolicySetCmd(), mailPolicyGetCmd())
	return cmd
}

func mailPolicySetCmd() *cobra.Command {
	var owner string
	var policy string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set contact policy",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("policy set", err)
				}
			}()
			if strings.TrimSpace(owner) == "" {
				return fmt.Errorf("owner required")
			}
			if strings.TrimSpace(policy) == "" {
				return fmt.Errorf("policy required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()
			if err := storage.SetContactPolicy(db, owner, policy); err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]interface{}{
					"owner":  owner,
					"policy": policy,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "policy %s %s\n", owner, policy)
			return nil
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "Owner name")
	cmd.Flags().StringVar(&policy, "policy", "", "Policy (open|auto|contacts_only|block_all)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func mailPolicyGetCmd() *cobra.Command {
	var owner string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get contact policy",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("policy get", err)
				}
			}()
			if strings.TrimSpace(owner) == "" {
				return fmt.Errorf("owner required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()
			policy, err := storage.GetContactPolicy(db, owner)
			if err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]interface{}{
					"owner":  owner,
					"policy": policy,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "policy %s %s\n", owner, policy)
			return nil
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "Owner name")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func mailContactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contact",
		Short: "Manage contact requests",
	}
	cmd.AddCommand(mailContactRequestCmd(), mailContactRespondCmd(), mailContactListCmd())
	return cmd
}

func mailContactRequestCmd() *cobra.Command {
	var requester string
	var recipient string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "request",
		Short: "Request contact approval",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("contact request", err)
				}
			}()
			if strings.TrimSpace(requester) == "" || strings.TrimSpace(recipient) == "" {
				return fmt.Errorf("requester and recipient required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()
			if err := storage.RequestContact(db, requester, recipient); err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]interface{}{
					"requester": requester,
					"recipient": recipient,
					"status":    "pending",
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "requested %s -> %s\n", requester, recipient)
			return nil
		},
	}
	cmd.Flags().StringVar(&requester, "requester", "", "Requester name")
	cmd.Flags().StringVar(&recipient, "recipient", "", "Recipient name")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func mailContactRespondCmd() *cobra.Command {
	var requester string
	var recipient string
	var accept bool
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "respond",
		Short: "Respond to contact request",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("contact respond", err)
				}
			}()
			if strings.TrimSpace(requester) == "" || strings.TrimSpace(recipient) == "" {
				return fmt.Errorf("requester and recipient required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()
			if err := storage.RespondContact(db, requester, recipient, accept); err != nil {
				return err
			}
			status := "denied"
			if accept {
				status = "accepted"
			}
			if jsonOut {
				payload := map[string]interface{}{
					"requester": requester,
					"recipient": recipient,
					"status":    status,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "responded %s\n", status)
			return nil
		},
	}
	cmd.Flags().StringVar(&requester, "requester", "", "Requester name")
	cmd.Flags().StringVar(&recipient, "recipient", "", "Recipient name")
	cmd.Flags().BoolVar(&accept, "accept", false, "Accept request")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func mailContactListCmd() *cobra.Command {
	var owner string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List accepted contacts",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("contact list", err)
				}
			}()
			if strings.TrimSpace(owner) == "" {
				return fmt.Errorf("owner required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()
			contacts, err := storage.ListContacts(db, owner)
			if err != nil {
				return err
			}
			sort.Strings(contacts)
			if jsonOut {
				payload := map[string]interface{}{
					"owner":    owner,
					"contacts": contacts,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "contacts %s %s\n", owner, strings.Join(contacts, ","))
			return nil
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "Owner name")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func mailInboxCmd() *cobra.Command {
	var recipient string
	var limit int
	var since string
	var urgentOnly bool
	var pageToken string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "List inbox messages for a recipient",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("inbox", err)
				}
			}()
			if strings.TrimSpace(recipient) == "" {
				return fmt.Errorf("recipient required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			deliveries, nextToken, err := storage.FetchInboxPage(db, recipient, limit, since, urgentOnly, pageToken)
			if err != nil {
				return err
			}
			if jsonOut {
				messageIDs := make([]string, 0, len(deliveries))
				for _, d := range deliveries {
					messageIDs = append(messageIDs, d.Message.ID)
				}
				attachmentsByMessage, err := storage.ListAttachmentsForMessages(db, messageIDs)
				if err != nil {
					return err
				}
				payload := struct {
					Messages []struct {
						ID          string               `json:"id"`
						ThreadID    string               `json:"thread_id"`
						Sender      string               `json:"sender"`
						Subject     string               `json:"subject"`
						Body        string               `json:"body"`
						CreatedAt   string               `json:"created_ts"`
						Importance  string               `json:"importance"`
						AckRequired bool                 `json:"ack_required"`
						Recipient   string               `json:"recipient"`
						ReadAt      string               `json:"read_ts"`
						AckAt       string               `json:"ack_ts"`
						Attachments []storage.Attachment `json:"attachments"`
						To          []string             `json:"to"`
						Cc          []string             `json:"cc"`
						Bcc         []string             `json:"bcc"`
					} `json:"messages"`
					NextToken string `json:"next_token"`
				}{Messages: make([]struct {
					ID          string               `json:"id"`
					ThreadID    string               `json:"thread_id"`
					Sender      string               `json:"sender"`
					Subject     string               `json:"subject"`
					Body        string               `json:"body"`
					CreatedAt   string               `json:"created_ts"`
					Importance  string               `json:"importance"`
					AckRequired bool                 `json:"ack_required"`
					Recipient   string               `json:"recipient"`
					ReadAt      string               `json:"read_ts"`
					AckAt       string               `json:"ack_ts"`
					Attachments []storage.Attachment `json:"attachments"`
					To          []string             `json:"to"`
					Cc          []string             `json:"cc"`
					Bcc         []string             `json:"bcc"`
				}, 0, len(deliveries)), NextToken: nextToken}
				for _, d := range deliveries {
					to, cc, bcc := storage.ParseRecipientMetadata(d.Message.Metadata)
					payload.Messages = append(payload.Messages, struct {
						ID          string               `json:"id"`
						ThreadID    string               `json:"thread_id"`
						Sender      string               `json:"sender"`
						Subject     string               `json:"subject"`
						Body        string               `json:"body"`
						CreatedAt   string               `json:"created_ts"`
						Importance  string               `json:"importance"`
						AckRequired bool                 `json:"ack_required"`
						Recipient   string               `json:"recipient"`
						ReadAt      string               `json:"read_ts"`
						AckAt       string               `json:"ack_ts"`
						Attachments []storage.Attachment `json:"attachments"`
						To          []string             `json:"to"`
						Cc          []string             `json:"cc"`
						Bcc         []string             `json:"bcc"`
					}{
						ID:          d.Message.ID,
						ThreadID:    d.Message.ThreadID,
						Sender:      d.Message.Sender,
						Subject:     d.Message.Subject,
						Body:        d.Message.Body,
						CreatedAt:   d.Message.CreatedAt,
						Importance:  d.Message.Importance,
						AckRequired: d.Message.AckRequired,
						Recipient:   d.Recipient,
						ReadAt:      d.ReadAt,
						AckAt:       d.AckAt,
						Attachments: attachmentsByMessage[d.Message.ID],
						To:          to,
						Cc:          cc,
						Bcc:         bcc,
					})
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			for _, d := range deliveries {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", d.Message.ID, d.Message.Sender, d.Message.Subject)
			}
			if strings.TrimSpace(nextToken) != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "next_token %s\n", nextToken)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&recipient, "recipient", "", "Recipient name")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max messages")
	cmd.Flags().StringVar(&since, "since", "", "Only messages created at or after this timestamp (RFC3339)")
	cmd.Flags().BoolVar(&urgentOnly, "urgent-only", false, "Only urgent messages")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Page token from previous response")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func mailAckCmd() *cobra.Command {
	var messageID string
	var recipient string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "ack",
		Short: "Acknowledge a message",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("ack", err)
				}
			}()
			if strings.TrimSpace(messageID) == "" {
				return fmt.Errorf("message id required")
			}
			if strings.TrimSpace(recipient) == "" {
				return fmt.Errorf("recipient required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			if err := storage.AckMessage(db, messageID, recipient, ""); err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]interface{}{
					"id":        messageID,
					"recipient": recipient,
					"acked":     true,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "acked %s\n", messageID)
			return nil
		},
	}

	cmd.Flags().StringVar(&messageID, "id", "", "Message id")
	cmd.Flags().StringVar(&recipient, "recipient", "", "Recipient name")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func mailReadCmd() *cobra.Command {
	var messageID string
	var recipient string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "read",
		Short: "Mark a message as read",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("read", err)
				}
			}()
			if strings.TrimSpace(messageID) == "" {
				return fmt.Errorf("message id required")
			}
			if strings.TrimSpace(recipient) == "" {
				return fmt.Errorf("recipient required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			if err := storage.MarkMessageRead(db, messageID, recipient, ""); err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]interface{}{
					"id":        messageID,
					"recipient": recipient,
					"read":      true,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "read %s\n", messageID)
			return nil
		},
	}

	cmd.Flags().StringVar(&messageID, "id", "", "Message id")
	cmd.Flags().StringVar(&recipient, "recipient", "", "Recipient name")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func mailSearchCmd() *cobra.Command {
	var query string
	var limit int
	var pageToken string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search messages",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("search", err)
				}
			}()
			if strings.TrimSpace(query) == "" {
				return fmt.Errorf("query required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			msgs, nextToken, err := storage.SearchMessagesPage(db, query, limit, pageToken)
			if err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]interface{}{
					"messages":   msgs,
					"next_token": nextToken,
				}
				return writeJSON(cmd, payload)
			}
			for _, msg := range msgs {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", msg.ID, msg.Sender, msg.Subject)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&query, "query", "", "Search query")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max results")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Page token from previous response")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func mailSummarizeCmd() *cobra.Command {
	var threadID string
	var jsonOut bool
	var llmMode bool
	var includeExamples bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "summarize",
		Short: "Summarize a thread",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapMailError("summarize", err)
				}
			}()
			if strings.TrimSpace(threadID) == "" && !dryRun {
				return fmt.Errorf("thread id required")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			useLLM := llmMode || includeExamples || dryRun
			llmConfig := config.LLMSummaryConfig{}
			if useLLM {
				root, err := project.FindRoot(".")
				if err != nil {
					return err
				}
				cfg, err := config.LoadFromProject(root)
				if err != nil {
					return err
				}
				llmConfig = cfg.LLMSummary
			}
			if dryRun {
				input := coordination.LLMSummaryInput{
					ThreadID:        "dry-run",
					IncludeExamples: includeExamples,
					Messages: []coordination.LLMMessage{
						{ID: "m1", Sender: "alice", Subject: "Example", Body: "Test message"},
						{ID: "m2", Sender: "bob", Subject: "Follow-up", Body: "Second test message"},
					},
				}
				output, err := coordination.RunLLMSummaryCommand(context.Background(), llmConfig, input)
				if err != nil {
					return err
				}
				if jsonOut {
					payload := map[string]interface{}{
						"thread_id":    input.ThreadID,
						"participants": output.Summary.Participants,
						"key_points":   output.Summary.KeyPoints,
						"action_items": output.Summary.ActionItems,
						"examples":     output.Examples,
					}
					return writeJSON(cmd, payload)
				}
				if len(output.Summary.Participants) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "participants: %s\n", strings.Join(output.Summary.Participants, ","))
				}
				if len(output.Summary.KeyPoints) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "key_points: %s\n", strings.Join(output.Summary.KeyPoints, "; "))
				}
				if len(output.Summary.ActionItems) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "action_items: %s\n", strings.Join(output.Summary.ActionItems, "; "))
				}
				if includeExamples && len(output.Examples) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "examples: %d\n", len(output.Examples))
				}
				return nil
			}

			summary, err := coordination.SummarizeThread(db, coordination.SummarizeThreadRequest{
				ThreadID:        threadID,
				IncludeExamples: includeExamples,
				LLMMode:         useLLM,
				LLMConfig:       llmConfig,
			})
			if err != nil {
				return err
			}
			participants := append([]string(nil), summary.Participants...)
			sort.Strings(participants)
			if jsonOut {
				payload := map[string]interface{}{
					"thread_id":     summary.ThreadID,
					"participants":  participants,
					"message_count": summary.MessageCount,
					"key_points":    summary.KeyPoints,
					"action_items":  summary.ActionItems,
					"examples":      summary.Examples,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "thread %s messages=%d participants=%s\n", summary.ThreadID, summary.MessageCount, strings.Join(participants, ","))
			if useLLM {
				if len(summary.KeyPoints) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "key_points: %s\n", strings.Join(summary.KeyPoints, "; "))
				}
				if len(summary.ActionItems) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "action_items: %s\n", strings.Join(summary.ActionItems, "; "))
				}
				if includeExamples && len(summary.Examples) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "examples: %d\n", len(summary.Examples))
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&threadID, "thread", "", "Thread id")
	cmd.Flags().BoolVar(&llmMode, "llm", false, "Use LLM-backed summary command")
	cmd.Flags().BoolVar(&includeExamples, "examples", false, "Include example messages (requires LLM mode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate LLM summary command with synthetic input")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func enforceContactPolicies(db *sql.DB, sender string, recipients []string) error {
	for _, r := range recipients {
		policy, err := storage.GetContactPolicy(db, r)
		if err != nil {
			return err
		}
		switch policy {
		case "block_all":
			return fmt.Errorf("recipient %s blocks all contacts", r)
		case "contacts_only":
			ok, err := storage.HasAcceptedContact(db, sender, r)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("contact required for %s", r)
			}
		}
	}
	return nil
}

func wrapMailError(command string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("mail %s failed: %w", command, err)
}

func parseAttachment(raw string) (string, string) {
	parts := strings.SplitN(raw, "::", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func replySubject(prefix, original string) string {
	prefix = strings.TrimSpace(prefix)
	original = strings.TrimSpace(original)
	if prefix == "" {
		return original
	}
	if original == "" {
		return prefix
	}
	if strings.HasPrefix(strings.ToLower(original), strings.ToLower(prefix)) {
		return original
	}
	return prefix + " " + original
}
