// Package store implements a thread-safe in-memory key-value store with
// TTL support and background cleanup of expired entries.
package store

import (
	"context"
	"sync"
	"time"
)

// defaultSweepInterval is the default period of the background sweep for
// expired keys, used when StartSweep is called without an explicit interval.
const defaultSweepInterval = time.Second

// entry is a stored value together with its TTL expiration moment.
// A zero expiresAt means no TTL (the key never expires).
type entry struct {
	value     string
	expiresAt time.Time
}

// expired reports whether the entry's TTL has elapsed as of now.
func (e entry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && now.After(e.expiresAt)
}

// KVStore is a thread-safe in-memory store of key-value pairs. Concurrent
// access is guarded by sync.RWMutex: reads (Get) allow an arbitrary number
// of concurrent readers, writes (Set/Delete) are exclusive. For a
// read-heavy workload (typical of a KV cache) this reduces contention
// compared to sync.Mutex.
//
// The zero value of KVStore is not usable — create an instance via New.
type KVStore struct {
	mu   sync.RWMutex
	data map[string]entry
}

// New creates an empty store, ready for use.
func New() *KVStore {
	return &KVStore{
		data: make(map[string]entry),
	}
}

// Set writes a value for key. If ttl > 0, the key expires after the given
// duration and becomes unavailable via Get; if ttl <= 0, the key is stored
// indefinitely (until an explicit Delete or overwrite).
func (s *KVStore) Set(key, value string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e := entry{value: value}
	if ttl > 0 {
		e.expiresAt = time.Now().Add(ttl)
	}
	s.data[key] = e
}

// Get returns the value for key. The second return value is false if the
// key is absent or its TTL has elapsed; in the latter case the entry is
// not physically removed immediately — that happens on the next sweep
// goroutine pass — but the expired key is already unavailable to the
// reader.
func (s *KVStore) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.data[key]
	if !ok || e.expired(time.Now()) {
		return "", false
	}
	return e.value, true
}

// Delete removes a key. Deleting an absent key is a no-op.
func (s *KVStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// StartSweep launches a background goroutine that periodically (every
// interval, or defaultSweepInterval if interval <= 0) removes expired keys
// from the store. The alternative — time.AfterFunc per key — spawns one
// goroutine per key for a large number of entries; a single ticker gives
// O(1) background goroutines at the cost of delaying deletion by up to one
// sweep period.
//
// The goroutine terminates when ctx is canceled — this is its only
// termination path; the caller must eventually cancel ctx, otherwise the
// goroutine runs for the lifetime of the process.
func (s *KVStore) StartSweep(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultSweepInterval
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.sweepExpired()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// sweepExpired removes all entries with an elapsed TTL from the store.
// Complexity: O(n) in the number of keys per pass, O(1) extra memory —
// there is no way to visit every entry without a full scan absent an
// auxiliary index on expiresAt; for a very large number of TTL keys, a
// min-heap ordered by expiresAt would allow amortized removal without a
// full scan, but that is only justified when the fraction of TTL keys is
// high and deletion latency has hard requirements.
func (s *KVStore) sweepExpired() {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.data {
		if v.expired(now) {
			delete(s.data, k)
		}
	}
}
