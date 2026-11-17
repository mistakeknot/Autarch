// Package callerlog provides best-effort, bounded asynchronous audit logging.
package callerlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type item struct {
	fields map[string]any
	remote string
}

// Log owns one bounded writer. Request paths never wait for its worker.
type Log struct {
	dir      string
	queue    chan item
	mu       sync.RWMutex
	closed   bool
	done     chan struct{}
	dropped  atomic.Uint64
	maxBytes int64
	warned   bool
}

const defaultMaxBytes int64 = 8 << 20

// Option configures a logger before its worker starts.
type Option func(*Log)

// WithMaxBytes lowers the pilot's 8 MiB file cap, including existing contents.
// Zero disables writes. Negative values and increases are ignored.
func WithMaxBytes(n int64) Option {
	return func(l *Log) {
		if n >= 0 && n < l.maxBytes {
			l.maxBytes = n
		}
	}
}

// New writes into a dedicated audit directory, which it owns and secures.
// Pass an empty directory for a drop-only logger.
func New(dir string, opts ...Option) *Log {
	if dir == "" {
		done := make(chan struct{})
		close(done)
		return &Log{closed: true, done: done}
	}
	l := &Log{dir: dir, queue: make(chan item, 256), done: make(chan struct{}), maxBytes: defaultMaxBytes}
	for _, opt := range opts {
		opt(l)
	}
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
		if err := l.appendLine(append(data, '\n')); err != nil {
			l.dropped.Add(1)
			if !l.warned {
				l.warned = true
				fmt.Fprintln(os.Stderr, "callerlog: audit records dropped (storage unavailable or size limit reached)")
			}
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
		dir = filepath.Join(home, ".autarch", "audit")
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

// openAuditDir checks the path with Lstat, then checks the opened directory
// again before changing permissions. The descriptor pins the validated inode.
func openAuditDir(dir string, uid uint32) (*os.File, error) {
	fi, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		fi, err = os.Lstat(dir)
	}
	if err != nil {
		return nil, err
	}
	if !ownedDirectory(fi, uid) {
		return nil, errors.New("unsafe audit directory")
	}
	f, err := os.OpenFile(dir, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	current, err := f.Stat()
	if err != nil || !os.SameFile(fi, current) || !ownedDirectory(current, uid) {
		f.Close()
		return nil, errors.New("audit directory changed")
	}
	if current.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0700 {
		if err := f.Chmod(0700); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func ownedDirectory(fi os.FileInfo, uid uint32) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return fi.IsDir() && ok && st.Uid == uid
}

func (l *Log) appendLine(line []byte) (result error) {
	dir, err := openAuditDir(l.dir, uint32(os.Geteuid()))
	if err != nil {
		return err
	}
	defer dir.Close()
	// Open relative to the checked directory, refusing symlinks and blocking FIFOs.
	fd, err := unix.Openat(int(dir.Fd()), "caller-log.jsonl", unix.O_CREAT|unix.O_APPEND|unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), "caller-log.jsonl")
	defer func() {
		if err := f.Close(); result == nil {
			result = err
		}
	}()
	// Keep stat + append atomic across cooperating serve and MCP processes.
	// Contention drops a record rather than delaying worker shutdown.
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return err
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.Mode().IsRegular() || !ok || st.Uid != uint32(os.Geteuid()) {
		return errors.New("unsafe audit file")
	}
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if fi.Size() >= l.maxBytes || int64(len(line)) > l.maxBytes-fi.Size() {
		return errors.New("audit size limit reached")
	}
	n, err := f.Write(line)
	if err == nil && n != len(line) {
		return errors.New("short audit write")
	}
	if err != nil {
		return err
	}
	return nil
}
