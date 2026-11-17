//go:build linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are real owned-server acceptance tests, never a fake SDK substitute.
// Missing offline fixtures leave acceptance unverified, with an explicit skip.
func TestPickClickPlantedFaults(t *testing.T) {
	config := os.Getenv("AUTARCH_VERIFY_ORACLE_FIXTURES")
	if config == "" {
		t.Skip("INCOMPLETE: owned-server offline fixture configuration not supplied")
	}
	if os.Geteuid() == 0 {
		t.Skip("INCOMPLETE: owned-server acceptance requires a non-root user")
	}
	lock, err := acquireRepoLock(repoLockName())
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	var fixtures struct {
		Cache string `json:"cache"`
		App   string `json:"app"`
		Seed  string `json:"seed"`
	}
	config, e := safePath(config, "")
	if e != nil {
		t.Fatal(e)
	}
	b, e := readOwnedFile(filepath.Dir(config), config)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &fixtures); e != nil {
		t.Fatal(e)
	}
	root, e := git(".", "rev-parse", "--show-toplevel")
	if e != nil {
		t.Fatal(e)
	}
	// Reject unrelated product/setup failures before accepting any planted fault.
	controlWork := ownedTestDir(t)
	controlMirror := filepath.Join(controlWork, "objects.git")
	controlProduct := filepath.Join(controlWork, "product")
	controlOut, e := os.MkdirTemp("/tmp", "autarch-verify-oracle-control-")
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("control evidence retained at %s", controlOut)
	if _, e = git(root, "clone", "--bare", "--no-local", root, controlMirror); e != nil {
		t.Fatal(e)
	}
	if _, e = git(controlMirror, "worktree", "add", "--detach", controlProduct, "HEAD"); e != nil {
		t.Fatal(e)
	}
	controlReport := Report{Artifacts: map[string]string{}, Cache: map[string]any{}}
	control := runHomePick(controlWork, controlProduct, controlOut, fixtures.Cache, fixtures.App, fixtures.Seed, Row{Flow: "home-pick", Label: "new", Seconds: 300}, &controlReport)
	if control.Verdict != "PASS" {
		t.Fatalf("control must PASS before fault acceptance: %s: %s", control.Verdict, control.Cause)
	}
	for _, fault := range []struct{ name, file, old, new, assertion string }{
		{"no-op-click", "ui/asks.tsx", "onClick={onClick} onBlur={onBlur} data-option={o.id}", "onClick={() => undefined} onBlur={onBlur} data-option={o.id}", "persisted browser pick"},
		{"suppressed-pick", "store.ts", "WHERE d.id = @decision_id AND d.revision = @revision", "WHERE 0 AND d.id = @decision_id AND d.revision = @revision", "persisted browser pick"},
		{"wrong-recipient", "service.ts", "recipient: d.thread,", "recipient: (this.store.db.prepare('SELECT thread FROM decisions WHERE thread != ? LIMIT 1').get(d.thread) as {thread: string}).thread,", "owning thread's ruling turn"},
	} {
		t.Run(fault.name, func(t *testing.T) {
			// A standalone scratch Git repository keeps fault commits out of the product branch.
			work := ownedTestDir(t)
			mirror := filepath.Join(work, "objects.git")
			product := filepath.Join(work, "product")
			out, err := os.MkdirTemp("/tmp", "autarch-verify-oracle-evidence-")
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("oracle evidence retained at %s", out)
			if _, e = git(root, "clone", "--bare", "--no-local", root, mirror); e != nil {
				t.Fatal(e)
			}
			if _, e = git(mirror, "worktree", "add", "--detach", product, "HEAD"); e != nil {
				t.Fatal(e)
			}
			name := filepath.Join(product, plugin, fault.file)
			src, e := os.ReadFile(name)
			if e != nil {
				t.Fatal(e)
			}
			if strings.Count(string(src), fault.old) != 1 {
				t.Fatal("fault anchor must match exactly once")
			}
			if e = os.WriteFile(name, []byte(strings.Replace(string(src), fault.old, fault.new, 1)), 0600); e != nil {
				t.Fatal(e)
			}
			if fault.name == "wrong-recipient" {
				// Give the planted routing bug a real second recipient. Only the scratch
				// scenario changes; the browser pick and all delivery assertions stay intact.
				file := filepath.Join(product, plugin, "e2e/real.ts")
				source, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				anchor := `assert.ok(card, "filed decision has no card");`
				if strings.Count(string(source), anchor) != 1 {
					t.Fatal("routing fixture anchor mismatch")
				}
				fixture := `
      const originalAskFile = this.askFile.bind(this);
      this.askFile = (name, question) => {
        const file = originalAskFile(name, question);
        if (question === 'oracle routing target') {
          const ask = JSON.parse(readFileSync(file, 'utf8'));
          ask.options = [{ id: 'day', label: 'Fixture complete', kind: 'ruling-only' }];
          writeFileSync(file, JSON.stringify(ask));
        }
        return file;
      };
      const routingDecision = await this.ask(other, 'oracle routing target');
      assert.equal((await this.pick(routingDecision, 'day', 'oracle-fixture-' + routingDecision)).status, 201);
      this.askFile = originalAskFile;
`
				if err = os.WriteFile(file, []byte(strings.Replace(string(source), anchor, anchor+fixture, 1)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, e = git(product, "add", plugin); e != nil {
				t.Fatal(e)
			}
			if _, e = git(product, "-c", "user.name=verify oracle", "-c", "user.email=verify@example.invalid", "commit", "-m", "test: plant verifier oracle fault"); e != nil {
				t.Fatal(e)
			}
			r := Report{Artifacts: map[string]string{}, Cache: map[string]any{}}
			got := runHomePick(work, product, out, fixtures.Cache, fixtures.App, fixtures.Seed, Row{Flow: "home-pick", Label: "new", Seconds: 300}, &r)
			if got.Verdict != "FAIL" {
				t.Fatalf("fault must FAIL, got %s: %s; evidence: %v", got.Verdict, got.Cause, got.Evidence)
			}
			matched := false
			for _, assertion := range got.Assertions {
				var row struct {
					Error string `json:"error"`
				}
				if json.Unmarshal(assertion, &row) == nil && strings.Contains(row.Error, fault.assertion) {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("fault failed outside its expected assertion %q: %s", fault.assertion, got.Assertions)
			}
		})
	}
}
