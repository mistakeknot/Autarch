package signals

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type subscriber struct {
	ch    chan Signal
	types map[SignalType]bool
}

// Broker fans out signals to subscribers.
type Broker struct {
	mu      sync.Mutex
	subs    map[*subscriber]struct{}
	Dropped atomic.Int64
}

// NewBroker creates a new broker.
func NewBroker() *Broker {
	return &Broker{subs: make(map[*subscriber]struct{})}
}

// Subscribe registers a subscriber for the given signal types.
// Empty types means all.
func (b *Broker) Subscribe(types []SignalType) *Subscription {
	sub := &subscriber{ch: make(chan Signal, 64), types: make(map[SignalType]bool)}
	for _, t := range types {
		sub.types[t] = true
	}
	b.mu.Lock()
	b.subs[sub] = struct{}{}
	b.mu.Unlock()
	return &Subscription{broker: b, sub: sub}
}

// Publish broadcasts a signal to subscribers.
func (b *Broker) Publish(sig Signal) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for sub := range b.subs {
		if len(sub.types) > 0 {
			if !sub.types[sig.Type] {
				continue
			}
		}
		select {
		case sub.ch <- sig:
		default:
			// Channel is full: evict oldest queued signal so newest wins.
			select {
			case <-sub.ch:
				b.Dropped.Add(1)
			default:
			}
			// Second non-blocking attempt after eviction.
			select {
			case sub.ch <- sig:
			default:
				b.Dropped.Add(1)
			}
		}
	}
}

// ServeWS upgrades the connection and streams signals as JSON.
func (b *Broker) ServeWS(w http.ResponseWriter, r *http.Request, types []SignalType) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "closing")

	sub := b.Subscribe(types)
	defer sub.Close()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case sig := <-sub.sub.ch:
			if err := wsjson.Write(ctx, conn, sig); err != nil {
				return
			}
		}
	}
}

// Subscription represents an active broker subscription.
type Subscription struct {
	broker *Broker
	sub    *subscriber
}

// Chan exposes the signal channel.
func (s *Subscription) Chan() <-chan Signal {
	return s.sub.ch
}

// Close removes the subscription.
func (s *Subscription) Close() {
	if s == nil || s.broker == nil || s.sub == nil {
		return
	}
	s.broker.mu.Lock()
	delete(s.broker.subs, s.sub)
	close(s.sub.ch)
	s.broker.mu.Unlock()
}

// Stream writes signals to a channel until context closes.
func (s *Subscription) Stream(ctx context.Context, out chan<- Signal) {
	for {
		select {
		case <-ctx.Done():
			return
		case sig := <-s.sub.ch:
			out <- sig
		}
	}
}
