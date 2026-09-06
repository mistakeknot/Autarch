package review

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
)

const maxMessage = 8 << 20

func DefaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".autarch", "reviews")
}
func DefaultSocket() string { return filepath.Join(DefaultDir(), "controller.sock") }
func BuildIdentity() string {
	if b, ok := debug.ReadBuildInfo(); ok {
		revision := "unknown"
		dirty := ""
		for _, s := range b.Settings {
			if s.Key == "vcs.revision" {
				revision = s.Value
			}
			if s.Key == "vcs.modified" && s.Value == "true" {
				dirty = "+modified"
			}
		}
		return revision + dirty
	}
	return "unknown"
}

type Client struct{ Socket string }

func (c Client) Call(ctx context.Context, r Request) (Response, error) {
	if c.Socket == "" {
		c.Socket = DefaultSocket()
	}
	if r.Version == 0 {
		r.Version = Version
	}
	if r.ID == "" {
		r.ID = NewID()
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	deadline := time.Now().Add(10 * time.Second)
	if r.Method == "trace" {
		deadline = time.Now().Add(60 * time.Second)
	}
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if err = json.NewEncoder(conn).Encode(r); err != nil {
		return Response{}, err
	}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), maxMessage)
	if !scanner.Scan() {
		if err = scanner.Err(); err == nil {
			err = errors.New("controller disconnected before acknowledgement")
		}
		return Response{}, err
	}
	var response Response
	if err = json.Unmarshal(scanner.Bytes(), &response); err != nil {
		return response, err
	}
	if response.Version != Version {
		return response, errors.New("controller version mismatch")
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, nil
}

type Server struct {
	OnQuery   func(Request) Response
	listener  net.Listener
	lock      *os.File
	store     *Store
	OnRequest func(Request)
}

func Listen(socket string, store *Store) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(socket), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(socket+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("review controller already running")
	}
	// Only the process holding the lock can remove a stale socket.
	if err = os.Remove(socket); err != nil && !os.IsNotExist(err) {
		lock.Close()
		return nil, err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		lock.Close()
		return nil, err
	}
	if err = os.Chmod(socket, 0600); err != nil {
		listener.Close()
		lock.Close()
		return nil, err
	}
	return &Server{listener: listener, lock: lock, store: store}, nil
}
func (s *Server) Close() error { err := s.listener.Close(); _ = s.lock.Close(); return err }
func (s *Server) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}
func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), maxMessage)
	if !scanner.Scan() {
		return
	}
	var req Request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{Version: Version, Error: "invalid request JSON"})
		return
	}
	var response Response
	if req.Method == "trace" {
		_ = conn.SetDeadline(time.Now().Add(65 * time.Second))
	}
	if req.Version != Version {
		response = Response{Version: Version, Error: "unsupported IPC version"}
	} else if strings.HasPrefix(req.Method, "auth.") {
		response = Response{Version: Version, ID: req.ID, Error: "provider connection unavailable"}
		if s.OnQuery != nil {
			response = s.OnQuery(req)
		}
	} else if req.Auth != nil {
		response = Response{Version: Version, ID: req.ID, Error: "authentication input requires the authentication channel"}
	} else if (req.Method == "trace" || req.Method == "execution.launch") && s.OnQuery != nil {
		response = s.OnQuery(req)
	} else {
		response = s.store.Apply(req)
	}
	if req.Method == "state" {
		response.StorageBytes = s.store.Usage()
	}
	_ = json.NewEncoder(conn).Encode(response)
	if response.Error == "" && !response.Replayed && s.OnRequest != nil && !strings.HasPrefix(req.Method, "auth.") {
		s.OnRequest(req)
	}
}

// EnsureController starts a detached copy of this exact binary. No UI process
// owns its lifetime. Concurrent starts resolve through the socket writer lock.
func EnsureController(ctx context.Context) (Client, error) {
	c := Client{Socket: DefaultSocket()}
	if _, err := c.Call(ctx, Request{Method: "state"}); err == nil {
		return c, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return c, err
	}
	if err = os.MkdirAll(DefaultDir(), 0700); err != nil {
		return c, err
	}
	log, err := os.OpenFile(filepath.Join(DefaultDir(), "controller.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return c, err
	}
	defer log.Close()
	cmd := exec.Command(exe, "review-controller")
	cmd.Stdin = nil
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = cmd.Start(); err != nil {
		return c, err
	}
	go cmd.Wait()
	timer := time.NewTicker(100 * time.Millisecond)
	defer timer.Stop()
	for i := 0; i < 50; i++ {
		select {
		case <-ctx.Done():
			return c, ctx.Err()
		case <-timer.C:
			if _, err = c.Call(ctx, Request{Method: "state"}); err == nil {
				return c, nil
			}
		}
	}
	return c, fmt.Errorf("review controller did not start; see %s", log.Name())
}
