package web

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"net/http"

	"sumsub-demo/internal/auth"
	"sumsub-demo/internal/store"
)

type TokenIssuer interface {
	Token(context.Context, string) (string, error)
}

//go:embed static/verification.js
var verificationJS []byte

func verificationBinding(token string) string { return auth.Digest("websdk-page:" + token) }

func (h *handler) verificationUser(w http.ResponseWriter, r *http.Request) *store.User {
	user, err := h.auth.CurrentUser(r.Context(), sessionToken(r))
	if err != nil {
		serverError(w)
		return nil
	}
	if user == nil {
		http.Error(w, "Войдите в аккаунт заново.", http.StatusUnauthorized)
		return nil
	}
	expected := verificationBinding(sessionToken(r))
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Verification-Session")), []byte(expected)) != 1 {
		http.Error(w, "Сессия изменилась. Откройте кабинет заново.", http.StatusConflict)
		return nil
	}
	return user
}

func (h *handler) verificationSession(w http.ResponseWriter, r *http.Request) {
	if h.verificationUser(w, r) == nil {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) verificationToken(w http.ResponseWriter, r *http.Request) {
	user := h.verificationUser(w, r)
	if user == nil {
		return
	}
	if !tokenAllowed(user.Status) {
		http.Error(w, "Запуск проверки недоступен для текущего статуса.", http.StatusForbidden)
		return
	}
	if h.issuer == nil {
		http.Error(w, "Проверка пока недоступна.", http.StatusServiceUnavailable)
		return
	}
	id, err := h.auth.Store.VerificationID(r.Context(), user.ID)
	if err != nil {
		serverError(w)
		return
	}
	token, err := h.issuer.Token(r.Context(), id)
	if err != nil {
		http.Error(w, "Не удалось открыть проверку. Попробуйте ещё раз.", http.StatusBadGateway)
		return
	}
	// Recheck after the upstream call: logout during a slow exchange must not
	// release another SDK token to a session that has already been revoked.
	if current := h.verificationUser(w, r); current == nil {
		return
	} else if !tokenAllowed(current.Status) {
		http.Error(w, "Статус проверки изменился.", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Token string `json:"token"`
	}{token})
}

func (h *handler) verificationScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(verificationJS)
}
