package agenttargets

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// Explicit staging keeps capture-only commits behaviorally identical.
const recoveryStage = 2
const recoveryNotice = "[stderr withheld: 4096 bytes with no complete line]"

var recoveryEvaluations int

// Per-test limits total 1400, preserving the default suite budget regardless
// of test order or -count. Fuzz and extended evaluations remain separate.
func recoveryEvaluationBudget(t *testing.T, budget int) {
	t.Helper()
	start := recoveryEvaluations
	t.Cleanup(func() {
		evaluations := recoveryEvaluations - start
		t.Logf("recovery evaluations: %d (cap %d; fuzz/extended separate)", evaluations, budget)
		if evaluations > budget {
			t.Fatal("default evaluation budget")
		}
	})
}

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
	recoveryEvaluations++
	err := processError(context.Canceled, false, tail)
	prefix := "context canceled (timed out: false): stderr: "
	if err == nil || !strings.HasPrefix(err.Error(), prefix) {
		t.Fatalf("process error: %v", err)
	}
	return strings.TrimPrefix(err.Error(), prefix)
}

func recoveryAllocationInputs() map[string]string {
	r := strings.Repeat
	inputs := map[string]string{
		"ansi": r("\x1b[m", 2730), "osc": r("\x1b]\x07", 2730),
		"legacy": r("sk-x ", 1638), "alternating": r("sk-x\xff", 1365),
		"bearer": "Bearer " + r("1", 8185), "json": `{"token":"` + r("1", 8180) + `"}`,
		"quoted":    r(`"password":"11" `, 480),
		"mix":       r("password=11 sk-x https://u:11@example.test/ ", 180),
		"lf-legacy": "\n" + r("sk-x ", 1638),
		"invalid":   r("\xff", 8192), "plain": r("x", 8192),
		"unicode-left": r("é", 2500) + "x", "unicode-right": "x" + r("é", 2500),
	}
	distinct := ""
	for k := 1; ; k++ {
		next := "token:" + r("\\", k) + "\" "
		if len(distinct)+len(next) > 8192 {
			break
		}
		distinct += next
	}
	inputs["distinct-quotes"] = distinct
	for k := 0; k < 3; k++ {
		inputs[fmt.Sprintf("cjk-%d", k)] = r("x", k) + r("界", 1700)
	}
	variants := map[string]string{}
	for name, input := range inputs {
		if strings.Contains(input, "\n") {
			continue
		}
		if len(input) == 8192 {
			// Preserve grammar, removing one payload byte rather than the JSON close.
			if name == "json" {
				input = `{"token":"` + r("1", 8179) + `"}`
			} else {
				input = input[:len(input)-1]
			}
		}
		variants[name+"-lf"] = input + "\n"
	}
	for name, input := range variants {
		inputs[name] = input
	}
	for _, name := range []string{"ansi", "osc"} {
		inputs[name+"-crlf"] = inputs[name] + "\r\n"
	}
	for _, c := range recoveryCases() {
		if strings.HasPrefix(c.name, "error-") || c.name == "json-token" || c.name == "json-error" {
			inputs[c.name] = c.input
		}
	}
	return inputs
}

func TestRecoveryAllocBudget(t *testing.T) {
	if raceEnabled {
		t.Skip("race instrumentation and randomized sync.Pool eviction distort regexp allocations; run the 1 MiB gate without -race")
	}
	fixedErr := context.Canceled
	for name, input := range recoveryAllocationInputs() {
		raw := []byte(input)
		for _, chunk := range []int{len(raw), 1024} {
			run := func() {
				var tail stderrTail
				for p := 0; p < len(raw); p += chunk {
					tail.Write(raw[p:min(p+chunk, len(raw))])
				}
				processError(fixedErr, false, &tail)
			}
			// Warm regexp pools before measuring a fixed number of recoveries.
			run()
			const iterations = 5
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for i := 0; i < iterations; i++ {
				run()
			}
			runtime.ReadMemStats(&after)
			bytesPerOp := (after.TotalAlloc - before.TotalAlloc) / iterations
			allocsPerOp := (after.Mallocs - before.Mallocs) / iterations
			t.Logf("%s/chunk-%d: %d B/op %d allocs/op", name, chunk, bytesPerOp, allocsPerOp)
			if bytesPerOp > 1<<20 {
				t.Fatalf("allocation budget %s/chunk-%d: %d B/op", name, chunk, bytesPerOp)
			}
		}
	}
}

func recoveryWrite(input string, chunk int) *stderrTail {
	tail := new(stderrTail)
	raw := []byte(input)
	tail.Write(nil)
	for pos := 0; pos < len(input); pos += chunk {
		tail.Write(raw[pos:min(pos+chunk, len(input))])
		tail.Write(nil)
	}
	return tail
}

func TestRecoveryExact(t *testing.T) {
	recoveryEvaluationBudget(t, 300)
	for _, c := range recoveryCases() {
		for _, chunk := range []int{1, 7, 1024, 4095, 4096, len(c.input)} {
			t.Run(fmt.Sprintf("%s/%d", c.name, chunk), func(t *testing.T) {
				tail := recoveryWrite(c.input, chunk)
				if tail.historyLost != (len(c.input) > 8192) || tail.seen != min(len(c.input), 8193) {
					t.Fatal("chunk-dependent capture/loss")
				}
				if len(c.input) <= 4096 && tail.history != nil {
					t.Fatal("eager history")
				}
				if len(c.input) > 4096 && len(c.input) <= 8192 {
					if cap(tail.history) != 8192 || string(tail.history) != c.input {
						t.Fatal("inexact captured history")
					}
				}
				got := recoveryOutput(t, tail)
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
	recoveryEvaluationBudget(t, 10)
	tail := &stderrTail{started: true, seen: 5000, history: []byte(strings.Repeat("x", 5000)), data: []byte(strings.Repeat("y", 4096))}
	if _, ok := tail.completeHistory(); ok {
		t.Fatal("inconsistent history certified")
	}
	if got := recoveryOutput(t, tail); got != recoveryNotice {
		t.Fatal(got)
	}
}

func TestRecoveryPrepopulatedWrite(t *testing.T) {
	recoveryEvaluationBudget(t, 50)
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
	recoveryEvaluationBudget(t, 40)
	if recoveryStage != 2 {
		t.Fatal("final stage must enable every recovery row")
	}
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
