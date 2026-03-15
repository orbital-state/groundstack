package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/orbital-state/groundstack/internal/config"
	"github.com/orbital-state/groundstack/internal/httpserver"
	"github.com/orbital-state/groundstack/internal/state"
)

func main() {
	if err := run(); err != nil {
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()

	dbStatus := func() string { return "disabled" }
	if cfg.DBURL != "" {
		pool, err := state.Connect(ctx, cfg.DBURL)
		if err != nil {
			log.Printf("db: connect failed: %v", err)
			dbStatus = func() string { return "error" }
		} else {
			defer pool.Close()
			dbStatus = func() string { return "connected" }
		}
	}

	mux := httpserver.NewMux(dbStatus)
	srv, err := httpserver.New(httpserver.Options{
		ListenAddr: cfg.ListenAddr,
		Handler:    mux,
	})
	if err != nil {
		return err
	}

	log.Printf("listening on %s", srv.Addr())

	serveErr := make(chan error, 1)
	go func() {
		err := srv.Serve()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		log.Printf("signal: %s", sig)
	case err := <-serveErr:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
