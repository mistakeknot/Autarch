package agenttargets

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func TestStderrTailBlankFrameNotice(t *testing.T) {
	for _, prefix := range []string{"", strings.Repeat("x", 5000) + "\n"} {
		for _, ending := range []string{"p\r", "p\r   ", "p\r\t\u2003", "p\r\r\r", "p\r\x1b]0;title\x07", "p\r \x1b]0;x\ry\x1b\\\t"} {
			input := prefix + ending
			t.Run(fmt.Sprintf("%d/%q", len(prefix), ending), func(t *testing.T) {
				for _, chunk := range []int{1, 7, len(input)} {
					tail := recoveryWrite(input, chunk)
					want := fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", len(tail.data))
					if got := recoveryOutput(t, tail); got != want {
						t.Errorf("chunk %d: got %q, want %q", chunk, got, want)
					}
				}
			})
		}
	}
}

func TestStderrTailBlankBoundaries(t *testing.T) {
	for _, row := range []struct{ input, want string }{
		{"\n\n", "\n\n"}, {" \t\u2003", " \t\u2003"},
		{"\x1b]0;p\r\x07 \t", " \t"},
		{"p\x1b]0;x\ry\x07", "p"},
		{"p\r\n", "p\n"}, {"old\rp\r\n", "p\n"},
		{strings.Repeat("x", 5000) + "\np\r\n", "p\n"},
		{strings.Repeat("x", 5000) + "\rp\r\n", "p\n"},
		{strings.Repeat("x", 5000) + "\r \t", recoveryNotice},
		{strings.Repeat("x", 5000) + "\r\x1b]0;x\ry\x07", recoveryNotice},
	} {
		t.Run(fmt.Sprintf("%d/%q", len(row.input), row.want), func(t *testing.T) {
			for _, chunk := range []int{1, 7, len(row.input)} {
				if got := recoveryOutput(t, recoveryWrite(row.input, chunk)); got != row.want {
					t.Fatalf("chunk %d: got %q, want %q", chunk, got, row.want)
				}
			}
		})
	}
}

// Copy of the tail renderer at the parent commit. The pure renderer and
// recovery implementation stay unchanged, so this isolates tail policy changes.
func beforeBlankNoticeStderrTail(t *stderrTail) string {
	if history, ok := t.completeHistory(); ok {
		if text, accepted := recoverStderrComplete(history); accepted {
			return text
		}
	}
	return sanitizedStderr(t.data)
}

func TestStderrTailNonblankUnchanged(t *testing.T) {
	rng := rand.New(rand.NewSource(105))
	parts := []string{"p", "\r", "\r\n", "\n", "\t", "\u2003", "\x1b]0;x\ry\x07", "\x1b[m", "token=1111 ", "Bearer 1111", "\xff", "é", "useful"}
	nonblank := 0
	for i := 0; i < 300; i++ {
		var input strings.Builder
		input.WriteString(strings.Repeat("x", []int{0, 4090, 5000, 8200}[i%4]))
		for j := 0; j < 1+rng.Intn(20); j++ {
			input.WriteString(parts[rng.Intn(len(parts))])
		}
		tail := recoveryWrite(input.String(), 7)
		old := beforeBlankNoticeStderrTail(tail)
		got := sanitizedStderrTail(tail)
		if strings.TrimSpace(old) != "" {
			nonblank++
			if got != old {
				t.Fatalf("nonblank render changed at case %d: %q != %q", i, got, old)
			}
		} else if got != old && got != fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", len(tail.data)) {
			t.Fatalf("blank render exposed content at case %d: %q", i, got)
		}
	}
	if nonblank < 100 {
		t.Fatalf("insufficient nonblank coverage: %d", nonblank)
	}
}
