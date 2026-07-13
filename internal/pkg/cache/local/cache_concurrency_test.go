package local

import (
	"sync"
	"testing"
	"time"
)

func TestGetExpiredDeleteDoesNotRemoveConcurrentSet(t *testing.T) {
	cache := New(Options{TTL: 5 * time.Millisecond, MaxEntries: 8}, func(v string) string { return v })
	cache.Set("k", "stale")
	time.Sleep(8 * time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		cache.Set("k", "fresh")
	}()

	go func() {
		defer wg.Done()
		_, _ = cache.Get("k")
	}()

	wg.Wait()

	got, ok := cache.Get("k")
	if !ok {
		t.Fatal("expected refreshed entry to survive concurrent expired delete")
	}
	if got != "fresh" {
		t.Fatalf("got %q, want fresh", got)
	}
}

func TestCacheConcurrentGetSetDeletePrefix(t *testing.T) {
	cache := New(Options{TTL: time.Minute, MaxEntries: 64}, func(v string) string { return v })

	const workers = 16
	const rounds = 200
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		worker := i
		go func() {
			defer wg.Done()
			for n := 0; n < rounds; n++ {
				key := string(rune('a' + (worker+n)%26))
				switch n % 3 {
				case 0:
					cache.Set(key, key)
				case 1:
					_, _ = cache.Get(key)
				default:
					cache.DeletePrefix("z")
				}
			}
		}()
	}

	wg.Wait()
}
