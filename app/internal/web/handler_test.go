package web

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"sumsub-demo/internal/auth"
	"sumsub-demo/internal/store"
)

func TestRoutes(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	service, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(service, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path      string
		status            int
		contentType, body string
	}{
		{"GET", "/", 200, "text/html; charset=utf-8", `href="/account"`},
		{"GET", "/account", 200, "text/html; charset=utf-8", `href="/"`},
		{"GET", "/healthz", 200, "text/plain; charset=utf-8", "ok\n"},
		{"GET", "/missing", 404, "", ""},
		{"GET", "/account/extra", 404, "", ""},
		{"GET", "/healthz/extra", 404, "", ""},
		{"POST", "/", 405, "", ""},
		{"POST", "/account", 405, "", ""},
		{"POST", "/healthz", 405, "", ""},
		{"HEAD", "/", 200, "text/html; charset=utf-8", ""},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if tc.contentType != "" && response.Header().Get("Content-Type") != tc.contentType {
				t.Errorf("Content-Type = %q, want %q", response.Header().Get("Content-Type"), tc.contentType)
			}
			if !strings.Contains(response.Body.String(), tc.body) {
				t.Errorf("response does not contain %q", tc.body)
			}
		})
	}
}
