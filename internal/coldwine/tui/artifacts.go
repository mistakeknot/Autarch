package tui

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mistakeknot/autarch/pkg/contract"
	"github.com/mistakeknot/autarch/pkg/events"
)

func recordRunLogArtifact(root, runID, logPath string) error {
	artifactDir := filepath.Join(root, ".autarch", "artifacts", runID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}

	artifactPath := filepath.Join(artifactDir, "session.log")
	if _, err := os.Lstat(artifactPath); os.IsNotExist(err) {
		if err := os.Symlink(logPath, artifactPath); err != nil {
			return err
		}
	}

	store, err := events.OpenStore("")
	if err != nil {
		return err
	}
	defer store.Close()

	writer := events.NewWriter(store, events.SourceColdwine)
	writer.SetProjectPath(root)

	artifact := contract.RunArtifact{
		ID:        runID + ":log",
		RunID:     runID,
		Type:      "log",
		Label:     "session log",
		Path:      artifactPath,
		MimeType:  "text/plain",
		CreatedAt: time.Now(),
	}

	return writer.EmitRunArtifactAdded(artifact)
}
