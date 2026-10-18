package backlogrun

import (
	"fmt"
	"strings"
	"time"
)

// coordMsg collects what one coordinator is told in one run.
type coordMsg struct {
	board     string
	ready     []Verdict
	needsSpec []Verdict
	closeIt   []Verdict
	ruled     []Verdict // mk-gated, Home card already ruled or closed: carry out the ruling
	noHome    []Verdict // mk-gated, no Home card: file one, do it yourself, or close
}

func (m *coordMsg) empty() bool {
	return len(m.ready)+len(m.needsSpec)+len(m.closeIt)+len(m.ruled)+len(m.noHome) == 0
}

func (m *coordMsg) all() []Verdict {
	var out []Verdict
	for _, l := range [][]Verdict{m.ready, m.needsSpec, m.closeIt, m.ruled, m.noHome} {
		out = append(out, l...)
	}
	return out
}

func sortV(vs []Verdict) {
	for i := 1; i < len(vs); i++ {
		for j := i; j > 0 && vs[j].Key < vs[j-1].Key; j-- {
			vs[j], vs[j-1] = vs[j-1], vs[j]
		}
	}
}

func line(v Verdict, extra string) string {
	s := "- " + v.Key + ": " + oneLine(v.Reason)
	if extra != "" {
		s += " " + extra
	}
	return s
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// text is the message to the coordinator of a board.
func (m *coordMsg) text(mode Mode, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From the backlog run (%s, %s), board %s. Automated triage; check a card before acting on its label.\n", mode, now.Format("2006-01-02"), m.board)
	if len(m.ready) > 0 {
		sortV(m.ready)
		b.WriteString("\nREADY (newly ready; start at most the room you have, process as usual):\n")
		for _, v := range m.ready {
			x := ""
			if v.Size != "" || v.Seat != "" {
				x = "[" + strings.TrimSpace(v.Size+" "+v.Seat) + "]"
			}
			b.WriteString(line(v, x) + "\n")
		}
	}
	if len(m.needsSpec) > 0 {
		sortV(m.needsSpec)
		b.WriteString("\nNEEDS-SPEC (one cheap pass each: (a) put a done-condition and size on the card, (b) close it if stale, a duplicate (`duplicate of KEY`) or done, (c) mark it mk-gated naming the exact decision):\n")
		for _, v := range m.needsSpec {
			b.WriteString(line(v, "") + "\n")
		}
	}
	if len(m.closeIt) > 0 {
		sortV(m.closeIt)
		b.WriteString("\nLOOKS CLOSABLE (verify, then close with the reason, or `duplicate of KEY`):\n")
		for _, v := range m.closeIt {
			x := ""
			if v.DuplicateOf != "" {
				x = "(duplicate of " + v.DuplicateOf + "?)"
			}
			b.WriteString(line(v, x) + "\n")
		}
	}
	if len(m.ruled) > 0 {
		sortV(m.ruled)
		b.WriteString("\nALREADY RULED on Home, board card still open (carry out the ruling, then close or advance the card):\n")
		for _, v := range m.ruled {
			b.WriteString(line(v, "") + "\n")
		}
	}
	if len(m.noHome) > 0 {
		sortV(m.noHome)
		b.WriteString("\nNO HOME CARD (mk-gated but mk has nothing to answer: file one with `autarch needs-mk file`, do it yourself if an agent may, or close it if stale):\n")
		for _, v := range m.noHome {
			x := ""
			if v.Decision != "" {
				x = "Decision: " + oneLine(v.Decision)
			}
			b.WriteString(line(v, x) + "\n")
		}
	}
	return b.String()
}

func renderDigest(o Options, now time.Time, rec Receipt, msgs map[string]*coordMsg, boards, unrouted, skipped, homeNote []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Backlog run %s (%s%s)\n\n", now.Format(time.RFC3339), o.Mode, map[bool]string{true: ", dry run", false: ""}[o.DryRun])
	fmt.Fprintf(&b, "Cards: %d. Triaged this run: %d. %s\n", rec.Counts["cards"], rec.Counts["triaged"], countLine(rec.Counts))
	if rec.Seat != nil {
		fmt.Fprintf(&b, "Triage seat: role %s, profile %s, %s %s %s, policy %s\n", rec.Seat.Role, rec.Seat.ProfileRef, rec.Seat.Backend, rec.Seat.Model, rec.Seat.Effort, rec.Seat.PolicyHash)
	}
	for _, bd := range boards {
		fmt.Fprintf(&b, "\n## %s\n\n%s", bd, msgs[bd].text(o.Mode, now))
	}
	if len(homeNote) > 0 {
		fmt.Fprintf(&b, "\n## Waiting on mk (Home card open)\n\n%s\n", strings.Join(homeNote, ", "))
	}
	if len(skipped) > 0 {
		b.WriteString("\n## Ready cards not dispatched\n\n")
		for _, s := range skipped {
			b.WriteString("- " + s + "\n")
		}
	}
	if len(unrouted) > 0 {
		fmt.Fprintf(&b, "\n## No single coordinator thread\n\n%s\n", strings.Join(unrouted, ", "))
	}
	return b.String()
}
