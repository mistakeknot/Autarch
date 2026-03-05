package clavain

import (
	"testing"

	"github.com/mistakeknot/intercore/pkg/contract"
)

func TestBuildIntent(t *testing.T) {
	intent := contract.Intent{
		Type:           contract.IntentSprintAdvance,
		BeadID:         "iv-abc123",
		IdempotencyKey: "sess-123-sprint.advance-iv-abc123",
		SessionID:      "sess-123",
		Timestamp:      1772749697,
		Params:         map[string]any{"phase": "executing"},
	}
	if err := intent.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestBuildIntentValidation(t *testing.T) {
	tests := []struct {
		name    string
		intent  contract.Intent
		wantErr bool
	}{
		{
			name: "valid sprint advance",
			intent: contract.Intent{
				Type:           contract.IntentSprintAdvance,
				BeadID:         "iv-abc123",
				IdempotencyKey: "key-1",
				SessionID:      "sess-1",
				Timestamp:      1772749697,
			},
			wantErr: false,
		},
		{
			name: "valid gate enforce",
			intent: contract.Intent{
				Type:           contract.IntentGateEnforce,
				BeadID:         "iv-abc123",
				IdempotencyKey: "key-2",
				SessionID:      "sess-1",
				Timestamp:      1772749697,
				Params:         map[string]any{"target_phase": "testing"},
			},
			wantErr: false,
		},
		{
			name: "missing type",
			intent: contract.Intent{
				IdempotencyKey: "key-3",
				SessionID:      "sess-1",
				Timestamp:      1772749697,
			},
			wantErr: true,
		},
		{
			name: "missing idempotency key",
			intent: contract.Intent{
				Type:      contract.IntentSprintAdvance,
				SessionID: "sess-1",
				Timestamp: 1772749697,
			},
			wantErr: true,
		},
		{
			name: "missing session ID",
			intent: contract.Intent{
				Type:           contract.IntentSprintAdvance,
				IdempotencyKey: "key-4",
				Timestamp:      1772749697,
			},
			wantErr: true,
		},
		{
			name: "missing timestamp",
			intent: contract.Intent{
				Type:           contract.IntentSprintAdvance,
				IdempotencyKey: "key-5",
				SessionID:      "sess-1",
			},
			wantErr: true,
		},
		{
			name: "unknown type",
			intent: contract.Intent{
				Type:           "invalid.type",
				IdempotencyKey: "key-6",
				SessionID:      "sess-1",
				Timestamp:      1772749697,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.intent.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
