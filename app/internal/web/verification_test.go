package web

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sumsub-demo/internal/auth"
	"sumsub-demo/internal/store"
)

type tokenFunc func(context.Context, string) (string, error)

func (f tokenFunc) Token(ctx context.Context, id string) (string, error) { return f(ctx, id) }

func TestVerificationRoutes(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	fail := false
	issuer := tokenFunc(func(ctx context.Context, id string) (string, error) {
		ids = append(ids, id)
		if fail {
			return "", errors.New("sensitive-provider-error")
		}
		return "test-sdk-token", nil
	})
	h, err := NewVerificationHandler(service, false, "missing.jpg", issuer)
	if err != nil {
		t.Fatal(err)
	}
	const endpoint = "/account/verification/token"
	guest := request(h, "POST", endpoint, "", nil, nil)
	expect(t, guest, 401, "")
	if guest.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("guest response cacheable")
	}
	cookies := make([]*http.Cookie, 2)
	for i, login := range []string{"alice", "bob"} {
		token, _, err := service.Authenticate(context.Background(), login, testPassword, "", true)
		if err != nil {
			t.Fatal(err)
		}
		cookies[i] = &http.Cookie{Name: "session", Value: token}
	}
	alice, bob := cookies[0], cookies[1]
	headers := map[string]string{"X-Verification-Session": verificationBinding(alice.Value)}
	w := request(h, "GET", "/account", "", alice, nil)
	expect(t, w, 200, "Подтвердить личность")
	if !strings.Contains(w.Body.String(), `data-session="`+headers["X-Verification-Session"]+`"`) || strings.Contains(w.Body.String(), alice.Value) {
		t.Fatal("page binding missing or cookie exposed")
	}
	expect(t, request(h, "POST", endpoint, "", alice, nil), 409, "")
	expect(t, request(h, "POST", endpoint, "", bob, headers), 409, "")
	expect(t, request(h, "GET", endpoint, "", alice, headers), 405, "")
	cross := map[string]string{"X-Verification-Session": headers["X-Verification-Session"], "Sec-Fetch-Site": "cross-site", "Origin": "https://evil.invalid"}
	expect(t, request(h, "POST", endpoint, "", alice, cross), 403, "")
	if len(ids) != 0 {
		t.Fatal("unauthorized request reached provider")
	}
	for range 2 {
		w = request(h, "POST", endpoint, `userId=bob&levelName=other`, alice, headers)
		expect(t, w, 200, "test-sdk-token")
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
			t.Fatal("token headers")
		}
	}
	expect(t, request(h, "POST", endpoint, "", bob, map[string]string{"X-Verification-Session": verificationBinding(bob.Value)}), 200, "")
	if len(ids) != 3 || ids[0] != ids[1] || ids[0] == ids[2] || ids[0] == "alice" {
		t.Fatal("identity isolation/renewal failed")
	}
	meme := request(h, "GET", "/meme", "", alice, nil)
	if meme.Code != 403 || meme.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("meme access changed")
	}
	expect(t, request(h, "GET", "/account/verification/session", "", alice, headers), 204, "")
	fail = true
	w = request(h, "POST", endpoint, "", alice, headers)
	expect(t, w, 502, "")
	if strings.Contains(w.Body.String(), "sensitive") {
		t.Fatal("provider error leaked")
	}
	expect(t, request(h, "POST", "/logout", "", alice, nil), 303, "")
	expect(t, request(h, "POST", endpoint, "", alice, headers), 401, "")
	token, _, err := service.Authenticate(context.Background(), "alice", testPassword, "", false)
	if err != nil {
		t.Fatal(err)
	}
	renewed := &http.Cookie{Name: "session", Value: token}
	fail = false
	expect(t, request(h, "POST", endpoint, "", renewed, headers), 409, "")
	expect(t, request(h, "POST", endpoint, "", renewed, map[string]string{"X-Verification-Session": verificationBinding(token)}), 200, "")
	if ids[len(ids)-1] != ids[0] {
		t.Fatal("login changed external id")
	}
	disabled, _ := NewHandler(service, false, "missing.jpg")
	expect(t, request(disabled, "POST", endpoint, "", renewed, map[string]string{"X-Verification-Session": verificationBinding(token)}), 503, "")
	script := request(h, "GET", "/assets/verification.js", "", nil, nil)
	if script.Code != 200 || !strings.Contains(script.Body.String(), "snsWebSdk") {
		t.Fatal("missing client script")
	}
	if !strings.HasPrefix(script.Header().Get("Content-Type"), "text/javascript") {
		t.Fatal("wrong script type")
	}
	db.Close()
	expect(t, request(h, "POST", endpoint, "", renewed, headers), 500, "")
}

func TestVerificationExpiredAndRevokedDuringExchange(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	user := &store.User{Login: "alice", PasswordHash: "unused"}
	now := time.Now()
	ctx := context.Background()
	if err = db.CreateSession(ctx, user, true, auth.Digest(strings.Repeat("e", 64)), "", now.Add(-time.Hour), now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	issuer := tokenFunc(func(ctx context.Context, id string) (string, error) {
		calls++
		if err := db.DeleteSession(ctx, auth.Digest(strings.Repeat("c", 64))); err != nil {
			t.Error(err)
		}
		return "must-not-leak", nil
	})
	h, err := NewVerificationHandler(service, false, "missing.jpg", issuer)
	if err != nil {
		t.Fatal(err)
	}
	w := request(h, "POST", "/account/verification/token", "", &http.Cookie{Name: "session", Value: strings.Repeat("e", 64)}, map[string]string{"X-Verification-Session": verificationBinding(strings.Repeat("e", 64))})
	expect(t, w, 401, "")
	if calls != 0 {
		t.Fatal("expired session reached provider")
	}
	if err = db.CreateSession(ctx, user, false, auth.Digest(strings.Repeat("c", 64)), "", now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	w = request(h, "POST", "/account/verification/token", "", &http.Cookie{Name: "session", Value: strings.Repeat("c", 64)}, map[string]string{"X-Verification-Session": verificationBinding(strings.Repeat("c", 64))})
	expect(t, w, 401, "")
	if strings.Contains(w.Body.String(), "must-not-leak") || calls != 1 {
		t.Fatal("revoked session received token")
	}
}
