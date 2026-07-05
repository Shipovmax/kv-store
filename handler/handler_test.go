package handler

import (
	"encoding/json"
	"io"
	"kv-store/store"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestHandler() *Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(store.New(), logger)
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return body
}

func TestSetKey_Success(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/set", strings.NewReader(`{"key":"k","value":"v"}`))
	rec := httptest.NewRecorder()

	h.SetKey(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if v, ok := h.store.Get("k"); !ok || v != "v" {
		t.Fatalf("expected key to be stored, got %q %v", v, ok)
	}
}

func TestSetKey_InvalidJSON(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/set", strings.NewReader(`not-json`))
	rec := httptest.NewRecorder()

	h.SetKey(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSetKey_EmptyKey(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/set", strings.NewReader(`{"key":"","value":"v"}`))
	rec := httptest.NewRecorder()

	h.SetKey(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSetKey_NegativeTTL(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/set", strings.NewReader(`{"key":"k","value":"v","ttl_seconds":-1}`))
	rec := httptest.NewRecorder()

	h.SetKey(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetKey_Success(t *testing.T) {
	h := newTestHandler()
	h.store.Set("k", "v", 0)

	req := httptest.NewRequest(http.MethodGet, "/get?key=k", nil)
	rec := httptest.NewRecorder()

	h.GetKey(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := decodeBody(t, rec)
	if body["value"] != "v" {
		t.Fatalf("expected value %q, got %v", "v", body["value"])
	}
}

func TestGetKey_MissingParam(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/get", nil)
	rec := httptest.NewRecorder()

	h.GetKey(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetKey_NotFound(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/get?key=missing", nil)
	rec := httptest.NewRecorder()

	h.GetKey(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteKey_Success(t *testing.T) {
	h := newTestHandler()
	h.store.Set("k", "v", 0)

	req := httptest.NewRequest(http.MethodDelete, "/delete?key=k", nil)
	rec := httptest.NewRecorder()

	h.DeleteKey(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if _, ok := h.store.Get("k"); ok {
		t.Fatal("expected key to be deleted")
	}
}

func TestDeleteKey_MissingParam(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodDelete, "/delete", nil)
	rec := httptest.NewRecorder()

	h.DeleteKey(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
