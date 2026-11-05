//go:build unix

package homerun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
)

// Kind is one step in an attempt's audit sequence. The legal sequences are:
//
//	AUTHORIZED, STARTED, ENDED
//	AUTHORIZED, STARTED, UNCERTAIN
//	AUTHORIZED, ABORTED_BEFORE_START
//
// An attempt is "active" from AUTHORIZED until it reaches ENDED, UNCERTAIN or ABORTED_BEFORE_START.
type Kind string

const (
	KindAuthorized Kind = "AUTHORIZED"
	KindStarted    Kind = "STARTED"
	KindEnded      Kind = "ENDED"
	KindUncertain  Kind = "UNCERTAIN"
	KindAborted    Kind = "ABORTED_BEFORE_START"
)

// Results an ENDED record may carry.
const (
	ResultSuccess = "success" // exit status 0
	ResultFailed  = "failed"  // non-zero exit status
	ResultTimeout = "timeout" // killed at runtime_max_s
	ResultSignal  = "signal"  // killed by another signal
)

// Record is one audit entry. Every record carries the identity of the attempt (its ID, the envelope digest,
// the signer, the nonce) so that a line read alone says what it is about; the remaining fields belong to
// the Kind.
type Record struct {
	Seq            int64  `json:"seq"`
	Attempt        string `json:"attempt"`
	Kind           Kind   `json:"kind"`
	At             string `json:"at"`
	EnvelopeDigest string `json:"envelope_digest"`
	Signer         Signer `json:"signer"`
	Nonce          string `json:"nonce"`
	Op             string `json:"op"`
	Privilege      string `json:"privilege"`
	CardID         string `json:"card_id"`

	// STARTED
	Unit         string `json:"unit,omitempty"`
	InvocationID string `json:"invocation_id,omitempty"`
	CASSHA       string `json:"cas_sha256,omitempty"`
	// ENDED
	Result       string `json:"result,omitempty"`
	ExitStatus   *int   `json:"exit_status,omitempty"`
	OutputSHA256 string `json:"output_sha256,omitempty"`
	OutputBytes  *int64 `json:"output_bytes,omitempty"`
	DurationMS   *int64 `json:"duration_ms,omitempty"`
	// UNCERTAIN, ABORTED_BEFORE_START
	Note string `json:"note,omitempty"`
}

// StartInfo is what the launcher reports once the unit is running.
type StartInfo struct {
	Unit         string
	InvocationID string // systemd InvocationID (32 hex characters)
	CASSHA       string // sha256 of the CAS bytes the unit runs; must equal the envelope's script_sha256
}

// EndInfo is what is known when the unit has ended.
type EndInfo struct {
	Result       string
	ExitStatus   int
	OutputSHA256 string
	OutputBytes  int64
	Duration     time.Duration
}

var legalPrev = map[Kind][]Kind{
	KindStarted:   {KindAuthorized},
	KindEnded:     {KindStarted},
	KindUncertain: {KindStarted},
	KindAborted:   {KindAuthorized},
}

func isActive(k Kind) bool { return k == KindAuthorized || k == KindStarted }

func (i StartInfo) validate(scriptSHA string) error {
	if i.Unit == "" || len(i.Unit) > 256 {
		return errors.New("STARTED needs the unit name")
	}
	if !reHex32.MatchString(i.InvocationID) {
		return errors.New("STARTED needs the 32-hex systemd InvocationID")
	}
	if !reHex64.MatchString(i.CASSHA) || i.CASSHA != scriptSHA {
		return errors.New("STARTED CAS sha must equal the envelope script_sha256")
	}
	return nil
}

func (i EndInfo) validate() error {
	switch i.Result {
	case ResultSuccess:
		if i.ExitStatus != 0 {
			return errors.New("ENDED success needs exit status 0")
		}
	case ResultFailed:
		if i.ExitStatus == 0 {
			return errors.New("ENDED failed needs a non-zero exit status")
		}
	case ResultTimeout, ResultSignal:
	default:
		return fmt.Errorf("ENDED has unknown result %q", i.Result)
	}
	if !reHex64.MatchString(i.OutputSHA256) || i.OutputBytes < 0 || i.Duration < 0 {
		return errors.New("ENDED needs the output digest, output size and duration")
	}
	return nil
}

// mirror is the append-only JSON-lines copy of the audit table. The database is the source of truth; the
// mirror is brought up to date after every commit and can always be caught up from the database.
type mirror struct {
	path string
	mu   sync.Mutex
}

// ErrMirror reports that the state change was committed to the database but the append-only mirror could
// not be brought up to date. The caller must treat the operation as failed (fail closed); the next
// successful write catches the mirror up.
var ErrMirror = errors.New("audit mirror not updated")

const mirrorTail = 1 << 20

func (m *mirror) sync(fetch func(after int64) ([]Record, error)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, err := os.OpenFile(m.path, os.O_RDWR|os.O_CREATE|os.O_APPEND|syscall.O_NOFOLLOW, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	last, torn, err := lastMirroredSeq(f)
	if err != nil {
		return err
	}
	recs, err := fetch(last)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if torn {
		// A crash left a partial line, which was never a complete record: cut it off so the next record
		// starts clean. Only the unterminated tail is removed.
		if err := truncateTornTail(f); err != nil {
			return err
		}
	}
	for _, r := range recs {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if buf.Len() == 0 {
		return nil
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		return err
	}
	return f.Sync()
}

func truncateTornTail(f *os.File) error {
	st, err := f.Stat()
	if err != nil {
		return err
	}
	n := st.Size()
	if n > mirrorTail {
		n = mirrorTail
	}
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, st.Size()-n); err != nil {
		return err
	}
	i := bytes.LastIndexByte(buf, '\n')
	if i < 0 {
		if st.Size() > mirrorTail {
			return errors.New("audit mirror tail has no line break")
		}
		return f.Truncate(0)
	}
	return f.Truncate(st.Size() - n + int64(i) + 1)
}

// lastMirroredSeq returns the highest seq in the file's last parseable line and whether the file ends in
// a partial line.
func lastMirroredSeq(f *os.File) (seq int64, torn bool, err error) {
	st, err := f.Stat()
	if err != nil {
		return 0, false, err
	}
	if st.Size() == 0 {
		return 0, false, nil
	}
	n := st.Size()
	if n > mirrorTail {
		n = mirrorTail
	}
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, st.Size()-n); err != nil {
		return 0, false, err
	}
	torn = buf[len(buf)-1] != '\n'
	lines := bytes.Split(buf, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var r Record
		if json.Unmarshal(lines[i], &r) == nil && r.Seq > 0 {
			return r.Seq, torn, nil
		}
	}
	if st.Size() > mirrorTail {
		return 0, false, errors.New("audit mirror tail holds no readable record")
	}
	return 0, torn, nil
}

// ReadMirror parses a mirror file. A line that does not parse is reported as an error, never skipped.
func ReadMirror(path string) ([]Record, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Record
	for i, line := range bytes.Split(b, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("audit mirror line %d: %w", i+1, err)
		}
		out = append(out, r)
	}
	return out, nil
}
