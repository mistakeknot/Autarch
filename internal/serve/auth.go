package serve

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// LoadOrCreateToken reads the bearer token at path, creating it (32 random
// bytes, hex, mode 0600) when absent. A token file readable by group or other
// is refused.
func LoadOrCreateToken(path string) (string, error) {
	if path == "" {
		return "", errors.New("serve: token path is empty")
	}
	fi, err := os.Stat(path)
	switch {
	case err == nil:
		if fi.Mode().Perm()&0o077 != 0 {
			return "", fmt.Errorf("serve: token file %s has mode %04o; refusing (want 0600)", path, fi.Mode().Perm())
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		tok := strings.TrimSpace(string(b))
		if len(tok) != 64 {
			return "", fmt.Errorf("serve: token file %s is not a 64 hex-character token; refusing", path)
		}
		if _, err := hex.DecodeString(tok); err != nil {
			return "", fmt.Errorf("serve: token file %s is not a 64 hex-character token; refusing", path)
		}
		return tok, nil
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return "", err
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		tok := hex.EncodeToString(raw)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return "", err
		}
		if _, err := f.WriteString(tok + "\n"); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		return tok, nil
	default:
		return "", err
	}
}

// guard is the P-3 middleware: Origin (403), then Host (403), then the bearer
// token (401). /health needs no token. There is no query-string token.
func guard(token string, allowOrigins []string, listenAddr string, next http.Handler) http.Handler {
	origins := map[string]bool{}
	for _, o := range allowOrigins {
		origins[strings.TrimRight(strings.TrimSpace(o), "/")] = true
	}
	listenHost, _, _ := net.SplitHostPort(listenAddr)
	hostOK := func(h string) bool {
		name := h
		if n, _, err := net.SplitHostPort(h); err == nil {
			name = n
		}
		name = strings.Trim(name, "[]")
		switch strings.ToLower(name) {
		case "localhost", "127.0.0.1", "::1":
			return true
		}
		return listenHost != "" && strings.EqualFold(name, listenHost)
	}
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && !origins[strings.TrimRight(o, "/")] {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		if !hostOK(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.URL.Path != "/health" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") ||
				subtle.ConstantTimeCompare([]byte(got), want) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
