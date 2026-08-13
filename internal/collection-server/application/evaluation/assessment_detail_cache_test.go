package evaluation

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	localcache "github.com/FangcunMount/qs-server/internal/pkg/cache/local"
)

type assessmentDetailReader struct {
	BFFReader
	value       *AssessmentDetailResponse
	err         error
	calls       atomic.Int32
	accessCalls atomic.Int32
}

func (r *assessmentDetailReader) AuthorizeAssessment(context.Context, uint64, uint64) error {
	r.accessCalls.Add(1)
	time.Sleep(5 * time.Millisecond)
	return r.err
}

func (r *assessmentDetailReader) GetMyAssessment(context.Context, uint64, uint64) (*AssessmentDetailResponse, error) {
	r.calls.Add(1)
	time.Sleep(5 * time.Millisecond)
	return cloneAssessmentDetailResponse(r.value), r.err
}

func newAssessmentDetailTestCache() *LocalAssessmentDetailCache {
	return NewLocalAssessmentDetailCache(localcache.Options{TTL: time.Minute, MaxEntries: 8})
}

func TestAssessmentAccessCacheIsPositivePairScopedAndSingleflighted(t *testing.T) {
	reader := &assessmentDetailReader{}
	cache := NewLocalAssessmentAccessCache(localcache.Options{TTL: time.Minute, MaxEntries: 8})
	service := NewQueryService(reader, WithAssessmentAccessReader(reader), WithAssessmentAccessCache(cache, true))
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := service.AuthorizeAssessment(context.Background(), 7, 42); err != nil {
				t.Errorf("AuthorizeAssessment: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := reader.accessCalls.Load(); got != 1 {
		t.Fatalf("access source calls = %d, want 1", got)
	}
	if err := service.AuthorizeAssessment(context.Background(), 8, 42); err != nil {
		t.Fatal(err)
	}
	if got := reader.accessCalls.Load(); got != 2 {
		t.Fatalf("different testee reused access token; calls = %d", got)
	}
}

func TestAssessmentAccessDoesNotCacheDeniedResult(t *testing.T) {
	reader := &assessmentDetailReader{err: errors.New("denied")}
	cache := NewLocalAssessmentAccessCache(localcache.Options{TTL: time.Minute, MaxEntries: 8})
	service := NewQueryService(reader, WithAssessmentAccessReader(reader), WithAssessmentAccessCache(cache, true))
	for range 2 {
		if err := service.AuthorizeAssessment(context.Background(), 7, 42); err == nil {
			t.Fatal("AuthorizeAssessment error = nil")
		}
	}
	if got := reader.accessCalls.Load(); got != 2 {
		t.Fatalf("denied source calls = %d, want 2", got)
	}
}

func TestAssessmentDetailCachesOnlyEvaluatedDTO(t *testing.T) {
	for _, status := range []string{"pending", "submitted", "failed", "evaluated"} {
		t.Run(status, func(t *testing.T) {
			reader := &assessmentDetailReader{value: &AssessmentDetailResponse{ID: "42", TesteeID: "7", Status: status}}
			service := NewQueryService(reader, WithAssessmentDetailCache(newAssessmentDetailTestCache(), true))
			for range 2 {
				if _, err := service.GetMyAssessment(context.Background(), 7, 42); err != nil {
					t.Fatal(err)
				}
			}
			wantCalls := int32(2)
			if status == "evaluated" {
				wantCalls = 1
			}
			if got := reader.calls.Load(); got != wantCalls {
				t.Fatalf("source calls = %d, want %d", got, wantCalls)
			}
		})
	}
}

func TestAssessmentDetailDoesNotCacheErrors(t *testing.T) {
	reader := &assessmentDetailReader{err: errors.New("source unavailable")}
	service := NewQueryService(reader, WithAssessmentDetailCache(newAssessmentDetailTestCache(), true))
	for range 2 {
		if _, err := service.GetMyAssessment(context.Background(), 7, 42); err == nil {
			t.Fatal("GetMyAssessment error = nil")
		}
	}
	if got := reader.calls.Load(); got != 2 {
		t.Fatalf("error source calls = %d, want 2", got)
	}
}

func TestAssessmentDetailCacheIdentityIncludesTestee(t *testing.T) {
	cache := newAssessmentDetailTestCache()
	cache.Set(7, 42, &AssessmentDetailResponse{ID: "42", TesteeID: "7", Status: "evaluated"})
	if _, ok := cache.Get(8, 42); ok {
		t.Fatal("detail cached for a different testee")
	}
}

func TestAssessmentDetailCacheDeepClonesNestedPointers(t *testing.T) {
	max := 100.0
	cache := newAssessmentDetailTestCache()
	cache.Set(7, 42, &AssessmentDetailResponse{
		ID: "42", Status: "evaluated",
		PrimaryScore: &ScoreValueResponse{Value: 88, Max: &max},
		Level:        &ResultLevelResponse{Code: "high"},
	})
	first, ok := cache.Get(7, 42)
	if !ok {
		t.Fatal("cache miss")
	}
	*first.PrimaryScore.Max = 1
	first.Level.Code = "mutated"
	second, _ := cache.Get(7, 42)
	if *second.PrimaryScore.Max != 100 || second.Level.Code != "high" {
		t.Fatalf("cached value was mutated: %#v", second)
	}
}

func TestAssessmentDetailSingleflightCoalescesSameIdentity(t *testing.T) {
	reader := &assessmentDetailReader{value: &AssessmentDetailResponse{ID: "42", TesteeID: "7", Status: "evaluated"}}
	service := NewQueryService(reader, WithAssessmentDetailCache(newAssessmentDetailTestCache(), true))
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.GetMyAssessment(context.Background(), 7, 42); err != nil {
				t.Errorf("GetMyAssessment: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := reader.calls.Load(); got != 1 {
		t.Fatalf("source calls = %d, want 1", got)
	}
}
