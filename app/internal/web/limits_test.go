package web

import (
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimitWindows(t *testing.T) {
	for _, mode := range []string{"ip", "login"} {
		t.Run(mode, func(t *testing.T) {
			l := newAuthLimits()
			now := time.Now()
			for i := 0; i < 10; i++ {
				ip, login := "ip", "login"
				if mode == "ip" {
					login = fmt.Sprint(i)
				} else {
					ip = fmt.Sprint(i)
				}
				release, retry := l.acquire(ip, login, now)
				if retry != 0 {
					t.Fatal(retry)
				}
				release()
			}
			release, retry := l.acquire("ip", "login", now)
			if release != nil || retry <= 0 {
				t.Fatal("limit bypassed")
			}
			wait := time.Minute
			if mode == "login" {
				wait = 15 * time.Minute
			}
			release, retry = l.acquire("ip", "login", now.Add(wait))
			if retry != 0 {
				t.Fatal("window did not expire")
			}
			release()
		})
	}
}
func TestLimitConcurrencyAndCapacity(t *testing.T) {
	l := newAuthLimits()
	now := time.Now()
	var wg sync.WaitGroup
	var admitted atomic.Int32
	releases := make(chan func(), 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			release, retry := l.acquire(fmt.Sprint(i), fmt.Sprint(i), now)
			if retry == 0 {
				admitted.Add(1)
				releases <- release
			}
		}(i)
	}
	wg.Wait()
	close(releases)
	if admitted.Load() != 2 {
		t.Fatalf("admitted %d", admitted.Load())
	}
	for release := range releases {
		release()
	}
	l = newAuthLimits()
	for i := 0; i < maxLimitEntries/2; i++ {
		release, retry := l.acquire(fmt.Sprint(i), fmt.Sprint(i), now)
		if retry != 0 {
			t.Fatal(retry)
		}
		release()
	}
	if release, retry := l.acquire("new", "new", now); release != nil || retry == 0 {
		t.Fatal("capacity bypassed")
	}
	if len(l.entries) != maxLimitEntries {
		t.Fatal(len(l.entries))
	}
	release, retry := l.acquire("new", "new", now.Add(15*time.Minute))
	if retry != 0 {
		t.Fatal(retry)
	}
	release()
	if len(l.entries) != 2 {
		t.Fatal("expired entries retained")
	}
}
func TestClientIPTrust(t *testing.T) {
	r := httptest.NewRequest("POST", "/login", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("Fly-Client-IP", "198.51.100.1")
	r.Header.Set("X-Forwarded-For", "203.0.113.1")
	if ip, ok := clientIP(r, false); !ok || ip != "192.0.2.1" {
		t.Fatal(ip, ok)
	}
	if ip, ok := clientIP(r, true); !ok || ip != "198.51.100.1" {
		t.Fatal(ip, ok)
	}
	for _, bad := range []string{"", "unknown", "198.51.100.1, 203.0.113.1", "fe80::1%en0"} {
		r.Header.Set("Fly-Client-IP", bad)
		if _, ok := clientIP(r, true); ok {
			t.Fatal(bad)
		}
	}
	r.Header.Set("Fly-Client-IP", "198.51.100.1")
	r.Header.Add("Fly-Client-IP", "198.51.100.2")
	if _, ok := clientIP(r, true); ok {
		t.Fatal("duplicate accepted")
	}
}
func TestHTTPAuthenticationLimits(t *testing.T) {
	h, db := newTestHandler(t, filepath.Join(t.TempDir(), "app.db"), false)
	defer db.Close()
	for i := 0; i < 10; i++ {
		expect(t, request(h, "POST", "/login", form(" Alice ", "wrong"), nil, map[string]string{"Fly-Client-IP": fmt.Sprintf("192.0.2.%d", i)}), 401, "")
	}
	for _, path := range []string{"/login", "/register"} {
		w := request(h, "POST", path, form("ALICE", testPassword), nil, nil)
		expect(t, w, 429, "Слишком много попыток")
		if w.Header().Get("Retry-After") == "" || len(w.Result().Cookies()) != 0 {
			t.Fatal("bad rejection")
		}
	}
	// Saturated auth must not block health, pages or logout.
	if w := request(h, "GET", "/healthz", "", nil, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	expect(t, request(h, "POST", "/logout", "", nil, nil), 303, "")
}
