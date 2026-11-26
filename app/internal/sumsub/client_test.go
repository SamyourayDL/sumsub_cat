package sumsub

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSignedToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.RequestURI != tokenPath {
			t.Error("wrong endpoint")
		}
		if r.Header.Get("X-App-Token") != "test-app" || r.Header.Get("X-App-Access-Ts") != "1700000000" {
			t.Error("wrong auth headers")
		}
		mac := hmac.New(sha256.New, []byte("test-secret"))
		mac.Write(append([]byte("1700000000POST/resources/accessTokens/sdk"), body...))
		if r.Header.Get("X-App-Access-Sig") != hex.EncodeToString(mac.Sum(nil)) {
			t.Error("invalid signature")
		}
		var data map[string]any
		if json.Unmarshal(body, &data) != nil || data["userId"] != "demo-test" || data["levelName"] != "id + уровень" || data["ttlInSecs"] != float64(600) || len(data) != 3 {
			t.Error("incorrect payload")
		}
		io.WriteString(w, `{"token":"opaque-token","userId":"demo-test"}`)
	}))
	defer server.Close()
	c := New("test-app", "test-secret", "id + уровень")
	c.baseURL = server.URL
	c.now = func() time.Time { return time.Unix(1700000000, 0) }
	token, err := c.Token(context.Background(), "demo-test")
	if err != nil || token != "opaque-token" {
		t.Fatal("exchange failed", err)
	}
}

func TestProviderFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", 401, "secret-provider-body"}, {"limited", 429, "secret-provider-body"},
		{"server", 500, "secret-provider-body"}, {"invalid-json", 200, "secret-provider-body"},
		{"empty", 200, `{"token":"","userId":"demo-test"}`},
		{"wrong-user", 200, `{"token":"secret-token","userId":"other"}`},
		{"long-token", 200, `{"token":"` + strings.Repeat("x", 1025) + `","userId":"demo-test"}`},
		{"oversized", 200, strings.Repeat("x", 17000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) }))
			defer srv.Close()
			c := New("app", "secret", "level")
			c.baseURL = srv.URL
			token, err := c.Token(context.Background(), "demo-test")
			if token != "" || err != ErrUnavailable {
				t.Fatal("unsafe error or token")
			}
		})
	}
}

func TestTimeoutCancellationAndRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer srv.Close()
	c := New("app", "secret", "level")
	c.baseURL = srv.URL
	c.http.Timeout = 20 * time.Millisecond
	if _, err := c.Token(context.Background(), "demo-test"); err != ErrUnavailable {
		t.Fatal("timeout not handled")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Token(ctx, "demo-test"); err != ErrUnavailable {
		t.Fatal("cancellation not handled")
	}
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	c.baseURL = redirect.URL
	if _, err := c.Token(context.Background(), "demo-test"); err != ErrUnavailable || reached {
		t.Fatal("redirect followed")
	}
}
