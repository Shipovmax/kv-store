package store

import (
	"sync"
	"testing"
)

func TestConcurrentGetSet(t *testing.T) {
	s := New()
	s.Set("key", "value", 0)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, ok := s.Get("key")
			if !ok || v != "value" {
				t.Errorf("unexpected result: %q %v", v, ok)
			}
		}()
	}
	wg.Wait()
}
