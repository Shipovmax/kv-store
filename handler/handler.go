// Package handler is a thin HTTP layer on top of store.KVStore: request
// parsing, store invocation, response serialization. It contains no
// business logic (TTL policy validation, computation, etc.) — that
// belongs to store.
package handler

import (
	"encoding/json"
	"kv-store/store"
	"log/slog"
	"net/http"
	"time"
)

// maxRequestBodyBytes caps the request body size so a client cannot
// exhaust server memory with a single oversized request.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

// Handler wires HTTP requests to a KVStore.
type Handler struct {
	store  *store.KVStore
	logger *slog.Logger
}

// New creates a Handler on top of the given store. If logger is nil,
// slog.Default() is used.
func New(s *store.KVStore, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{store: s, logger: logger}
}

// setRequest is the request body for POST /set.
type setRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// TTLSeconds is the key's time-to-live in seconds. 0 or an absent
	// field means the key never expires. A negative value is a 400 error.
	TTLSeconds int `json:"ttl_seconds"`
}

// SetKey handles POST /set.
//
// Request: JSON { "key": string, "value": string, "ttl_seconds"?: int }.
// Response: 200 { "ok": true }.
// Errors: 400 on invalid JSON, empty key, or negative ttl_seconds.
func (h *Handler) SetKey(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var req setRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	if req.Key == "" {
		h.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
		return
	}
	if req.TTLSeconds < 0 {
		h.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ttl_seconds must not be negative"})
		return
	}

	h.store.Set(req.Key, req.Value, time.Duration(req.TTLSeconds)*time.Second)
	h.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GetKey handles GET /get?key=....
//
// Response: 200 { "key": string, "value": string }.
// Errors: 400 if the key parameter is missing, 404 if the key is not found
// or has expired.
func (h *Handler) GetKey(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		h.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
		return
	}

	value, ok := h.store.Get(key)
	if !ok {
		h.writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": value})
}

// DeleteKey handles DELETE /delete?key=....
//
// Response: 200 { "ok": true } — idempotent, including for an absent key.
// Errors: 400 if the key parameter is missing.
func (h *Handler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		h.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
		return
	}

	h.store.Delete(key)
	h.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// writeJSON serializes v into the response body. An encoding error can
// only occur after headers have already been sent (WriteHeader has been
// called), so it cannot be reported to the client — it is logged
// structurally as server-side diagnostics instead.
func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.logger.Error("failed to encode json response", "error", err, "status", status)
	}
}
