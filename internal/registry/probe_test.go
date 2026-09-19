package registry

import (
	"os"
	"testing"
)

func selfPID() int { return os.Getpid() }

func TestParseElapsedReadsBSDFormats(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"02:29:48", 2*3600 + 29*60 + 48, true},
		{"39:20", 39*60 + 20, true},
		{"1-02:03:04", 86400 + 2*3600 + 3*60 + 4, true},
		{"", 0, false},
		{"48", 0, false},
		// What macOS ps actually printed when handed a Linux field name.
		{"keyword", 0, false},
		{"1:2:3:4", 0, false},
	}
	for _, c := range cases {
		got, ok := parseElapsed(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseElapsed(%q) = %d,%v; want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// The failure that closed eleven live agents: `ps -o pid=,etimes=` printed
// "keyword not found", listed bare pids, and exited 0. Every path that is not
// a clean answer must resolve to unknown.
func TestAProbeThatCouldNotRunConcludesNothing(t *testing.T) {
	now := int64(1_000_000_000_000)
	targets := []probeTarget{
		{instanceID: "a", pid: 100, startedMs: now - 60_000, local: true},
		{instanceID: "b", pid: 200, startedMs: now - 60_000, local: true},
	}

	degraded := []struct {
		name   string
		out    string
		errOut string
	}{
		{"the etimes failure: bare pids, no elapsed column", "100\n200\n", "ps: etimes: keyword not found"},
		{"nothing parseable at all", "%cpu %mem acflag\n", ""},
		{"empty output", "", ""},
		{"a complaint on stderr", "100 01:00\n", "ps: process id too large: 4000001"},
	}
	for _, d := range degraded {
		got := interpretProbe(targets, d.out, d.errOut, nil, now)
		for _, id := range []string{"a", "b"} {
			if got[id] != ProbeUnknown {
				t.Errorf("%s: instance %s = %q, want %q -- a probe that could not run must not report a death",
					d.name, id, got[id], ProbeUnknown)
			}
		}
	}
}

func TestProbeReadsAbsenceAsDeathOnlyOnACleanRun(t *testing.T) {
	now := int64(1_000_000_000_000)
	targets := []probeTarget{
		{instanceID: "alive", pid: 100, startedMs: now - 3_600_000, local: true},
		{instanceID: "gone", pid: 200, startedMs: now - 3_600_000, local: true},
		{instanceID: "recycled", pid: 300, startedMs: now - 3_600_000, local: true},
		{instanceID: "remote", pid: 400, startedMs: now - 3_600_000, local: false},
	}
	// 100 has been up an hour, matching its record. 300 has been up ten
	// seconds, so the pid was reused and our process is gone. 200 is absent.
	out := "100 01:00:00\n300 00:10\n"
	got := interpretProbe(targets, out, "", nil, now)

	want := map[string]string{
		"alive":    ProbeAlive,
		"gone":     ProbeDead,
		"recycled": ProbeDead,
		"remote":   ProbeUnknown,
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("instance %s = %q, want %q", id, got[id], w)
		}
	}
}

// A process starts before it writes its record, so an earlier-than-expected
// start is normal. Reading it as a recycled pid would close live agents over
// clock slop -- which is what a symmetric comparison did.
func TestAnEarlierStartThanRecordedIsNotADeath(t *testing.T) {
	now := int64(1_000_000_000_000)
	targets := []probeTarget{{instanceID: "a", pid: 100, startedMs: now - 3_600_000, local: true}}
	// Running for two hours; the record was written one hour ago. Consistent
	// with a long-lived process that rewrote its record.
	got := interpretProbe(targets, "100 02:00:00\n", "", nil, now)
	if got["a"] != ProbeAlive {
		t.Errorf("instance a = %q, want %q", got["a"], ProbeAlive)
	}
}

// The real process table, through the real ps invocation: this process is
// alive, and a pid that cannot exist is not.
func TestProbeAgainstTheRealProcessTable(t *testing.T) {
	self := probeTarget{instanceID: "self", pid: int64(selfPID()), startedMs: 0, local: true}
	got := probeProcesses([]probeTarget{self})
	if got["self"] != ProbeAlive {
		t.Errorf("this process probed as %q, want %q -- the ps invocation is wrong", got["self"], ProbeAlive)
	}
}
