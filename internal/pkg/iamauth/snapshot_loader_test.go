package iamauth

import (
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

func TestSnapshotCacheRejectsResponseOlderThanNotification(t *testing.T) {
	l := NewSnapshotLoader(nil, SnapshotLoaderOptions{CacheTTL: time.Minute})
	key := cacheKey("42", "qs")
	l.ObserveAuthzVersion(12)
	if err := l.setCached(key, &authz.Snapshot{AuthzVersion: 11}); err == nil {
		t.Fatal("late stale response accepted")
	}
	if l.getCached(key) != nil {
		t.Fatal("stale response cached")
	}
	if err := l.setCached(key, &authz.Snapshot{AuthzVersion: 12}); err != nil {
		t.Fatal(err)
	}
	if l.getCached(key) == nil {
		t.Fatal("current response not cached")
	}
}

func TestGlobalVersionInvalidatesEverySubjectAndNeverRegresses(t *testing.T) {
	l := NewSnapshotLoader(nil, SnapshotLoaderOptions{})
	for _, user := range []string{"42", "43"} {
		if err := l.setCached(cacheKey(user, "qs"), &authz.Snapshot{AuthzVersion: 10}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for version := int64(11); version <= 100; version++ {
		wg.Add(1)
		go func(v int64) { defer wg.Done(); l.ObserveAuthzVersion(v) }(version)
	}
	wg.Wait()
	for _, user := range []string{"42", "43"} {
		if l.getCached(cacheKey(user, "qs")) != nil {
			t.Fatal("old subject snapshot survived")
		}
	}
	if err := l.setCached(cacheKey("44", "qs"), &authz.Snapshot{AuthzVersion: 99}); err == nil {
		t.Fatal("watermark regressed")
	}
}
