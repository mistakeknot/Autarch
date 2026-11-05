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
// It starts no process other than ssh-keygen, launches no unit, and reads no credential. The service
// that wires it to a socket, a unit launcher and a review service is a later slice. The package is
// parameterised on every path (allowed signers, revocation list, state database, CAS root, audit
// mirror) so that nothing here names a deployment location.
package homerun
