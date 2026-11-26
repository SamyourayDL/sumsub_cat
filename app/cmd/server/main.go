package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sumsub-demo/internal/auth"
	"sumsub-demo/internal/config"
	"sumsub-demo/internal/store"
	"sumsub-demo/internal/sumsub"
	"sumsub-demo/internal/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	service, err := auth.New(db)
	if err != nil {
		return err
	}
	var issuer web.TokenIssuer
	if cfg.SumsubAppToken != "" {
		issuer = sumsub.New(cfg.SumsubAppToken, cfg.SumsubSecretKey, cfg.SumsubLevelName)
	}
	handler, err := web.NewVerificationHandler(service, cfg.CookieSecure, cfg.MemePath, issuer, cfg.TrustFlyProxy)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: "0.0.0.0:8080", Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	listener, err := net.Listen("tcp4", server.Addr)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
	}()
	log.Print("Listening on http://localhost:8080")
	err = server.Serve(listener)
	stop()
	<-done
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
