package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sumsub-demo/internal/auth"
	"sumsub-demo/internal/store"
)

const webhookTestSecret = "local-test-webhook-secret"

func signedEvent(h http.Handler, body string, change func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/webhooks/sumsub", strings.NewReader(body))
	mac := hmac.New(sha256.New, []byte(webhookTestSecret))
	mac.Write([]byte(body))
	r.Header.Set("X-Payload-Digest-Alg", "HMAC_SHA256_HEX")
	r.Header.Set("X-Payload-Digest", hex.EncodeToString(mac.Sum(nil)))
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	if change != nil {
		change(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func eventBody(id, typ, answer, reject, at string) string {
	p := map[string]any{"type": typ, "applicantId": "test-applicant", "applicantType": "individual", "externalUserId": id, "levelName": "test-level", "sandboxMode": true, "createdAtMs": at, "reviewResult": map[string]string{"reviewAnswer": answer, "reviewRejectType": reject}}
	b, _ := json.Marshal(p)
	return string(b)
}
func TestWebhookAndStatus(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "results.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	svc, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := svc.Authenticate(ctx, "alice", testPassword, "", true)
	if err != nil {
		t.Fatal(err)
	}
	alice := &http.Cookie{Name: "session", Value: token}
	headers := map[string]string{"X-Verification-Session": verificationBinding(token)}
	user, _ := svc.CurrentUser(ctx, token)
	id, err := db.VerificationID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	bob := &store.User{Login: "bob", PasswordHash: "unused"}
	if err := db.CreateSession(ctx, bob, true, auth.Digest(strings.Repeat("b", 64)), "", time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	issuer := tokenFunc(func(context.Context, string) (string, error) { calls++; return "mock-token", nil })
	h, err := NewResultsHandler(svc, false, "../../resources/meme.jpg", issuer, webhookTestSecret, "test-level")
	if err != nil {
		t.Fatal(err)
	}
	body := eventBody(id, "applicantReviewed", "GREEN", "", "2026-11-26 12:00:00.001")
	for _, tc := range []struct {
		name, body string
		change     func(*http.Request)
		code       int
	}{
		{"missing algorithm", body, func(r *http.Request) { r.Header.Del("X-Payload-Digest-Alg") }, 403},
		{"wrong algorithm", body, func(r *http.Request) { r.Header.Set("X-Payload-Digest-Alg", "HMAC_SHA1_HEX") }, 403},
		{"duplicate algorithm", body, func(r *http.Request) { r.Header.Add("X-Payload-Digest-Alg", "HMAC_SHA256_HEX") }, 403},
		{"duplicate digest", body, func(r *http.Request) { r.Header.Add("X-Payload-Digest", "bad") }, 403},
		{"missing digest", body, func(r *http.Request) { r.Header.Del("X-Payload-Digest") }, 403},
		{"wrong digest", body, func(r *http.Request) { r.Header.Set("X-Payload-Digest", "ab") }, 403},
		{"changed bytes", body, func(r *http.Request) { r.Body = http.NoBody }, 403},
		{"oversized", strings.Repeat("x", 65537), nil, 413},
		{"invalid JSON", "{", nil, 400}, {"null", "null", nil, 400},
		{"missing fields", `{"type":"applicantCreated"}`, nil, 400},
		{"bad time", strings.Replace(body, "2026-11-26 12:00:00.001", "invalid", 1), nil, 400},
		{"bad review", strings.Replace(body, "GREEN", "BLUE", 1), nil, 400},
		{"production", strings.Replace(body, `"sandboxMode":true`, `"sandboxMode":false`, 1), nil, 200},
		{"test mode", strings.Replace(body, `"sandboxMode":true`, `"sandboxMode":true,"testMode":true`, 1), nil, 200},
		{"company", strings.Replace(body, "individual", "company", 1), nil, 200},
		{"other level", strings.Replace(body, "test-level", "other-level", 1), nil, 200},
		{"unknown user", strings.Replace(body, id, "unknown", 1), nil, 200},
		{"unsupported", strings.Replace(body, "applicantReviewed", "unsupported", 1), nil, 200},
	} {
		t.Run(tc.name, func(t *testing.T) { expect(t, signedEvent(h, tc.body, tc.change), tc.code, "") })
	}
	u, _ := svc.CurrentUser(ctx, token)
	if u.Status != "not_started" {
		t.Fatal("ignored event changed user")
	}
	disabled, _ := NewResultsHandler(svc, false, "", issuer, "", "test-level")
	expect(t, signedEvent(disabled, body, nil), 503, "")
	const statusURL = "/account/verification/status"
	expect(t, request(h, "GET", statusURL, "", nil, nil), 401, "")
	expect(t, request(h, "GET", statusURL, "", alice, nil), 409, "")
	for i, tc := range []struct {
		typ, answer, reject, status, action string
		allowed                             bool
	}{
		{"applicantCreated", "", "", "in_progress", "sdk", true},
		{"applicantPending", "", "", "pending", "none", true},
		{"applicantOnHold", "", "", "on_hold", "none", true},
		{"applicantReviewed", "RED", "RETRY", "retry", "sdk", true},
		{"applicantReviewed", "GREEN", "", "approved", "meme", false},
		{"applicantReviewed", "RED", "FINAL", "rejected", "none", false},
		{"applicantReset", "", "", "not_started", "sdk", true},
	} {
		at := time.Date(2026, 11, 26, 12, 0, i, 0, time.UTC).Format("2006-01-02 15:04:05.000")
		b := eventBody(id, tc.typ, tc.answer, tc.reject, at)
		expect(t, signedEvent(h, b, nil), 200, "")
		expect(t, signedEvent(h, b, nil), 200, "")
		w := request(h, "GET", statusURL+"?userId=bob", "", alice, headers)
		expect(t, w, 200, `"status":"`+tc.status+`"`)
		if w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), id) {
			t.Fatal("status exposure/cache")
		}
		expect(t, w, 200, `"action":"`+tc.action+`"`)
		before := calls
		code := 403
		if tc.allowed {
			code = 200
		}
		expect(t, request(h, "POST", "/account/verification/token", "", alice, headers), code, "")
		if !tc.allowed && calls != before {
			t.Fatal("terminal called issuer")
		}
		memeCode := 403
		if tc.status == "approved" {
			memeCode = 200
		}
		expectMemeResult(t, request(h, "GET", "/meme", "", alice, nil), memeCode, "")
		bCookie := &http.Cookie{Name: "session", Value: strings.Repeat("b", 64)}
		expect(t, request(h, "GET", statusURL, "", bCookie, headers), 409, "")
		expectMemeResult(t, request(h, "GET", "/meme", "", bCookie, nil), 403, "")
	}
	// Legacy seconds precision is accepted; invalid milliseconds never fall back.
	legacy := strings.Replace(eventBody(id, "applicantPending", "", "", "2026-11-26 12:00:30.000"), `"createdAtMs":"2026-11-26 12:00:30.000"`, `"createdAt":"2026-11-26 12:00:30"`, 1)
	expect(t, signedEvent(h, legacy, nil), 200, "")
	expect(t, request(h, "GET", statusURL, "", alice, headers), 200, `"status":"pending"`)
	// Newer approval, conflicting outcome at same time, replay and recovery.
	at := "2026-11-26 12:01:00.000"
	green := eventBody(id, "applicantReviewed", "GREEN", "", at)
	expect(t, signedEvent(h, green, nil), 200, "")
	expect(t, signedEvent(h, eventBody(id, "applicantReviewed", "RED", "FINAL", at), nil), 200, "")
	expect(t, signedEvent(h, green, nil), 200, "")
	expect(t, request(h, "GET", statusURL, "", alice, headers), 200, `"status":"conflict"`)
	expectMemeResult(t, request(h, "GET", "/meme", "", alice, nil), 403, "")
	expect(t, request(h, "POST", "/account/verification/token", "", alice, headers), 403, "")
	newer := strings.Replace(green, "12:01:00", "12:02:00", 1)
	expect(t, signedEvent(h, newer, nil), 200, "")
	expect(t, signedEvent(h, body, nil), 200, "")
	expect(t, signedEvent(h, strings.Replace(strings.Replace(newer, "test-applicant", "different", 1), "12:02:00", "12:03:00", 1), nil), 200, "")
	expect(t, request(h, "GET", statusURL, "", alice, headers), 200, `"status":"approved"`)
	expect(t, request(h, "GET", "/account", "", alice, nil), 200, "Личность подтверждена")
	db.Close()
	expect(t, signedEvent(h, newer, nil), 500, "")
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	svc.Store = db
	expect(t, signedEvent(h, newer, nil), 200, "")
	expect(t, request(h, "GET", statusURL, "", alice, headers), 200, `"status":"approved"`)
	expect(t, request(h, "POST", "/logout", "", alice, nil), 303, "")
	token, _, err = svc.Authenticate(ctx, "alice", testPassword, "", false)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, request(h, "GET", statusURL, "", &http.Cookie{Name: "session", Value: token}, map[string]string{"X-Verification-Session": verificationBinding(token)}), 200, `"status":"approved"`)
}

func expectMemeResult(t *testing.T, w *httptest.ResponseRecorder, code int, _ string) {
	t.Helper()
	if w.Code != code || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("meme: status %d want %d, cache %q", w.Code, code, w.Header().Get("Cache-Control"))
	}
}

func TestResultChangesDuringTokenExchange(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "token.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	u := &store.User{Login: "alice", PasswordHash: "unused"}
	token := strings.Repeat("c", 64)
	if err = db.CreateSession(ctx, u, true, auth.Digest(token), "", time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	issuer := tokenFunc(func(ctx context.Context, id string) (string, error) {
		_, err := db.ApplyVerification(ctx, store.VerificationEvent{ExternalID: id, ApplicantID: "applicant", Type: "applicantReviewed", Status: "approved", Millis: 1})
		return "must-not-leak", err
	})
	h, err := NewResultsHandler(svc, false, "", issuer, webhookTestSecret, "test-level")
	if err != nil {
		t.Fatal(err)
	}
	w := request(h, "POST", "/account/verification/token", "", &http.Cookie{Name: "session", Value: token}, map[string]string{"X-Verification-Session": verificationBinding(token)})
	expect(t, w, 403, "")
	if strings.Contains(w.Body.String(), "must-not-leak") {
		t.Fatal("late token leaked")
	}
}
