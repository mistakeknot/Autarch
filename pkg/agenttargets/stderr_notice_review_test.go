package agenttargets

import (
	"fmt"
	"strings"
	"testing"
)

func TestStderrTailNoticeNoLength(t *testing.T) {
	for _, size := range []int{1, 8, 128, 4000} {
		input := "sk-" + strings.Repeat("a", size) + "\r"
		for _, chunk := range []int{1, 7, len(input)} {
			if got := recoveryOutput(t, recoveryWrite(input, chunk)); got != "[stderr withheld: no visible final frame]" {
				t.Errorf("credential size %d, chunk %d: got %q", size, chunk, got)
			}
		}
	}
}

func TestStderrTailNoticeWording(t *testing.T) {
	for _, input := range []string{"\n\np\r", "p\r \n"} {
		for _, chunk := range []int{1, 7, len(input)} {
			if got := recoveryOutput(t, recoveryWrite(input, chunk)); got != "[stderr withheld: no visible final frame]" {
				t.Errorf("input %q, chunk %d: got %q", input, chunk, got)
			}
		}
	}
}

func TestStderrTailFormatFrames(t *testing.T) {
	for _, format := range []string{"\u200b", "\u200c\u200d", "\u2060", "\ufeff"} {
		for _, prefix := range []string{"", strings.Repeat("x", 5000), strings.Repeat("x", 5000) + "\n"} {
			input := prefix + "p\r \t" + format
			t.Run(fmt.Sprintf("%d/%q", len(prefix), format), func(t *testing.T) {
				for _, chunk := range []int{1, 7, len(input)} {
					if got := recoveryOutput(t, recoveryWrite(input, chunk)); got != "[stderr withheld: no visible final frame]" {
						t.Errorf("chunk %d: got %q", chunk, got)
					}
				}
			})
		}
	}
	for _, input := range []string{"p\r\u200b", "\u200b\u200b", "\u200b\r\u200b", strings.Repeat("\u200b", 1800), "p\r\u200bvisible\u200b"} {
		want := beforeBlankNoticeStderrTail(recoveryWrite(input, 7))
		if input == "p\r\u200b" {
			want = "[stderr withheld: no visible final frame]"
		}
		if got := recoveryOutput(t, recoveryWrite(input, 7)); got != want {
			t.Errorf("input length %d, prefix %q: got %q, want %q", len(input), input[:min(len(input), 12)], got, want)
		}
	}
}
