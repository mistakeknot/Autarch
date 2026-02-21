package aggregator

import (
	"testing"

	"github.com/mistakeknot/autarch/internal/bigend/config"
)

func TestNewAggregatorHasBroker(t *testing.T) {
	agg := New(nil, &config.Config{}, nil)
	if agg.Broker() == nil {
		t.Fatal("expected non-nil broker after New()")
	}
}
