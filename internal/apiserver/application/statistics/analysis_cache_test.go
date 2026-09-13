package statistics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	actorctx "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type analysisRevisionStub struct {
	mu      sync.Mutex
	reached chan struct{}
	*operationsStoreStub
	revision string
	changed  bool
	calls    int
	err      error
}

func (s *analysisRevisionStub) AnalysisCacheRevision(context.Context, int64, authz.StoreRange, string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.reached != nil {
		s.reached <- struct{}{}
	}
	if s.changed && s.calls%2 == 0 {
		return s.revision + "changed", s.err
	}
	return s.revision, s.err
}

type analysisMemoryCache struct {
	mu     sync.Mutex
	values map[string][]byte
	stale  bool
	sets   int
}

func (c *analysisMemoryCache) Get(_ context.Context, org int64, key string, out any) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.values[fmt.Sprintf("%d:%s", org, key)]
	return ok && json.Unmarshal(b, out) == nil, c.stale
}
func (c *analysisMemoryCache) Set(_ context.Context, org int64, key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, _ := json.Marshal(value)
	c.values[fmt.Sprintf("%d:%s", org, key)] = b
	c.sets++
}
func analysisCacheFixture() (*ReadService, *analysisRevisionStub, *operationsScope, *analysisMemoryCache, context.Context) {
	s, store, scope, _, ctx := operationsFixture()
	rev := &analysisRevisionStub{operationsStoreStub: store, revision: "current"}
	cache := &analysisMemoryCache{values: map[string][]byte{}}
	s.store = rev
	s.cache = cache
	return s, rev, scope, cache, actorctx.WithOperatorOrgID(ctx, 1)
}
func TestAnalysisCacheReusesOnlySamePublishedOwnershipAndScope(t *testing.T) {
	s, store, scope, cache, ctx := analysisCacheFixture()
	read := func() { t.Helper(); _, err := s.AnalysisOverview(ctx, 1, OperationsFilter{}); require.NoError(t, err) }
	read()
	read()
	require.Equal(t, 1, store.overviewReadHit)
	store.revision = "transferred"
	read()
	require.Equal(t, 2, store.overviewReadHit)
	store.snapshot.VisibleRunID++
	read()
	require.Equal(t, 3, store.overviewReadHit)
	scope.value = authz.StoreRange{AllStores: true}
	read()
	require.Equal(t, 4, store.overviewReadHit)
	before := cache.sets
	scope.err = errors.New("permission revoked")
	_, err := s.AnalysisOverview(ctx, 1, OperationsFilter{})
	require.Error(t, err)
	require.Equal(t, 4, store.overviewReadHit)
	require.Equal(t, before, cache.sets)
}
func TestAnalysisCacheRejectsConcurrentOwnershipOrPublicationChanges(t *testing.T) {
	for _, kind := range []string{"ownership", "publication", "revision-read"} {
		t.Run(kind, func(t *testing.T) {
			s, store, _, cache, ctx := analysisCacheFixture()
			switch kind {
			case "ownership":
				store.changed = true
			case "publication":
				store.nextSnapshot = &Snapshot{VisibleRunID: 8, DatabaseReadable: true}
			case "revision-read":
				store.err = errors.New("database unavailable")
			}
			_, err := s.AnalysisOverview(ctx, 1, OperationsFilter{})
			require.Error(t, err)
			require.Zero(t, cache.sets)
		})
	}
}
func TestAnalysisCacheDoesNotUseStaleFallbackOrWarmUnsafePublication(t *testing.T) {
	s, store, _, cache, ctx := analysisCacheFixture()
	_, err := s.AnalysisOverview(ctx, 1, OperationsFilter{})
	require.NoError(t, err)
	cache.stale = true
	_, err = s.AnalysisOverview(ctx, 1, OperationsFilter{})
	require.NoError(t, err)
	require.Equal(t, 2, store.overviewReadHit)
	store.snapshot.DatabaseReadable = false
	_, err = s.AnalysisOverview(ctx, 1, OperationsFilter{})
	require.Error(t, err)
	require.Equal(t, 2, store.overviewReadHit)
}
func TestAnalysisCacheSeparatesPaginationFiltersAndSafeDTOs(t *testing.T) {
	s, _, _, cache, ctx := analysisCacheFixture()
	_, err := s.AnalysisClinicians(ctx, 1, OperationsFilter{}, 1, 20)
	require.NoError(t, err)
	_, err = s.AnalysisClinicians(ctx, 1, OperationsFilter{}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, 1, cache.sets)
	_, err = s.AnalysisClinicians(ctx, 1, OperationsFilter{}, 2, 20)
	require.NoError(t, err)
	require.Equal(t, 2, cache.sets)
	id := uint64(42)
	_, err = s.AnalysisEntries(ctx, 1, OperationsFilter{}, &id, nil, 1, 20)
	require.NoError(t, err)
	active := true
	_, err = s.AnalysisEntries(ctx, 1, OperationsFilter{}, &id, &active, 1, 20)
	require.NoError(t, err)
	require.Equal(t, 4, cache.sets)
	id = 43
	_, err = s.AnalysisEntries(ctx, 1, OperationsFilter{}, &id, nil, 1, 20)
	require.Error(t, err)
	require.Equal(t, 4, cache.sets)
	for _, b := range cache.values {
		require.NotContains(t, string(b), "token")
	}
}

func TestAnalysisCacheCoalescesConcurrentColdReads(t *testing.T) {
	s, store, _, _, ctx := analysisCacheFixture()
	q, err := s.prepareAnalysis(ctx, 1, OperationsFilter{})
	require.NoError(t, err)
	const n = 8
	store.reached = make(chan struct{}, n+1)
	release := make(chan struct{})
	var loads atomic.Int32
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, e := analysisCached(ctx, s, 1, q, "overview", nil, func() (int, error) { loads.Add(1); <-release; return 42, nil })
			results <- e
		}()
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for i := 0; i < n; i++ {
		select {
		case <-store.reached:
		case <-timer.C:
			close(release)
			t.Fatal("concurrent requests did not reach revision check")
		}
	}
	close(release)
	for i := 0; i < n; i++ {
		require.NoError(t, <-results)
	}
	require.EqualValues(t, 1, loads.Load())
}
