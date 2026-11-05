// Package homerun is the pure-logic core of the Citadel script-run verifier (bead mk-21nfo, slice 1).
//
// It holds the pieces that decide whether a signed operation may run, and records that it did:
//
//   - the operation envelope: canonical (RFC 8785) encoding and field validation (envelope.go, jcs.go);
//   - a wrapper around "ssh-keygen -Y verify" for the approval signature (sigverify.go, sshsig.go);
//   - a durable, single-use nonce store with the attempt table in the same transaction (store.go);
//   - the audit record sequence, mirrored append-only to a JSON-lines file (audit.go);
//   - a content-addressed store for the bytes that run (cas.go);
//   - the limits as one config struct (limits.go).
//
// It starts no process other than ssh-keygen, launches no unit, and reads no credential. The service
// that wires it to a socket, a unit launcher and a review service is a later slice. The package is
// parameterised on every path (allowed signers, revocation list, state database, CAS root, audit
// mirror) so that nothing here names a deployment location.
package homerun
