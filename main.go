package main

import (
	"context"
	"kv-store/handler"
	"kv-store/store"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s := store.New()
	s.StartSweep(ctx)

	h := handler.New(s)

	mux := http.NewServeMux()
	mux.HandleFunc("/set", h.SetKey)
	mux.HandleFunc("/get", h.GetKey)
	mux.HandleFunc("/delete", h.DeleteKey)

	srv := &http.Server{Addr: ":8080", Handler: mux}

	go func() {
		srv.ListenAndServe()
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
