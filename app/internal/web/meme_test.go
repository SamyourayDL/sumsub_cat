package web

import (
	"bytes"
	"database/sql"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sumsub-demo/internal/auth"
	"sumsub-demo/internal/store"
)

func TestMemeAccessAndReplacement(t *testing.T) {
	dir := t.TempDir()
	dbPath, memePath := filepath.Join(dir, "test.db"), filepath.Join(dir, "meme.jpg")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(service, false, memePath)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	execSQL := func(query string, args ...any) {
		t.Helper()
		if _, err := sqlDB.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	var jpg, pngBytes bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	if err := jpeg.Encode(&jpg, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	write := func(body []byte) {
		t.Helper()
		next := memePath + ".next"
		if err := os.WriteFile(next, body, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(next, memePath); err != nil {
			t.Fatal(err)
		}
	}
	write(jpg.Bytes())
	register := func(login string) *http.Cookie {
		t.Helper()
		w := request(h, "POST", "/register", form(login, testPassword), nil, nil)
		expect(t, w, 303, "")
		return w.Result().Cookies()[0]
	}
	alice, bob := register("alice"), register("bob")
	execSQL("UPDATE users SET verification_status='approved' WHERE login='alice'")
	check := func(method, path string, cookie *http.Cookie, status int) []byte {
		t.Helper()
		w := request(h, method, path, "", cookie, nil)
		if w.Code != status {
			t.Fatalf("%s %s: got %d want %d", method, path, w.Code, status)
		}
		if w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("missing private no-store")
		}
		return w.Body.Bytes()
	}
	for _, method := range []string{"GET", "HEAD"} {
		check(method, "/meme", nil, 401)
		check(method, "/meme", &http.Cookie{Name: "session", Value: "invalid"}, 401)
		check(method, "/meme?user_id=1", bob, 403)
		w := request(h, method, "/meme", "", alice, nil)
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("bad image response: %d %v", w.Code, w.Header())
		}
		if method == "GET" && !bytes.Equal(w.Body.Bytes(), jpg.Bytes()) {
			t.Fatal("wrong JPEG")
		}
		if method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD has body")
		}
	}
	for _, cookie := range []*http.Cookie{nil, bob} {
		body := string(check("GET", "/", cookie, 200))
		if !strings.Contains(body, "Котик после проверки") || strings.Contains(body, `src="/meme"`) {
			t.Fatal("changed unapproved home")
		}
	}
	body := string(check("GET", "/", alice, 200))
	if !strings.Contains(body, `src="/meme"`) || strings.Contains(body, `<section class="card`) || !strings.Contains(body, `href="/account"`) {
		t.Fatal("bad approved home")
	}
	for _, path := range []string{"/resources/meme.jpg", "/meme.jpg", "/meme/extra"} {
		if request(h, "GET", path, "", alice, nil).Code != 404 {
			t.Fatal("exposed resource", path)
		}
	}
	check("POST", "/meme", alice, 405)
	write(pngBytes.Bytes())
	w := request(h, "GET", "/meme", "", alice, map[string]string{"If-Modified-Since": "Wed, 01 Jan 2031 00:00:00 GMT"})
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), pngBytes.Bytes()) {
		t.Fatal("replacement not visible")
	}
	execSQL("UPDATE users SET verification_status='pending' WHERE login='alice'")
	check("GET", "/meme", alice, 403)
	if strings.Contains(string(check("GET", "/", alice, 200)), `src="/meme"`) {
		t.Fatal("stale home")
	}
	for _, status := range []string{"", "rejected", "APPROVED", "unknown", "not_started"} {
		execSQL("UPDATE users SET verification_status=? WHERE login='alice'", status)
		check("HEAD", "/meme", alice, 403)
	}
	execSQL("UPDATE users SET verification_status='approved' WHERE login='alice'")
	for _, invalid := range [][]byte{nil, []byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>"), []byte("not an image")} {
		write(invalid)
		body := check("GET", "/meme", alice, 500)
		if bytes.Contains(body, []byte(dir)) {
			t.Fatal("path leak")
		}
	}
	if err := os.Remove(memePath); err != nil {
		t.Fatal(err)
	}
	check("GET", "/meme", alice, 404)
	check("GET", "/meme", bob, 403) // Authorization precedes filesystem access.
	check("GET", "/meme", nil, 401)
	if err := os.Mkdir(memePath, 0700); err != nil {
		t.Fatal(err)
	}
	check("GET", "/meme", alice, 500)
	if err := os.Remove(memePath); err != nil {
		t.Fatal(err)
	}
	write(jpg.Bytes())
	if err := os.Chmod(memePath, 0000); err != nil {
		t.Fatal(err)
	}
	// Root can read mode-000 files; exercise permission denial where supported.
	if probe, err := os.Open(memePath); err != nil {
		check("GET", "/meme", alice, 500)
	} else {
		probe.Close()
	}
	if err := os.Chmod(memePath, 0600); err != nil {
		t.Fatal(err)
	}
	expect(t, request(h, "POST", "/logout", "", alice, nil), 303, "")
	check("GET", "/meme", alice, 401)
	check("HEAD", "/meme", alice, 401)
	alice = request(h, "POST", "/login", form("alice", testPassword), nil, nil).Result().Cookies()[0]
	execSQL("UPDATE sessions SET expires_at=0 WHERE token_hash=?", auth.Digest(alice.Value))
	check("GET", "/meme", alice, 401)
	check("HEAD", "/meme", alice, 401)
	db.Close()
	check("GET", "/meme", bob, 500)
	check("HEAD", "/meme", bob, 500)
	check("GET", "/", bob, 500)
}
