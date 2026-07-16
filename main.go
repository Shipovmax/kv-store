// Command kv-store runs an HTTP server on top of an in-memory KV store
// with TTL support and graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"kv-store/handler"
	"kv-store/store"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// defaultAddr is the HTTP server address used when the KV_STORE_ADDR
// environment variable is not set.
const defaultAddr = ":8080"

// shutdownTimeout is the maximum time to wait for in-flight requests to
// complete when the server is stopping.
const shutdownTimeout = 5 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := os.Getenv("KV_STORE_ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	s := store.New()
	s.StartSweep(ctx, 0)

	h := handler.New(s, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /set", h.SetKey)
	mux.HandleFunc("GET /get", h.GetKey)
	mux.HandleFunc("DELETE /delete", h.DeleteKey)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("starting server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return err
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	// srv.Shutdown returns after the ListenAndServe goroutine has finished
	// and sent to serveErr — we wait for that so the process never exits
	// while that goroutine is still running.
	return <-serveErr
}
