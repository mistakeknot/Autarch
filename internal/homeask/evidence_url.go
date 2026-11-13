package homeask

import (
	"regexp"
	"strconv"
	"strings"
)

// Evidence URL grammar (identical to evidence.ts): literal https://; ASCII host
// up to 253 bytes, with 1-63 byte lowercase LDH labels (no edge hyphen or xn--
// prefix), or exactly four canonical decimal IPv4 octets 0-255. Non-IP hosts
// end in a label with a letter, not a 0x prefix, to avoid browser numeric-host
// interpretation. Optional port is canonical decimal 1-65535 (no leading zero).
// Tail: optional /path, ?query, #fragment, in that order. Characters are ASCII
// unreserved A-Z a-z 0-9 . _ ~ -, sub-delimiters !$&'()*+,;=, plus : @ / and %HH;
// query and fragment may also contain ?. No raw space, controls, backslash,
// userinfo, non-ASCII, IPv6, escaped host, or extra fragment marker is accepted.
var (
	evidenceLabel      = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	evidenceLetter     = regexp.MustCompile(`[a-z]`)
	evidenceNumberHost = regexp.MustCompile(`^[0-9.]+$`)
	evidenceOctet      = regexp.MustCompile(`^(0|[1-9][0-9]{0,2})$`)
	evidencePort       = regexp.MustCompile(`^[1-9][0-9]{0,4}$`)
	evidenceTail       = regexp.MustCompile(`^(/[A-Za-z0-9._~!$&'()*+,;=:@%/-]*)?(\?[A-Za-z0-9._~!$&'()*+,;=:@%/?-]*)?(#[A-Za-z0-9._~!$&'()*+,;=:@%/?-]*)?$`)
)

// Caption trim set: U+0009-000D, 0020, 00A0, 1680, 2000-200A, 2028, 2029,
// 202F, 205F, 3000, FEFF. Trimming is only for emptiness; stored text is unchanged.
// Plain captions separately reject C0/C1 controls (0000-001F, 007F-009F).
func evidenceWhitespace(r rune) bool {
	return (r >= 0x09 && r <= 0x0d) || r == 0x20 || r == 0xa0 || r == 0x1680 ||
		(r >= 0x2000 && r <= 0x200a) || r == 0x2028 || r == 0x2029 || r == 0x202f ||
		r == 0x205f || r == 0x3000 || r == 0xfeff
}
func evidenceASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}
func evidenceHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
func evidenceURL(s string) bool {
	if len(s) > 2048 || !strings.HasPrefix(s, "https://") {
		return false
	}
	if !evidenceASCII(s) {
		return false
	}
	authority, tail := s[len("https://"):], ""
	if i := strings.IndexAny(authority, "/?#"); i >= 0 {
		authority, tail = authority[:i], authority[i:]
	}
	host, port, hasPort := strings.Cut(authority, ":")
	if hasPort {
		if !evidencePort.MatchString(port) {
			return false
		}
		n, _ := strconv.Atoi(port)
		if n > 65535 {
			return false
		}
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	if evidenceNumberHost.MatchString(host) {
		if len(labels) != 4 {
			return false
		}
		for _, p := range labels {
			if !evidenceOctet.MatchString(p) {
				return false
			}
			n, _ := strconv.Atoi(p)
			if n > 255 {
				return false
			}
		}
	} else {
		for _, p := range labels {
			if !evidenceLabel.MatchString(p) || strings.HasPrefix(p, "xn--") {
				return false
			}
		}
		last := labels[len(labels)-1]
		if !evidenceLetter.MatchString(last) || strings.HasPrefix(last, "0x") {
			return false
		}
	}
	if !evidenceTail.MatchString(tail) {
		return false
	}
	for i := 0; i < len(tail); i++ {
		if tail[i] == '%' {
			if i+2 >= len(tail) || !evidenceHex(tail[i+1]) || !evidenceHex(tail[i+2]) {
				return false
			}
			i += 2
		}
	}
	return true
}
