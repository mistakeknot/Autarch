package homeask

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Content-addressed names let retries observe a successful upload after a lost answer.
func (f *CardFiler) ensureAttachments(ctx context.Context, r CardRequest, id string) error {
	if len(r.Attachments) == 0 {
		return nil
	}
	if err := f.CheckUI(CardRequest{Attachments: r.Attachments}); err != nil {
		return err
	}
	var listed struct {
		Attachments []struct {
			Name     string `json:"name"`
			FileName string `json:"fileName"`
		}
	}
	if err := f.call(ctx, r.Thread, &listed, "tasks", "attachment", "list", id, "--json"); err != nil {
		return err
	}
	names := map[string]bool{}
	for _, a := range listed.Attachments {
		names[a.Name] = true
		names[a.FileName] = true
	}
	for _, path := range r.Attachments {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
		name := fmt.Sprintf("proof-%x%s", hash.Sum(nil), strings.ToLower(filepath.Ext(path)))
		if names[name] {
			continue
		}
		if err = f.call(ctx, r.Thread, nil, "tasks", "attachment", "add", id, "--file", path, "--name", name, "--json"); err != nil {
			return fmt.Errorf("%w: attachment outcome unknown; retry same request: %v", ErrOutcomeUnknown, err)
		}
		names[name] = true
	}
	return nil
}
