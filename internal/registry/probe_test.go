package registry

import (
	"os"
	"os/exec"
	"testing"
)

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

// Two failures, in opposite directions, both of which shipped.
//
// First: `ps -o pid=,etimes=` printed "keyword not found", listed bare pids
// and exited 0, and a default of dead closed eleven live agents. Second: BSD
// ps exits 1 and prints NOTHING when none of the pids exist, so "every
// tracked agent died" was indistinguishable from "the probe failed" -- and
// after a reboot the estate would have read live forever.
//
// The sentinel settles both: this process is unarguably running, so ps
// listing it proves the run worked, and any other absence is real.
func TestAProbeThatCouldNotRunConcludesNothing(t *testing.T) {
	now := int64(1_000_000_000_000)
	const sentinel = 999
	targets := []probeTarget{
		{instanceID: "a", pid: 100, startedMs: now - 60_000, local: true},
		{instanceID: "b", pid: 200, startedMs: now - 60_000, local: true},
	}

	degraded := []struct {
		name   string
		out    string
		errOut string
	}{
		{"the etimes failure: bare pids, no elapsed column", "100\n200\n999\n", "ps: etimes: keyword not found"},
		{"nothing parseable at all", "%cpu %mem acflag\n", ""},
		{"empty output, which is also what ps gives when every pid is gone", "", ""},
		{"a complaint on stderr", "999 00:01\n100 01:00\n", "ps: process id too large: 4000001"},
	}
	for _, d := range degraded {
		got := interpretProbe(targets, d.out, d.errOut, sentinel, now)
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
	// 999 is the sentinel, proving the run worked.
	out := "999 00:05\n100 01:00:00\n300 00:10\n"
	got := interpretProbe(targets, out, "", 999, now)

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
	got := interpretProbe(targets, "999 00:05\n100 02:00:00\n", "", 999, now)
	if got["a"] != ProbeAlive {
		t.Errorf("instance a = %q, want %q", got["a"], ProbeAlive)
	}
}

// A pid that is listed but whose elapsed time will not parse is running; it
// is simply not judgeable for pid reuse. It is certainly not dead.
func TestAnUnreadableElapsedTimeIsNotADeath(t *testing.T) {
	now := int64(1_000_000_000_000)
	targets := []probeTarget{{instanceID: "a", pid: 100, startedMs: now - 60_000, local: true}}
	got := interpretProbe(targets, "999 00:05\n100 ??\n", "", 999, now)
	if got["a"] != ProbeUnknown {
		t.Errorf("instance a = %q, want %q", got["a"], ProbeUnknown)
	}
}

// The real ps invocation, end to end: this process is alive, and a process
// that has genuinely exited is dead. The fixtures elsewhere inject the probe,
// so without this the real command is never exercised against a real death --
// which is how two ps bugs reached a live estate.
func TestProbeAgainstTheRealProcessTable(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start a process to kill: %v", err)
	}
	deadPid := int64(cmd.Process.Pid)
	_ = cmd.Wait()

	self := probeTarget{instanceID: "self", pid: int64(os.Getpid()), local: true}
	gone := probeTarget{instanceID: "gone", pid: deadPid, local: true}

	got := probeProcesses([]probeTarget{self, gone})
	if got["self"] != ProbeAlive {
		t.Errorf("this process probed as %q, want %q -- the ps invocation is wrong", got["self"], ProbeAlive)
	}
	if got["gone"] != ProbeDead {
		t.Errorf("an exited process probed as %q, want %q", got["gone"], ProbeDead)
	}

	// And the case that produced the mirror bug: every tracked pid dead, so
	// ps exits 1 with no output. The sentinel must still carry the run.
	onlyDead := probeProcesses([]probeTarget{gone})
	if onlyDead["gone"] != ProbeDead {
		t.Errorf("with every tracked pid gone, probe said %q, want %q -- the last agent to exit would never close",
			onlyDead["gone"], ProbeDead)
	}
}
