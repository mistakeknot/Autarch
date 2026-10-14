package escalcheck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	cmdTimeout   = 120 * time.Second
	cmdOutputCap = 64 << 10
)

// Result is a finished (or killed) command.
type Result struct {
	Stdout    string
	Stderr    string
	Code      int // -1 when the process did not exit normally
	Truncated bool
	TimedOut  bool
}

// Runner runs one command by argv with an explicit environment.
type Runner interface {
	Run(ctx context.Context, argv []string, env []string) (Result, error)
}

// ExecRunner is the real runner: no shell, closed stdin, its own process group
// killed at the timeout, and bounded capture that keeps draining the pipe.
type ExecRunner struct{}

type capBuf struct {
	buf       bytes.Buffer
	truncated bool
}

func (c *capBuf) Write(p []byte) (int, error) {
	room := cmdOutputCap - c.buf.Len()
	if room > 0 {
		n := len(p)
		if n > room {
			n = room
			c.truncated = true
		}
		c.buf.Write(p[:n])
	} else if len(p) > 0 {
		c.truncated = true
	}
	return len(p), nil // always report full consumption so the child never blocks
}

func (ExecRunner) Run(ctx context.Context, argv []string, env []string) (Result, error) {
	if len(argv) == 0 {
		return Result{Code: -1}, errors.New("empty argv")
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return Result{Code: -1}, fmt.Errorf("%s not found on PATH", argv[0])
	}
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	cmd := exec.Command(path, argv[1:]...)
	cmd.Env = env
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var so, se capBuf
	cmd.Stdout, cmd.Stderr = &so, &se
	if err := cmd.Start(); err != nil {
		return Result{Code: -1}, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var werr error
	timedOut := false
	select {
	case werr = <-done:
	case <-ctx.Done():
		timedOut = true
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		werr = <-done
	}
	res := Result{Stdout: so.buf.String(), Stderr: se.buf.String(), Truncated: so.truncated || se.truncated, TimedOut: timedOut, Code: 0}
	if werr != nil {
		var ee *exec.ExitError
		if errors.As(werr, &ee) && !timedOut {
			res.Code = ee.ExitCode()
		} else {
			res.Code = -1
		}
	}
	return res, nil
}

// readRegular reads at most max bytes of a regular file through one
// descriptor opened without blocking (a FIFO or device is refused). A file
// longer than max is an error, never a short read.
func readRegular(path string, max int64) ([]byte, os.FileInfo, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, st, fmt.Errorf("%s is not a regular file", path)
	}
	if st.Size() > max {
		return nil, st, fmt.Errorf("%s is larger than %d bytes", path, max)
	}
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, st, err
	}
	if int64(len(b)) > max {
		return nil, st, fmt.Errorf("%s grew past %d bytes while read", path, max)
	}
	return b, st, nil
}
