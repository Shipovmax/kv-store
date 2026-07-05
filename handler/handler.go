// Package handler содержит тонкий HTTP-слой поверх store.KVStore:
// разбор запроса, вызов хранилища, сериализация ответа. Никакой
// бизнес-логики (валидации TTL-политик, вычислений и т.п.) здесь нет —
// она принадлежит store.
package handler

import (
	"encoding/json"
	"kv-store/store"
	"log/slog"
	"net/http"
	"time"
)

// maxRequestBodyBytes ограничивает размер тела запроса, чтобы клиент не мог
// исчерпать память сервера одним запросом с гигантским телом.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

// Handler связывает HTTP-запросы с хранилищем KVStore.
type Handler struct {
	store  *store.KVStore
	logger *slog.Logger
}

// New создаёт Handler поверх переданного хранилища. Если logger == nil,
// используется slog.Default().
func New(s *store.KVStore, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{store: s, logger: logger}
}

// setRequest — тело запроса POST /set.
type setRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// TTLSeconds — время жизни ключа в секундах. 0 или отсутствие поля
	// означает бессрочное хранение. Отрицательное значение — ошибка 400.
	TTLSeconds int `json:"ttl_seconds"`
}

// SetKey обрабатывает POST /set.
//
// Запрос: JSON { "key": string, "value": string, "ttl_seconds"?: int }.
// Ответ: 200 { "ok": true }.
// Ошибки: 400 при невалидном JSON, пустом key или отрицательном ttl_seconds.
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

// GetKey обрабатывает GET /get?key=....
//
// Ответ: 200 { "key": string, "value": string }.
// Ошибки: 400 при отсутствующем параметре key, 404 если ключ не найден
// или истёк.
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

// DeleteKey обрабатывает DELETE /delete?key=....
//
// Ответ: 200 { "ok": true } — идемпотентно, в том числе для отсутствующего
// ключа. Ошибки: 400 при отсутствующем параметре key.
func (h *Handler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		h.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
		return
	}

	h.store.Delete(key)
	h.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// writeJSON сериализует v в тело ответа. Ошибка кодирования возможна только
// после того как заголовки уже отправлены (WriteHeader вызван), поэтому
// клиенту её сообщить нельзя — она логируется структурированно как
// диагностика на стороне сервера.
func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.logger.Error("failed to encode json response", "error", err, "status", status)
	}
}
