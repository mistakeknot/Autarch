package registry

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Identifiers here are deterministic functions of natural keys carried in the
// event log, never random. That is what lets every projection be dropped and
// rebuilt while the durable tables -- items, decisions, operator overrides --
// keep pointing at the right conversation. A random id would survive the
// rebuild as an orphan.

// idPart escapes the separator so a component containing ':' cannot forge a
// different identity. Total and reversible, rather than a documented promise
// that inputs are well behaved.
func idPart(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "%", "%25"), ":", "%3A")
}

// ConversationID is the stable id for a provider conversation on a host.
//
// host is part of it because the estate spans two machines and the same
// provider session id observed on each is two records, not one merged history.
func ConversationID(provider, host, providerSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", idPart(provider), idPart(host), idPart(providerSessionID))
}

// InstanceID is the stable id for one process launch.
//
// pidDomain is in the key because a pid means nothing without the namespace
// that issued it, and startedMs is the provider's own millisecond epoch -- not
// a parse of its human-readable start string, which carries one-second
// resolution, no timezone, and disagrees with the epoch field by over a second.
func InstanceID(host, pidDomain string, pid int64, startedMs int64) string {
	return fmt.Sprintf("%s:%s:%d:%d", idPart(host), idPart(pidDomain), pid, startedMs)
}

// ClaimedPaneKey is the pane_key for a binding taken from a session record's
// own claim, with no live server to confirm it.
//
// The pane id alone, deliberately: the session name and window id both change
// under a rename, a join-pane or a break-pane, and a key that moves lets one
// instance hold two open bindings on one pane.
func ClaimedPaneKey(paneID string) string { return "claimed:" + paneID }

// CanonicalSocket resolves a tmux socket path to the form the kernel reports,
// so /tmp/tmux-501/default and /private/tmp/tmux-501/default cannot fork one
// pane into two. An unresolvable path is returned unchanged: guessing would be
// worse than a key that is merely honest about what it saw.
func CanonicalSocket(path string) string {
	if path == "" {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

// The three table classes. The boundary is load bearing: a foreign key from a
// spine or durable table into a projection makes a rebuild impossible, which
// is why TestNothingForeignKeysIntoAProjection reads these lists rather than
// a comment.

// SpineTables are append-only and are never dropped.
func SpineTables() []string {
	return []string{"source", "source_scan", "event", "evidence"}
}

// ProjectionTables are derived from the spine and may be dropped and rebuilt.
func ProjectionTables() []string {
	return []string{
		"conversation", "conversation_lineage", "launch_instance",
		"instance_conversation", "pane_binding",
	}
}

// DurableTables hold operator-facing facts that no replay can reconstruct.
func DurableTables() []string {
	return []string{
		"project_association", "item", "item_source",
		"exposure", "outcome", "decision",
	}
}
