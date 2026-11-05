//go:build unix

package homerun

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

// sshsigInfo is what the verifier reads back out of an SSHSIG signature after ssh-keygen has accepted it:
// the namespace it was made in and, when the signing key is a certificate, the certificate's identity.
// It is recorded in the audit trail; acceptance is ssh-keygen's decision, not this parser's.
type sshsigInfo struct {
	Namespace string
	KeyType   string
	Cert      *certInfo
}

type certInfo struct {
	Serial      uint64
	KeyID       string
	Principals  []string
	ValidAfter  uint64
	ValidBefore uint64
}

const (
	sshsigBegin = "-----BEGIN SSH SIGNATURE-----"
	sshsigEnd   = "-----END SSH SIGNATURE-----"
)

// certKeyFields is the number of key-specific SSH strings between the nonce and the serial in each
// certificate type.
var certKeyFields = map[string]int{
	"ssh-ed25519-cert-v01@openssh.com":            1,
	"ecdsa-sha2-nistp256-cert-v01@openssh.com":    2,
	"ecdsa-sha2-nistp384-cert-v01@openssh.com":    2,
	"ecdsa-sha2-nistp521-cert-v01@openssh.com":    2,
	"ssh-rsa-cert-v01@openssh.com":                2,
	"sk-ssh-ed25519-cert-v01@openssh.com":         2,
	"sk-ecdsa-sha2-nistp256-cert-v01@openssh.com": 3,
}

type wireReader struct{ b []byte }

var errShort = errors.New("truncated SSH wire data")

func (r *wireReader) u32() (uint32, error) {
	if len(r.b) < 4 {
		return 0, errShort
	}
	v := binary.BigEndian.Uint32(r.b)
	r.b = r.b[4:]
	return v, nil
}

func (r *wireReader) u64() (uint64, error) {
	if len(r.b) < 8 {
		return 0, errShort
	}
	v := binary.BigEndian.Uint64(r.b)
	r.b = r.b[8:]
	return v, nil
}

func (r *wireReader) str() ([]byte, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if uint64(n) > uint64(len(r.b)) {
		return nil, errShort
	}
	s := r.b[:n]
	r.b = r.b[n:]
	return s, nil
}

// parseSSHSig decodes an armored SSHSIG block.
func parseSSHSig(armored []byte) (*sshsigInfo, error) {
	s := strings.TrimSpace(string(armored))
	rest, ok := strings.CutPrefix(s, sshsigBegin)
	if !ok {
		return nil, errors.New("not an SSH signature (missing armor header)")
	}
	body, ok := strings.CutSuffix(rest, sshsigEnd)
	if !ok {
		return nil, errors.New("not an SSH signature (missing armor footer)")
	}
	blob, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(body), ""))
	if err != nil {
		return nil, fmt.Errorf("SSH signature armor: %w", err)
	}
	if !bytes.HasPrefix(blob, []byte("SSHSIG")) {
		return nil, errors.New("SSH signature magic missing")
	}
	r := &wireReader{b: blob[6:]}
	ver, err := r.u32()
	if err != nil || ver != 1 {
		return nil, errors.New("unsupported SSH signature version")
	}
	pub, err := r.str()
	if err != nil {
		return nil, err
	}
	ns, err := r.str()
	if err != nil {
		return nil, err
	}
	info := &sshsigInfo{Namespace: string(ns)}
	pr := &wireReader{b: pub}
	kt, err := pr.str()
	if err != nil {
		return nil, err
	}
	info.KeyType = string(kt)
	if fields, isCert := certKeyFields[info.KeyType]; isCert {
		c, err := parseCert(pr, fields)
		if err != nil {
			return nil, fmt.Errorf("signing certificate: %w", err)
		}
		info.Cert = c
	}
	return info, nil
}

func parseCert(r *wireReader, keyFields int) (*certInfo, error) {
	if _, err := r.str(); err != nil { // nonce
		return nil, err
	}
	for i := 0; i < keyFields; i++ {
		if _, err := r.str(); err != nil {
			return nil, err
		}
	}
	c := &certInfo{}
	var err error
	if c.Serial, err = r.u64(); err != nil {
		return nil, err
	}
	if _, err = r.u32(); err != nil { // type
		return nil, err
	}
	id, err := r.str()
	if err != nil {
		return nil, err
	}
	c.KeyID = string(id)
	pr, err := r.str()
	if err != nil {
		return nil, err
	}
	pw := &wireReader{b: pr}
	for len(pw.b) > 0 {
		p, err := pw.str()
		if err != nil {
			return nil, err
		}
		c.Principals = append(c.Principals, string(p))
	}
	if c.ValidAfter, err = r.u64(); err != nil {
		return nil, err
	}
	if c.ValidBefore, err = r.u64(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *certInfo) validAt(t time.Time) bool {
	u := uint64(t.Unix())
	return u >= c.ValidAfter && u < c.ValidBefore
}
