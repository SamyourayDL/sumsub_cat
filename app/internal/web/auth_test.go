package web

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sumsub-demo/internal/auth"
	"sumsub-demo/internal/store"
)

const testPassword = "test-password-12345"

func newTestHandler(t *testing.T, path string, secure bool) (http.Handler, *store.Store) {
	t.Helper()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(service, secure)
	if err != nil {
		t.Fatal(err)
	}
	return h, db
}
func request(h http.Handler, method, path, body string, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func form(login, password string) string {
	return url.Values{"login": {login}, "password": {password}}.Encode()
}
func expect(t *testing.T, w *httptest.ResponseRecorder, status int, text string) {
	t.Helper()
	if w.Code != status || !strings.Contains(w.Body.String(), text) {
		t.Fatalf("got %d, wanted %d with %q; body %s", w.Code, status, text, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
}
func TestAccountLifecycleAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	h, db := newTestHandler(t, path, false)
	defer func() { db.Close() }()
	w := request(h, "POST", "/register", form(" Alice ", testPassword), nil, nil)
	expect(t, w, 303, "")
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge != 86400 || time.Until(cookie.Expires) < 23*time.Hour {
		t.Fatalf("bad cookie attributes: %+v", cookie)
	}
	expect(t, request(h, "GET", "/account", "", cookie, nil), 200, "Проверка не начата")
	expect(t, request(h, "POST", "/register", form("other", testPassword), cookie, nil), 409, "Сначала выйдите")
	expect(t, request(h, "POST", "/login", form("other", testPassword), cookie, nil), 409, "Сначала выйдите")
	expect(t, request(h, "POST", "/register", form("ALICE", testPassword), nil, nil), 409, "Этот логин уже занят")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var hash, digest string
	if err := raw.QueryRow("SELECT password_hash FROM users WHERE login='alice'").Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if hash == testPassword || !strings.HasPrefix(hash, "$2a$12$") {
		t.Fatal("password not bcrypt cost 12")
	}
	if err := raw.QueryRow("SELECT token_hash FROM sessions").Scan(&digest); err != nil {
		t.Fatal(err)
	}
	if digest != auth.Digest(cookie.Value) || digest == cookie.Value {
		t.Fatal("raw token persisted")
	}
	if _, err := raw.Exec("UPDATE users SET verification_status='approved' WHERE login='alice'"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	h, db = newTestHandler(t, path, false)
	expect(t, request(h, "GET", "/account", "", cookie, nil), 200, "approved")
	expect(t, request(h, "POST", "/logout", "", cookie, nil), 303, "")
	expect(t, request(h, "GET", "/account", "", cookie, nil), 200, "Вход в кабинет")
	for _, login := range []string{"alice", "unknown"} {
		w = request(h, "POST", "/login", form(login, "wrong-password"), nil, nil)
		expect(t, w, 401, "Неверный логин или пароль")
		if len(w.Result().Cookies()) != 0 {
			t.Fatal("failed login issued cookie")
		}
	}
	w = request(h, "POST", "/login", form("ALICE", testPassword), cookie, nil)
	expect(t, w, 303, "")
	fresh := w.Result().Cookies()[0]
	if fresh.Value == cookie.Value {
		t.Fatal("token reused")
	}
	expect(t, request(h, "GET", "/account", "", fresh, nil), 200, "approved")
	if _, err := raw.Exec("UPDATE sessions SET expires_at=?", time.Now().Add(-time.Second).Unix()); err != nil {
		t.Fatal(err)
	}
	expect(t, request(h, "GET", "/account", "", fresh, nil), 200, "Вход в кабинет")
	expect(t, request(h, "GET", "/account", "", &http.Cookie{Name: "session", Value: strings.Repeat("a", 64)}, nil), 200, "Вход в кабинет")
	w = request(h, "POST", "/logout", "", fresh, nil)
	expect(t, w, 303, "")
	if w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout did not clear cookie")
	}
	db.Close()
	expect(t, request(h, "GET", "/account", "", fresh, nil), 500, "Внутренняя ошибка")
	expect(t, request(h, "POST", "/login", form("alice", testPassword), nil, nil), 500, "Внутренняя ошибка")
	expect(t, request(h, "POST", "/logout", "", fresh, nil), 500, "Внутренняя ошибка")
}
func TestFormsProtectionAndLimits(t *testing.T) {
	h, db := newTestHandler(t, filepath.Join(t.TempDir(), "app.db"), true)
	defer db.Close()
	for _, path := range []string{"/register", "/login", "/logout"} {
		expect(t, request(h, "GET", path, "", nil, nil), 405, "")
		expect(t, request(h, "HEAD", path, "", nil, nil), 405, "")
		expect(t, request(h, "POST", path, form("alice", testPassword), nil, map[string]string{"Origin": "https://evil.example"}), 403, "")
		expect(t, request(h, "POST", path, "", nil, map[string]string{"Sec-Fetch-Site": "cross-site"}), 403, "")
		expect(t, request(h, "POST", path, "x="+strings.Repeat("a", 8192), nil, nil), 413, "слишком большая")
	}
	expect(t, request(h, "POST", "/logout", strings.Repeat("a", 8193), nil, map[string]string{"Content-Type": "text/plain"}), 413, "слишком большая")
	for _, pair := range [][2]string{{"ab", testPassword}, {strings.Repeat("a", 33), testPassword}, {"a b", testPassword}, {"алиса", testPassword}, {"alice", "short"}, {"alice", strings.Repeat("я", 37)}} {
		w := request(h, "POST", "/register", form(pair[0], pair[1]), nil, nil)
		expect(t, w, 400, "")
		if strings.Contains(w.Body.String(), `value="`+pair[1]+`"`) {
			t.Fatal("password echoed")
		}
	}
	expect(t, request(h, "POST", "/register", "login=%zz", nil, nil), 400, "Не удалось")
	w := request(h, "POST", "/register", form("alice", testPassword), nil, map[string]string{"Origin": "http://example.com", "Sec-Fetch-Site": "same-origin"})
	expect(t, w, 303, "")
	if !w.Result().Cookies()[0].Secure {
		t.Fatal("missing Secure")
	}
}
func TestParallelDuplicateAndSeparateClients(t *testing.T) {
	h, db := newTestHandler(t, filepath.Join(t.TempDir(), "app.db"), false)
	defer db.Close()
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- request(h, "POST", "/register", form("alice", testPassword), nil, nil).Code
		}()
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[303] != 1 || counts[409] != 1 {
		t.Fatalf("statuses %v", counts)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	clients := make([]*http.Client, 2)
	for i, login := range []string{"alice", "bob"} {
		jar, _ := cookiejar.New(nil)
		clients[i] = &http.Client{Jar: jar}
		path := "/login"
		if i == 1 {
			path = "/register"
		}
		resp, err := clients[i].PostForm(server.URL+path, url.Values{"login": {login}, "password": {testPassword}})
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	for i, login := range []string{"alice", "bob"} {
		resp, err := clients[i].Get(server.URL + "/account")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		other := "bob"
		if i == 1 {
			other = "alice"
		}
		if !strings.Contains(string(body), "<strong>"+login+"</strong>") || strings.Contains(string(body), "<strong>"+other+"</strong>") {
			t.Fatal("client isolation failed")
		}
	}
}

func TestGuestFormNavigation(t *testing.T) {
	h, db := newTestHandler(t, filepath.Join(t.TempDir(), "app.db"), false)
	defer db.Close()
	for _, tc := range []struct {
		method, path, body, action, heading, link string
		status                                    int
	}{
		{"GET", "/account", "", "/login", "Вход в кабинет", "/account/register", 200},
		{"GET", "/account/register", "", "/register", "Создать аккаунт", "/account", 200},
		{"POST", "/register", form("ab", "short"), "/register", "Создать аккаунт", "/account", 400},
		{"POST", "/login", form("unknown", "wrong-password"), "/login", "Вход в кабинет", "/account/register", 401},
	} {
		w := request(h, tc.method, tc.path, tc.body, nil, nil)
		expect(t, w, tc.status, tc.heading)
		body := w.Body.String()
		if strings.Count(body, "<form ") != 1 || !strings.Contains(body, `action="`+tc.action+`"`) || !strings.Contains(body, `href="`+tc.link+`"`) {
			t.Fatalf("wrong form/navigation on %s %s", tc.method, tc.path)
		}
	}
	w := request(h, "POST", "/register", form("alice", testPassword), nil, nil)
	expect(t, w, 303, "")
	cookie := w.Result().Cookies()[0]
	w = request(h, "GET", "/account/register", "", cookie, nil)
	expect(t, w, 303, "")
	if w.Header().Get("Location") != "/account" {
		t.Fatal("authenticated user must return to account")
	}
}
