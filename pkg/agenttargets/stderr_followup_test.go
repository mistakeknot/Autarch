package agenttargets

import (
	"fmt"
	"strings"
	"testing"
)

func init() {
	stderrStopOrderCheck = func(start, last int) { panic("stderr value starts out of order") }
}

func TestStderrCleanedRemainderNotice(t *testing.T) {
	for _, ending := range []string{"\x1b[?25h", "\x00\x07", " \x1b[0m\t"} {
		input := strings.Repeat("x", 5000) + "\n" + ending
		var tail stderrTail
		tail.Write([]byte(input))
		for _, raw := range [][]byte{[]byte(input), tail.data} {
			want := fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", len(raw))
			if got := sanitizedStderr(raw); got != want {
				t.Errorf("cleaned remainder %q: got %q, want %q", ending, got, want)
			}
		}
	}
}

func TestStderrStopCursorOutOfOrder(t *testing.T) {
	raw := []byte("ok 1111 end")
	region := normalizeStderr(raw, 0)
	cursor := stderrStopCursor{}
	cursor.end(region.text, 8, stderrSpace)
	violations := 0
	end := cursor.endChecked(region.text, 3, stderrSpace, func(start, last int) {
		violations++
		if start != 3 || last != 8 {
			t.Fatalf("order check: %d, %d", start, last)
		}
	})
	if violations != 1 {
		t.Fatal("strict order check was not called")
	}
	if end != len(region.text) {
		t.Fatalf("out-of-order end %d, want region end", end)
	}
}

func TestStderrStopCursorProductionFallback(t *testing.T) {
	raw := []byte("ok 1111 end")
	region := normalizeStderr(raw, 0)
	cursor := stderrStopCursor{last: 8, pos: 10}
	// A nil check is the production path; force the otherwise-unreachable order.
	end := cursor.endChecked(region.text, 3, stderrSpace, nil)
	diff := make([]int32, len(raw)+1)
	// Even a caller that caps its match before end cannot weaken the fallback.
	maskStderrSpan(diff, region, 3, min(7, end))
	maskStderrCursorFallback(diff, region, &cursor)
	for i := 1; i < len(diff); i++ {
		diff[i] += diff[i-1]
	}
	if got := renderStderrMasked(raw, 0, region, diff); got != "ok [REDACTED]" {
		t.Fatalf("safe fallback: %q", got)
	}
}
