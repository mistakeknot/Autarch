package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCancelDeliveredHasExplicitFeedback(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	ctx := context.Background()
	r := testReq()
	r.NonBlocking = true
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	groups, err := f.Release(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := groups["other"][0].ID
	err = f.CancelHeld(r.Key)
	if err == nil || !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "already delivered as card "+id+"; nothing to cancel") {
		t.Fatalf("operator received silent or misleading cancellation: %v", err)
	}
	budget, err := f.BudgetStatus("autarch")
	if err != nil || budget.Ordinary != 1 {
		t.Fatal("cancellation changed delivery charge", budget, err)
	}
}
