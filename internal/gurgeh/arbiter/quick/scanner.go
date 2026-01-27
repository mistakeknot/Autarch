package quick

import (
	"context"
	"time"
)

// ScanResult holds research findings.
type ScanResult struct {
	Topic     string
	Summary   string
	ScannedAt time.Time
}

// Scanner performs quick research scans.
type Scanner struct{}

// NewScanner creates a new Scanner.
func NewScanner() *Scanner {
	return &Scanner{}
}

// Scan performs a quick scan for the given topic and project path.
func (s *Scanner) Scan(_ context.Context, topic string, projectPath string) (*ScanResult, error) {
	_ = projectPath
	return &ScanResult{
		Topic:     topic,
		Summary:   "Quick scan results for: " + topic,
		ScannedAt: time.Now(),
	}, nil
}
