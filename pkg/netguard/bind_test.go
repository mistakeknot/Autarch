package netguard

import "testing"

func TestEnsureLocalOnly_AllowsLoopback(t *testing.T) {
	t.Parallel()

	allowed := []string{
		"127.0.0.1:8080",
		"localhost:8080",
		"[::1]:8080",
	}

	for _, addr := range allowed {
		if err := EnsureLocalOnly(addr); err != nil {
			t.Fatalf("expected %q to be allowed, got error: %v", addr, err)
		}
	}
}

func TestEnsureLocalOnly_RejectsNonLoopback(t *testing.T) {
	t.Parallel()

	rejected := []string{
		"0.0.0.0:8080",
		"192.168.1.10:8080",
		"8.8.8.8:8080",
	}

	for _, addr := range rejected {
		if err := EnsureLocalOnly(addr); err == nil {
			t.Fatalf("expected %q to be rejected", addr)
		}
	}
}
