package intercore

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"strconv"
)

// EventsTail returns a channel of events for a run.
// If follow is true, the channel stays open and streams new events (line-delimited JSON).
// The channel closes when the context is cancelled or the ic process exits.
func (c *Client) EventsTail(ctx context.Context, runID string, follow bool, opts ...EventsOption) (<-chan Event, error) {
	var o eventsOpts
	for _, fn := range opts {
		fn(&o)
	}

	args := append(c.baseArgs(true), "events", "tail", runID)
	if follow {
		args = append(args, "--follow")
	}
	if o.limit > 0 {
		args = append(args, "--limit="+strconv.Itoa(o.limit))
	}
	if o.consumer != "" {
		args = append(args, "--consumer="+o.consumer)
	}

	cmd := exec.CommandContext(ctx, c.binPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("intercore: events pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("intercore: events start: %w", err)
	}

	ch := make(chan Event, 64)

	go func() {
		defer close(ch)
		defer cmd.Wait() //nolint:errcheck

		if follow {
			readEventStream(ctx, stdout, ch)
		} else {
			readEventBatch(ctx, stdout, ch)
		}
	}()

	return ch, nil
}

// EventsOption configures EventsTail.
type EventsOption func(*eventsOpts)

type eventsOpts struct {
	limit    int
	consumer string
}

// WithLimit limits the number of events returned.
func WithLimit(n int) EventsOption {
	return func(o *eventsOpts) { o.limit = n }
}

// WithConsumer sets the named cursor consumer.
func WithConsumer(name string) EventsOption {
	return func(o *eventsOpts) { o.consumer = name }
}

// readEventStream reads line-delimited JSON events from a --follow stream.
func readEventStream(ctx context.Context, r io.Reader, ch chan<- Event) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // skip malformed lines
		}

		select {
		case ch <- ev:
		case <-ctx.Done():
			return
		}
	}
}

// readEventBatch reads a JSON array of events (non-follow mode).
func readEventBatch(ctx context.Context, r io.Reader, ch chan<- Event) {
	data, err := io.ReadAll(r)
	if err != nil {
		return
	}

	var events []Event
	if err := json.Unmarshal(data, &events); err != nil {
		// Try line-delimited fallback.
		readEventStream(ctx, strings.NewReader(string(data)), ch)
		return
	}

	for _, ev := range events {
		select {
		case ch <- ev:
		case <-ctx.Done():
			return
		}
	}
}
