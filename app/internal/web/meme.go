package web

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
)

func (h *handler) home(w http.ResponseWriter, r *http.Request) {
	user, err := h.auth.CurrentUser(r.Context(), sessionToken(r))
	if err != nil {
		serverError(w)
		return
	}
	p := page{Title: "Главная", Heading: "Котик после проверки", Message: "Здесь появится мем с котиком после успешной проверки личности. Пока можно заглянуть в личный кабинет.", Link: "/account", Label: "В личный кабинет"}
	if user != nil {
		switch user.Status {
		case "retry":
			p.CardClass = "card-retry"
			p.Heading = "Нужно исправить материалы."
			p.Message = "Продолжите проверку личности в личном кабинете."
			p.Label = "Исправить материалы"
		case "rejected":
			p.CardClass = "card-final"
			p.Heading = "Проверка личности окончательно отклонена."
			p.Message = "Доступ к мему закрыт для этого аккаунта."
			p.Link = ""
		case "approved":
			p.Meme = true
		}
	}
	h.render(w, http.StatusOK, p)
}

func (h *handler) meme(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	user, err := h.auth.CurrentUser(r.Context(), sessionToken(r))
	if err != nil {
		serverError(w)
		return
	}
	if user == nil {
		http.Error(w, "Войдите в аккаунт.", http.StatusUnauthorized)
		return
	}
	if user.Status != "approved" {
		http.Error(w, "Доступ откроется после успешной проверки личности.", http.StatusForbidden)
		return
	}
	f, err := os.Open(h.memePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
		} else {
			serverError(w)
		}
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		serverError(w)
		return
	}
	var prefix [512]byte
	n, err := f.Read(prefix[:])
	if err != nil && !errors.Is(err, io.EOF) {
		serverError(w)
		return
	}
	contentType := http.DetectContentType(prefix[:n])
	if contentType != "image/jpeg" && contentType != "image/png" {
		serverError(w)
		return
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		serverError(w)
		return
	}
	w.Header().Set("Content-Type", contentType)
	// Always return the current file, without cache validators or partial responses.
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, f)
	}
}
