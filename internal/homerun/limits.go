//go:build unix

package homerun

import (
	"errors"
	"fmt"
	"time"
)

// Limits is the policy the verifier enforces. Every value is owned by the verifier's configuration,
// never by a card or an envelope: the envelope can only ask for a runtime at or below the cap here.
type Limits struct {
	// ApprovalTTL is the longest time between an envelope's issued_at and expires_at.
	ApprovalTTL time.Duration
	// CheckFreshness is the oldest a check attempt may be when an apply or recover cites it.
	CheckFreshness time.Duration
	// CheckRuntimeMax and ApplyRuntimeMax cap the signed runtime_max_s per operation. Recover uses the
	// apply cap.
	CheckRuntimeMax time.Duration
	ApplyRuntimeMax time.Duration
	// ApplyInterval is the minimum gap between two applies on one card.
	ApplyInterval time.Duration
	// MaxExecutionsPerDay caps authorised executions in a rolling day.
	MaxExecutionsPerDay int
	// SigFailureThreshold bad signatures within SigFailureWindow lock the verifier until it is unlocked
	// by hand.
	SigFailureThreshold int
	SigFailureWindow    time.Duration
	// MaxActiveAsMk and MaxActiveRoot cap attempts that hold capacity: authorised, started, or UNCERTAIN (an unknown outcome may still be
	// running) and not yet ended, aborted before launch, or resolved by an operator.
	MaxActiveAsMk int
	MaxActiveRoot int
	// MaxScriptBytes caps the script (and each dependency) the CAS will take.
	MaxScriptBytes int64
	// RootEnabled gates privilege "root". It is false by default: the root path is built but off, and an
	// envelope asking for it is refused at validation.
	RootEnabled bool
	// Namespace is the SSH-signature namespace an approval must carry (NamespaceFor).
	Namespace string
}

// DefaultLimits returns the values the design records. Root stays disabled.
func DefaultLimits() Limits {
	return Limits{
		ApprovalTTL:         5 * time.Minute,
		CheckFreshness:      30 * time.Minute,
		CheckRuntimeMax:     10 * time.Minute,
		ApplyRuntimeMax:     3 * time.Hour,
		ApplyInterval:       10 * time.Minute,
		MaxExecutionsPerDay: 10,
		SigFailureThreshold: 3,
		SigFailureWindow:    10 * time.Minute,
		MaxActiveAsMk:       2,
		MaxActiveRoot:       1,
		MaxScriptBytes:      256 << 10,
		RootEnabled:         false,
		Namespace:           DefaultNamespace,
	}
}

// Validate reports a configuration that cannot be enforced (a zero or negative limit).
func (l Limits) Validate() error {
	var errs []error
	pos := func(name string, ok bool) {
		if !ok {
			errs = append(errs, fmt.Errorf("limit %s must be positive", name))
		}
	}
	pos("ApprovalTTL", l.ApprovalTTL > 0)
	pos("CheckFreshness", l.CheckFreshness > 0)
	pos("CheckRuntimeMax", l.CheckRuntimeMax >= time.Second)
	pos("ApplyRuntimeMax", l.ApplyRuntimeMax >= time.Second)
	pos("ApplyInterval", l.ApplyInterval > 0)
	pos("MaxExecutionsPerDay", l.MaxExecutionsPerDay > 0)
	pos("SigFailureThreshold", l.SigFailureThreshold > 0)
	pos("SigFailureWindow", l.SigFailureWindow > 0)
	pos("MaxActiveAsMk", l.MaxActiveAsMk > 0)
	pos("MaxActiveRoot", l.MaxActiveRoot >= 0)
	pos("MaxScriptBytes", l.MaxScriptBytes > 0)
	if !reNamespace.MatchString(l.Namespace) {
		errs = append(errs, fmt.Errorf("limit Namespace %q must look like %s<label>", l.Namespace, NamespacePrefix))
	}
	return errors.Join(errs...)
}

// runtimeCap is the largest runtime_max_s the policy allows for op.
func (l Limits) runtimeCap(op string) int64 {
	if op == OpCheck {
		return int64(l.CheckRuntimeMax / time.Second)
	}
	return int64(l.ApplyRuntimeMax / time.Second)
}
