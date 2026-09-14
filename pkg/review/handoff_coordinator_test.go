package review

import (
	"context"
	"testing"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
)

func TestHandoffCoordinatorPersistsBeforeTransportAndDeduplicatesIPC(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	req := handoffRequest(t)
	r := s.Apply(req)
	tr := &handoffTransport{store: s, id: r.ID}
	c := HandoffCoordinator{Store: s, Transport: tr}
	first := c.Handle(context.Background(), Request{Version: Version, Method: "handoff.deliver", Project: req.Project, Target: r.ID})
	second := c.Handle(context.Background(), Request{Version: Version, Method: "handoff.deliver", Project: req.Project, Target: r.ID})
	if first.Error != "" || first.Delivery != agenttransport.Delivered || second.Delivery != agenttransport.Delivered || tr.sends != 1 {
		t.Fatalf("first=%+v second=%+v sends=%d", first, second, tr.sends)
	}
}

func TestHandoffCoordinatorRejectsWrongProjectBeforeTransport(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	req := handoffRequest(t)
	r := s.Apply(req)
	tr := &handoffTransport{store: s, id: r.ID}
	c := HandoffCoordinator{Store: s, Transport: tr}

	response := c.Handle(context.Background(), Request{Version: Version, Method: "handoff.deliver", Project: t.TempDir(), Target: r.ID})
	if response.Error == "" || tr.sends != 0 {
		t.Fatalf("wrong project crossed transport boundary: response=%+v sends=%d", response, tr.sends)
	}
}

func TestHandoffCoordinatorDoesNotOpenPanes(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	req := handoffRequest(t)
	r := s.Apply(req)
	tr := &handoffTransport{store: s, id: r.ID}
	c := HandoffCoordinator{Store: s, Transport: tr}

	response := c.Handle(context.Background(), Request{Version: Version, Method: "handoff.open", Project: req.Project, Target: r.ID})
	if response.Error == "" || tr.opens != 0 {
		t.Fatalf("non-human controller route opened a pane: response=%+v opens=%d", response, tr.opens)
	}
}
