package store

import (
	"context"
	"sync"
	"time"
)

type entry struct {
	value     string
	expiresAt time.Time
	hasTTL    bool
}

type KVStore struct {
	mu   sync.RWMutex
	data map[string]entry
}

func New() *KVStore {
	return &KVStore{
		data: make(map[string]entry),
	}
}

func (s *KVStore) Set(key, value string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e := entry{value: value}
	if ttl > 0 {
		e.expiresAt = time.Now().Add(ttl)
		e.hasTTL = true
	}
	s.data[key] = e
}

func (s *KVStore) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	if !ok {
		return "", false
	}
	if e.hasTTL && time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.value, true
}

func (s *KVStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}
func (s *KVStore) StartSweep(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.mu.Lock()
				for k, v := range s.data {
					if v.hasTTL && time.Now().After(v.expiresAt) {
						delete(s.data, k)
					}
				}
				s.mu.Unlock()

			case <-ctx.Done():
				return
			}
		}
	}()
}
