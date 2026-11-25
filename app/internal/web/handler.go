package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
)

//go:embed templates/*.html
var templates embed.FS

type page struct {
	Title   string
	Heading string
	Message string
	Link    string
	Label   string
}

// NewHandler parses embedded templates once, before the server starts.
func NewHandler() (http.Handler, error) {
	tmpl, err := template.ParseFS(templates, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse page templates: %w", err)
	}
	render := func(data page) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var body bytes.Buffer
			if err := tmpl.ExecuteTemplate(&body, "page.html", data); err != nil {
				http.Error(w, "Unable to render page", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = body.WriteTo(w)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", render(page{
		Title: "Главная", Heading: "Котик после проверки",
		Message: "Здесь появится мем с котиком после успешной проверки личности. Пока можно заглянуть в личный кабинет.",
		Link:    "/account", Label: "В личный кабинет",
	}))
	mux.HandleFunc("GET /account", render(page{
		Title: "Личный кабинет", Heading: "Личный кабинет",
		Message: "Регистрация, вход и проверка личности появятся на следующих этапах. Сейчас это стартовая страница кабинета.",
		Link:    "/", Label: "На главную",
	}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux, nil
}
