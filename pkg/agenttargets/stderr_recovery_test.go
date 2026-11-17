package agenttargets

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// Explicit staging keeps capture-only commits behaviorally identical.
const recoveryStage = 1
const recoveryNotice = "[stderr withheld: 4096 bytes with no complete line]"

type recoveryCase struct {
	name, input, baselineWant, finalWant string
	greenFrom                            int
}

func recoveryCases() []recoveryCase {
	r := strings.Repeat
	var rows []recoveryCase
	add := func(name, input, want string) {
		baseline, stage := recoveryNotice, 2
		if want == recoveryNotice {
			stage = 0
		}
		rows = append(rows, recoveryCase{name, input, baseline, want, stage})
	}
	add("plain", r("x", 5000), r("x", 4096))
	for _, end := range []string{"", "\n", "\r\n"} {
		want := "Error: quota exceeded"
		if end != "" {
			want += "\n"
		}
		add(fmt.Sprintf("error-%q", end), r("x", 5000)+"\rError: quota exceeded"+end, want)
	}
	add("json-error", `{"error":"`+r("x", 5000)+"\"}\n", r("x", 4093)+"\"}\n")
	add("json-token", `{"token":"`+r("1", 5000)+"\",\"error\":\"quota exceeded\"}\n", "{[REDACTED],\"error\":\"quota exceeded\"}\n")
	add("bearer-final", "Bearer "+r("1", 5000)+"\n", "[REDACTED]\n")
	add("ansi-bearer", "Bea\x1b[mrer "+r("1", 1100)+"\r"+r("1", 5000), "[REDACTED]")
	whitespace := `"token":` + r(" ", 1100) + "\n\"" + r("1", 5000) + `"`
	third := "\"token\":\"a\nBearer z\"" + r("1", 5000)
	for _, c := range []struct{ name, input string }{{"json-whitespace", whitespace}, {"third-region", third}} {
		add(c.name, c.input, recoveryNotice)
		add(c.name+"-final", c.input+"\n", recoveryNotice)
	}
	rows = append(rows, recoveryCase{"third-short", "\"token\":\"a\nBearer z\"" + r("1", 4080), "[REDACTED]", "[REDACTED]", 0})
	for _, c := range []struct{ name, input, want string }{
		{"password", "password=" + r("1", 5000), "password=[REDACTED]"},
		{"token", "token=" + r("1", 5000), "token=[REDACTED]"},
		{"flag", "--password " + r("1", 5000), "--password [REDACTED]"},
		{"url", "https://u:" + r("1", 5000) + "@example.test/", "https://u:[REDACTED]@example.test/"},
		{"json-password", `{"password":"` + r("1", 5000) + `"}`, `{"password":[REDACTED]}`},
		{"window-new", r("x", 96) + "--token 1111 " + r("x", 4083), "n [REDACTED] " + r("x", 4083)},
		{"window-legacy", r("x", 1000) + "sk-1111 " + r("x", 4088), "DACTED] " + r("x", 4088)},
		{"unicode-left", r("é", 2500) + "x", r("é", 2047) + "x"},
		{"unicode-right", "x" + r("é", 2500), r("é", 2048)},
		{"invalid-frame", r("x", 5000) + "\r\xffok", "�ok"},
		{"empty-frame", r("x", 5000) + "\r", recoveryNotice},
		{"blank-frame", r("x", 5000) + "\r \t", recoveryNotice},
		{"plain-final", r("x", 5000) + "\n", r("x", 4095) + "\n"},
		{"interior-blank", r("x", 5000) + "\n \t", recoveryNotice},
		{"cap-low", r("x", 4096), r("x", 4096)},
		{"cap", r("x", 8192), r("x", 4096)},
		{"overflow", r("x", 8193), recoveryNotice},
		{"overflow-error", r("x", 8193) + "\rError: quota exceeded", recoveryNotice},
		{"expansion", r("\xff", 8192), r("�", 1365)},
		{"unterminated-final", `{"token":"` + r("1", 5000) + "\n", "{[REDACTED]"},
	} {
		add(c.name, c.input, c.want)
	}
	for k := 0; k < 3; k++ {
		add(fmt.Sprintf("cjk-%d", k), r("x", k)+r("界", 1700), r("界", 1365))
	}
	for _, c := range []struct{ input, want string }{
		{"password=1111", "password=[REDACTED]"}, {"token=1111", "token=[REDACTED]"},
		{"--password 1111", "--password [REDACTED]"},
		{"https://u:1111@example.test/", "https://u:[REDACTED]@example.test/"},
		{`{"password": "1111"}`, `{"password": [REDACTED]}`},
	} {
		add("short-"+c.input, r("x", 5000)+" \r"+c.input, c.want)
	}
	rows = append(rows, recoveryCase{"overflow-line", r("x", 8193) + "\nuseful", "useful", "useful", 0})
	return rows
}

func recoveryOutput(t testing.TB, tail *stderrTail) string {
	t.Helper()
	err := processError(context.Canceled, false, tail)
	prefix := "context canceled (timed out: false): stderr: "
	if err == nil || !strings.HasPrefix(err.Error(), prefix) {
		t.Fatalf("process error: %v", err)
	}
	return strings.TrimPrefix(err.Error(), prefix)
}

func recoveryWrite(input string, chunk int) *stderrTail {
	tail := new(stderrTail)
	tail.Write(nil)
	for pos := 0; pos < len(input); pos += chunk {
		tail.Write([]byte(input[pos:min(pos+chunk, len(input))]))
		tail.Write(nil)
	}
	return tail
}

func TestRecoveryExact(t *testing.T) {
	for _, c := range recoveryCases() {
		for _, chunk := range []int{1, 7, 1024, 4095, 4096, len(c.input)} {
			t.Run(fmt.Sprintf("%s/%d", c.name, chunk), func(t *testing.T) {
				got := recoveryOutput(t, recoveryWrite(c.input, chunk))
				want := c.baselineWant
				if recoveryStage >= c.greenFrom {
					want = c.finalWant
				}
				if got != want {
					t.Fatalf("stage %d got %q want %q", recoveryStage, got, want)
				}
				if recoveryStage < c.greenFrom && c.baselineWant != c.finalWant && got == c.finalWant {
					t.Fatal("premature recovery")
				}
			})
		}
	}
	if got := sanitizedStderr([]byte(strings.Repeat("x", 5000))); got != "[stderr withheld: 5000 bytes with no complete line]" {
		t.Fatal(got)
	}
}

func TestRecoveryInconsistentWindow(t *testing.T) {
	tail := &stderrTail{started: true, seen: 5000, history: []byte(strings.Repeat("x", 5000)), data: []byte(strings.Repeat("y", 4096))}
	if _, ok := tail.completeHistory(); ok {
		t.Fatal("inconsistent history certified")
	}
	if got := recoveryOutput(t, tail); got != recoveryNotice {
		t.Fatal(got)
	}
}

func TestRecoveryPrepopulatedWrite(t *testing.T) {
	for _, n := range []int{4096, 5000} {
		for _, chunk := range []int{n, 7} {
			t.Run(fmt.Sprintf("%d/%d", n, chunk), func(t *testing.T) {
				tail := &stderrTail{data: []byte("old")}
				input := strings.Repeat("x", n)
				for p := 0; p < n; p += chunk {
					tail.Write([]byte(input[p:min(p+chunk, n)]))
				}
				if !tail.historyLost {
					t.Fatal("initial loss not sticky")
				}
				if got := recoveryOutput(t, tail); got != recoveryNotice {
					t.Fatal("prepopulated recovery", got)
				}
			})
		}
	}
}

func TestRecoveryLazyCapture(t *testing.T) {
	tail := new(stderrTail)
	tail.Write(nil)
	if tail.started {
		t.Fatal("zero write started capture")
	}
	tail.Write([]byte(strings.Repeat("x", 4095)))
	if tail.history != nil || tail.seen != 4095 {
		t.Fatal("eager history")
	}
	tail.Write([]byte("y"))
	if tail.history != nil || tail.seen != 4096 {
		t.Fatal("history at exact window")
	}
	tail.Write([]byte("z"))
	if cap(tail.history) != 8192 || string(tail.history) != strings.Repeat("x", 4095)+"yz" {
		t.Fatal("threshold seed")
	}
	tail.Write([]byte(strings.Repeat("q", 4095)))
	if len(tail.history) != 8192 || tail.historyLost {
		t.Fatal("exact cap")
	}
	tail.Write([]byte("!"))
	if !tail.historyLost || tail.history != nil || tail.seen != 8193 {
		t.Fatal("overflow")
	}
	for _, end := range []string{"\r", "\n", " ", "\t", "\ruseful", strings.Repeat("x", 4096)} {
		tail.Write([]byte(end))
		if !tail.historyLost || tail.history != nil {
			t.Fatal("lost state reentered")
		}
	}
	for _, chunk := range []int{1, 7, 4096, 8193} {
		other := recoveryWrite(strings.Repeat("x", 8193), chunk)
		if !other.historyLost || other.history != nil {
			t.Fatal("chunk overflow")
		}
	}
	short := recoveryWrite("short", 1)
	if short.history != nil || processError(nil, false, short) != nil {
		t.Fatal("short success")
	}
}

func TestRecoveryCallerOwnership(t *testing.T) {
	input := strings.Repeat("x", 5000) + "\rError: quota exceeded\r\n"
	for _, chunk := range []int{1, 7, 1024, 4095, 4096, len(input)} {
		tail := new(stderrTail)
		buffer := make([]byte, chunk)
		for p := 0; p < len(input); p += chunk {
			n := copy(buffer, input[p:min(p+chunk, len(input))])
			tail.Write(buffer[:n])
			for i := range buffer {
				buffer[i] = 'q'
			}
		}
		h, ok := tail.completeHistory()
		if !ok || string(h) != input {
			t.Fatal("writer buffer retained")
		}
		want := recoveryNotice
		if recoveryStage == 2 {
			want = "Error: quota exceeded\n"
		}
		if got := recoveryOutput(t, tail); got != want {
			t.Fatal(got)
		}
	}
}
