package web

import (
	"crypto/sha256"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

const maxLimitEntries = 4096

type attemptWindow struct {
	count   int
	expires time.Time
}
type authLimits struct {
	mu      sync.Mutex
	entries map[[32]byte]attemptWindow
	active  int
}

func newAuthLimits() *authLimits { return &authLimits{entries: make(map[[32]byte]attemptWindow)} }

// Fixed windows shared by login and registration. Never evict active windows:
// when full, reject new identities instead of letting churn bypass the limits.
func (l *authLimits) acquire(ip, login string, now time.Time) (func(), time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, entry := range l.entries {
		if !now.Before(entry.expires) {
			delete(l.entries, key)
		}
	}
	keys := [2][32]byte{sha256.Sum256([]byte("ip:" + ip)), sha256.Sum256([]byte("login:" + login))}
	windows := [2]time.Duration{time.Minute, 15 * time.Minute}
	missing := 0
	var retry time.Duration
	for _, key := range keys {
		e, ok := l.entries[key]
		if !ok {
			missing++
		}
		if e.count >= 10 && e.expires.Sub(now) > retry {
			retry = e.expires.Sub(now)
		}
	}
	if retry > 0 {
		return nil, retry
	}
	if len(l.entries)+missing > maxLimitEntries {
		return nil, time.Minute
	}
	if l.active >= 2 {
		return nil, time.Second
	}
	for i, key := range keys {
		e := l.entries[key]
		if e.count == 0 {
			e.expires = now.Add(windows[i])
		}
		e.count++
		l.entries[key] = e
	}
	l.active++
	return func() { l.mu.Lock(); l.active--; l.mu.Unlock() }, 0
}

func clientIP(r *http.Request, trustFly bool) (string, bool) {
	var raw string
	if trustFly {
		// Only enable behind Fly HTTP proxy with the IPv4-only listener.
		// Reject missing, duplicate and malformed headers; never trust X-Forwarded-For.
		values := r.Header.Values("Fly-Client-IP")
		if len(values) != 1 {
			return "", false
		}
		raw = values[0]
	} else {
		var err error
		raw, _, err = net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return "", false
		}
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil || addr.Zone() != "" {
		return "", false
	}
	return addr.Unmap().String(), true
}
