package store

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestConcurrentGetSet(t *testing.T) {
	s := New()
	s.Set("key", "value", 0)

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			v, ok := s.Get("key")
			if !ok || v != "value" {
				t.Errorf("unexpected result: %q %v", v, ok)
			}
		})
	}
	wg.Wait()
}

func TestConcurrentSetGetDelete(t *testing.T) {
	s := New()

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { s.Set("key", "value", 0) })
		wg.Go(func() { s.Get("key") })
		wg.Go(func() { s.Delete("key") })
	}
	wg.Wait()
}

func TestSetWithoutTTLNeverExpires(t *testing.T) {
	s := New()
	s.Set("key", "value", 0)

	v, ok := s.Get("key")
	if !ok || v != "value" {
		t.Fatalf("expected value present, got %q %v", v, ok)
	}
}

func TestGetExpiredKeyNotFound(t *testing.T) {
	s := New()
	s.Set("key", "value", 10*time.Millisecond)

	time.Sleep(20 * time.Millisecond)

	if _, ok := s.Get("key"); ok {
		t.Fatal("expected expired key to be absent")
	}
}

func TestGetMissingKeyNotFound(t *testing.T) {
	s := New()
	if _, ok := s.Get("missing"); ok {
		t.Fatal("expected missing key to be absent")
	}
}

func TestDeleteRemovesKey(t *testing.T) {
	s := New()
	s.Set("key", "value", 0)
	s.Delete("key")

	if _, ok := s.Get("key"); ok {
		t.Fatal("expected deleted key to be absent")
	}
}

func TestDeleteMissingKeyIsNoop(t *testing.T) {
	s := New()
	s.Delete("missing")
}

func TestSetOverwritesExistingKey(t *testing.T) {
	s := New()
	s.Set("key", "first", 0)
	s.Set("key", "second", 0)

	v, ok := s.Get("key")
	if !ok || v != "second" {
		t.Fatalf("expected overwritten value, got %q %v", v, ok)
	}
}

func TestStartSweepRemovesExpiredEntries(t *testing.T) {
	s := New()
	s.Set("expiring", "value", 5*time.Millisecond)
	s.Set("permanent", "value", 0)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s.StartSweep(ctx, 10*time.Millisecond)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		s.mu.RLock()
		_, exists := s.data["expiring"]
		s.mu.RUnlock()
		if !exists {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	s.mu.RLock()
	_, expiringExists := s.data["expiring"]
	_, permanentExists := s.data["permanent"]
	s.mu.RUnlock()

	if expiringExists {
		t.Fatal("expected sweep to remove expired entry from underlying map")
	}
	if !permanentExists {
		t.Fatal("expected permanent entry to survive sweep")
	}
}

func TestStartSweepStopsOnContextCancel(t *testing.T) {
	s := New()
	ctx, cancel := context.WithCancel(t.Context())
	s.StartSweep(ctx, 5*time.Millisecond)
	cancel()

	// Give the goroutine time to notice the context cancellation and exit.
	// The public API has no explicit way to wait for goroutine exit without
	// leaking one — this test only asserts no panic/deadlock after cancel.
	time.Sleep(50 * time.Millisecond)
}
