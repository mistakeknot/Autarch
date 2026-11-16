package homeask

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestReleaseReflagsDismissedOwnerNotice(t *testing.T) {
	for _, ownerState := range []string{"archived", "missing"} {
		for _, fullCap := range []bool{false, true} {
			name := ownerState + "/below-cap"
			if fullCap {
				name = ownerState + "/full-cap"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv(EnvCardDailyCap, "1")
				bb := newFake(t, registryOnCreate)
				bb.threads = map[string]string{"thr_old": ownerState}
				f := bb.filer()
				f.StateDir = t.TempDir()
				if err := f.SetPilot("daycap", true); err != nil {
					t.Fatal(err)
				}
				r := testReq()
				r.Key, r.Owner, r.NonBlocking = "held", "thr_old", true
				if _, err := f.FileCard(context.Background(), r); err != nil {
					t.Fatal(err)
				}
				if report, err := f.ReleaseDetailed(context.Background()); err == nil || report.OwnerAttention[r.Key] == "" {
					t.Fatalf("initial release did not flag owner: %+v %v", report, err)
				}
				state, _, done, err := f.openState()
				if err != nil {
					t.Fatal(err)
				}
				before := state[r.Key]
				done()
				if err := f.DismissReleaseOwnerAttention(r.Key); err != nil {
					t.Fatal(err)
				}
				var footer bytes.Buffer
				if err := f.PrintReleaseOwnerAttention(&footer); err != nil || footer.Len() != 0 {
					t.Fatalf("dismissal did not clear footer: %q %v", footer.String(), err)
				}
				if fullCap {
					fill := testReq()
					fill.Key = "delivered"
					if result, err := f.FileCard(context.Background(), fill); err != nil || result.Queued {
						t.Fatalf("could not fill daily cap: %+v %v", result, err)
					}
				}
				report, err := f.ReleaseDetailed(context.Background())
				var entries *ReleaseEntryErrors
				if !errors.As(err, &entries) || !entries.DataOnly() || report.OwnerAttention[r.Key] == "" {
					t.Fatalf("release did not restore dismissed owner notice: %+v %v", report, err)
				}
				if err := f.PrintReleaseOwnerAttention(&footer); err != nil || !strings.Contains(footer.String(), r.Key) {
					t.Fatalf("restored notice missing from footer: %q %v", footer.String(), err)
				}
				state, _, done, err = f.openState()
				if err != nil {
					t.Fatal(err)
				}
				defer done()
				if !reflect.DeepEqual(state[r.Key], before) {
					t.Fatalf("reflag changed request, delivery or budget state: got %+v want %+v", state[r.Key], before)
				}
			})
		}
	}
}
