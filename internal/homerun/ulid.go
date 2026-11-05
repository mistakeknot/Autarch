//go:build unix

package homerun

import (
	"crypto/rand"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newULID returns a 26-character ULID: 48 bits of Unix milliseconds then 80 random bits, Crockford base32.
// Attempt IDs only need to be unique and roughly time-ordered; the store's primary key enforces the first.
func newULID(now time.Time) (string, error) {
	var b [16]byte
	ms := uint64(now.UnixMilli())
	for i := 0; i < 6; i++ {
		b[i] = byte(ms >> (8 * (5 - i)))
	}
	if _, err := rand.Read(b[6:]); err != nil {
		return "", err
	}
	// 128 bits -> 26 groups of 5 bits (the first group carries only 3 bits).
	out := make([]byte, 26)
	var acc, nbits int
	idx := 25
	for i := 15; i >= 0; i-- {
		acc |= int(b[i]) << nbits
		nbits += 8
		for nbits >= 5 && idx >= 1 {
			out[idx] = crockford[acc&31]
			acc >>= 5
			nbits -= 5
			idx--
		}
	}
	out[0] = crockford[acc&31]
	return string(out), nil
}
