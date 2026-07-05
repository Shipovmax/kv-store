// Command kv-store запускает HTTP-сервер поверх in-memory KV-хранилища
// с поддержкой TTL и graceful shutdown по SIGINT/SIGTERM.
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

// defaultAddr — адрес HTTP-сервера, если переменная окружения KV_STORE_ADDR
// не задана.
const defaultAddr = ":8080"

// shutdownTimeout — максимальное время ожидания завершения in-flight
// запросов при остановке сервера.
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

	// srv.Shutdown возвращается после того, как ListenAndServe в горутине
	// завершится и отправит в serveErr — ждём этого, чтобы не завершать
	// процесс с висящей незавершённой горутиной.
	return <-serveErr
}
