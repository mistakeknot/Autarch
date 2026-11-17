// Package callerlog provides best-effort, bounded asynchronous audit logging.
package callerlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type item struct {
	fields map[string]any
	remote string
}

// Log owns one bounded writer. Request paths never wait for its worker.
type Log struct {
	dir     string
	queue   chan item
	mu      sync.RWMutex
	closed  bool
	done    chan struct{}
	dropped atomic.Uint64
}

func New(dir string) *Log {
	if dir == "" {
		done := make(chan struct{})
		close(done)
		return &Log{closed: true, done: done}
	}
	l := &Log{dir: dir, queue: make(chan item, 256), done: make(chan struct{})}
	go l.run()
	return l
}
func (l *Log) Dir() string { return l.dir }

// Dropped counts queue overflow, lookup-independent serialization and write failures.
func (l *Log) Dropped() uint64 { return l.dropped.Load() }

// Record takes ownership of fields. Callers must not mutate it after this call.
func (l *Log) Record(fields map[string]any, remote string) {
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.closed {
		l.dropped.Add(1)
		return
	}
	select {
	case l.queue <- item{fields, remote}:
	default:
		l.dropped.Add(1)
	}
}

// Close drains queued lines. It is for lifecycle owners, never request handlers.
func (l *Log) Close() {
	l.mu.Lock()
	if !l.closed {
		l.closed = true
		close(l.queue)
	}
	l.mu.Unlock()
	<-l.done
}
func (l *Log) run() {
	defer close(l.done)
	for it := range l.queue {
		if it.remote != "" {
			it.fields["peer"] = resolvePeer(it.remote)
		}
		data, err := json.Marshal(it.fields)
		if err != nil {
			l.dropped.Add(1)
			continue
		}
		if os.MkdirAll(l.dir, 0700) != nil {
			l.dropped.Add(1)
			continue
		}
		fi, err := os.Stat(l.dir)
		if err != nil || !fi.IsDir() || fi.Mode().Perm()&0077 != 0 {
			l.dropped.Add(1)
			continue
		}
		f, err := os.OpenFile(filepath.Join(l.dir, "caller-log.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
		if err != nil {
			l.dropped.Add(1)
			continue
		}
		// Existing logs must be private too.
		if err = f.Chmod(0600); err == nil {
			_, err = f.Write(append(data, '\n'))
		}
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			l.dropped.Add(1)
		}
	}
}

var defaults sync.Map

// Default shares one bounded worker per home directory for process lifetime.
func Default() *Log {
	home, err := os.UserHomeDir()
	dir := ""
	// Test binaries must explicitly inject a log; never use the process HOME.
	if !testing.Testing() && err == nil && home != "" {
		dir = filepath.Join(home, ".autarch")
	}
	if v, ok := defaults.Load(dir); ok {
		return v.(*Log)
	}
	l := New(dir)
	v, loaded := defaults.LoadOrStore(dir, l)
	if loaded {
		l.Close()
	}
	return v.(*Log)
}
