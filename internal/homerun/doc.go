// Package homerun is the pure-logic core of the Citadel script-run verifier (bead mk-21nfo, slice 1).
//
// It holds the pieces that decide whether a signed operation may run, and records that it did:
//
//   - the operation envelope: canonical (RFC 8785) encoding and field validation (envelope.go, jcs.go);
//   - a wrapper around "ssh-keygen -Y verify" for the approval signature (sigverify.go, sshsig.go);
//   - a durable, single-use nonce store with the attempt table in the same transaction (store.go);
//   - the audit record sequence, mirrored append-only to a JSON-lines file (audit.go);
//   - a content-addressed store for the bytes that run (cas.go);
//   - the limits as one config struct (limits.go);
//   - enforcement of those limits (enforce.go, slice 2): the per-card apply interval, the rolling daily
//     cap, the check-receipt store that gates apply and recover, the bad-signature lockout, and the
//     runtime caps as values (Limits.RuntimeCap, Attempt.RuntimeMax) for the launcher to read.
//
// Not built, by design ambiguity: the "# writes:" script header. The design says ReadWritePaths comes from
// it and that it is "part of the envelope's flags/header digest", but it fixes neither the header syntax
// (separator, quoting, glob use), nor the required path prefix as a configuration value, nor which
// envelope field carries its digest (the envelope has no writes field). Parsing it now would invent a
// signed format, so it waits for a ruling; ParseFlagsHeader is the model once the syntax is fixed.
//
// Launcher write paths (launcher.go, slice 3), and what is not closed. Before a unit is started the
// launcher resolves the service account's uid at every launch and refuses uid 0; checks every
// component of every requested write path as a real directory (no symlink) whose parents are owned
// by root or the service user and writable by neither group nor other; pins each directory's identity
// (device and inode, through an O_PATH descriptor held until systemd-run returns); and verifies once
// more as late as it can. Residual, accepted under design D9: an agent running as the service user
// can still swap a path component between the last check and the moment systemd builds the unit's
// mount namespace, because ownership cannot stop a user from renaming inside its own directory and an
// open descriptor does not stop a rename. The launch signature is an intent record, not a boundary
// against agents running as the same user; closing this needs root/svc-review isolation. Nothing in
// this package claims more.
//
// Review receipts (internal/homerun/review, slice 5): the signed-receipt format, the verifier-side check,
// and a review-service core with injected provider and signer. No real provider or key is wired.
//
// It starts no process other than ssh-keygen, launches no unit, and reads no credential. The service
// that wires it to a socket, a unit launcher and a review service is a later slice. The package is
// parameterised on every path (allowed signers, revocation list, state database, CAS root, audit
// mirror) so that nothing here names a deployment location.
package homerun
