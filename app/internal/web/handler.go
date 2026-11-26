package web

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"time"

	"sumsub-demo/internal/auth"
	"sumsub-demo/internal/store"
)

//go:embed templates/*.html
var templates embed.FS

type page struct {
	Title, Heading, Message, Link, Label string
	Account                              bool
	User                                 *store.User
	Error, Login                         string
}

type handler struct {
	auth   *auth.Service
	secure bool
	tmpl   *template.Template
}

func NewHandler(service *auth.Service, secure bool) (http.Handler, error) {
	tmpl, err := template.ParseFS(templates, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse page templates: %w", err)
	}
	h := &handler{auth: service, secure: secure, tmpl: tmpl}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		h.render(w, 200, page{Title: "Главная", Heading: "Котик после проверки", Message: "Здесь появится мем с котиком после успешной проверки личности. Пока можно заглянуть в личный кабинет.", Link: "/account", Label: "В личный кабинет"})
	})
	mux.HandleFunc("GET /account", h.account)
	mux.HandleFunc("POST /register", h.authenticate)
	mux.HandleFunc("POST /login", h.authenticate)
	mux.HandleFunc("POST /logout", h.logout)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	protected := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/account" || r.URL.Path == "/register" || r.URL.Path == "/login" || r.URL.Path == "/logout" {
			w.Header().Set("Cache-Control", "no-store")
		}
		protected.ServeHTTP(w, r)
	}), nil
}

func (h *handler) render(w http.ResponseWriter, status int, data page) {
	var body bytes.Buffer
	if err := h.tmpl.ExecuteTemplate(&body, "page.html", data); err != nil {
		serverError(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = body.WriteTo(w)
}
func accountPage() page {
	return page{Title: "Личный кабинет", Heading: "Личный кабинет", Account: true, Link: "/", Label: "На главную"}
}
func sessionToken(r *http.Request) string {
	c, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	return c.Value
}
func serverError(w http.ResponseWriter) {
	http.Error(w, "Внутренняя ошибка сервера. Попробуйте позже.", 500)
}
func (h *handler) account(w http.ResponseWriter, r *http.Request) {
	user, err := h.auth.CurrentUser(r.Context(), sessionToken(r))
	if err != nil {
		serverError(w)
		return
	}
	p := accountPage()
	p.User = user
	h.render(w, 200, p)
}
func (h *handler) formError(w http.ResponseWriter, status int, message, login string) {
	p := accountPage()
	p.Error = message
	p.Login = login
	h.render(w, status, p)
}
func (h *handler) parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	body, err := io.ReadAll(r.Body)
	if err == nil {
		r.Body = io.NopCloser(bytes.NewReader(body))
		err = r.ParseForm()
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.formError(w, 413, "Форма слишком большая (максимум 8 КиБ)", "")
		} else {
			h.formError(w, 400, "Не удалось прочитать форму", "")
		}
		return false
	}
	return true
}
func (h *handler) authenticate(w http.ResponseWriter, r *http.Request) {
	if !h.parseForm(w, r) {
		return
	}
	oldToken := sessionToken(r)
	user, err := h.auth.CurrentUser(r.Context(), oldToken)
	if err != nil {
		serverError(w)
		return
	}
	if user != nil {
		p := accountPage()
		p.User = user
		p.Error = "Сначала выйдите из текущего аккаунта"
		h.render(w, 409, p)
		return
	}
	login, password := auth.NormalizeLogin(r.PostForm.Get("login")), r.PostForm.Get("password")
	register := r.URL.Path == "/register"
	if register {
		if err := auth.Validate(login, password); err != nil {
			h.formError(w, 400, err.Error(), login)
			return
		}
	}
	token, expires, err := h.auth.Authenticate(r.Context(), login, password, oldToken, register)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrLoginTaken):
			h.formError(w, 409, "Этот логин уже занят", login)
		case errors.Is(err, auth.ErrCredentials):
			h.formError(w, 401, auth.ErrCredentials.Error(), login)
		default:
			serverError(w)
		}
		return
	}
	http.SetCookie(w, h.cookie(token, expires, int(auth.SessionLifetime.Seconds())))
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}
func (h *handler) cookie(token string, expires time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{Name: "session", Value: token, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: maxAge}
}
func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if !h.parseForm(w, r) {
		return
	}
	if err := h.auth.Store.DeleteSession(r.Context(), auth.Digest(sessionToken(r))); err != nil {
		serverError(w)
		return
	}
	http.SetCookie(w, h.cookie("", time.Unix(1, 0), -1))
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}
