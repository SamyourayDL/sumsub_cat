package main

import (
	"log"
	"net/http"
	"time"

	"sumsub-demo/internal/web"
)

func main() {
	handler, err := web.NewHandler()
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Print("Listening on http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}
