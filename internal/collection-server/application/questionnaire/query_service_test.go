package questionnaire

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
)

type stubQuestionnaireClient struct {
	getCalls int32
	getFn    func(ctx context.Context, code, version string) (*grpcclient.QuestionnaireOutput, error)
}

func (s *stubQuestionnaireClient) GetQuestionnaire(ctx context.Context, code, version string) (*grpcclient.QuestionnaireOutput, error) {
	atomic.AddInt32(&s.getCalls, 1)
	if s.getFn != nil {
		return s.getFn(ctx, code, version)
	}
	return &grpcclient.QuestionnaireOutput{
		Code:    code,
		Version: version,
		Title:   "sample",
	}, nil
}

func (s *stubQuestionnaireClient) ListQuestionnaires(context.Context, int32, int32, string, string) (*grpcclient.ListQuestionnairesOutput, error) {
	return &grpcclient.ListQuestionnairesOutput{}, nil
}

func TestQueryServiceGetUsesCacheOnSecondCall(t *testing.T) {
	client := &stubQuestionnaireClient{}
	cache := NewLocalCache(LocalCacheOptions{TTL: time.Minute, MaxEntries: 8})
	service := NewQueryService(client, cache, true)

	first, err := service.Get(context.Background(), "q1", "")
	if err != nil || first == nil {
		t.Fatalf("first get failed: resp=%+v err=%v", first, err)
	}
	second, err := service.Get(context.Background(), "q1", "")
	if err != nil || second == nil {
		t.Fatalf("second get failed: resp=%+v err=%v", second, err)
	}
	if got := atomic.LoadInt32(&client.getCalls); got != 1 {
		t.Fatalf("get calls = %d, want 1", got)
	}
}

func TestQueryServiceGetDoesNotCacheNilOrError(t *testing.T) {
	t.Run("nil response", func(t *testing.T) {
		client := &stubQuestionnaireClient{
			getFn: func(context.Context, string, string) (*grpcclient.QuestionnaireOutput, error) {
				return nil, nil
			},
		}
		cache := NewLocalCache(LocalCacheOptions{TTL: time.Minute, MaxEntries: 8})
		service := NewQueryService(client, cache, true)

		if _, err := service.Get(context.Background(), "q1", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := service.Get(context.Background(), "q1", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := atomic.LoadInt32(&client.getCalls); got != 2 {
			t.Fatalf("get calls = %d, want 2", got)
		}
	})

	t.Run("grpc error", func(t *testing.T) {
		client := &stubQuestionnaireClient{
			getFn: func(context.Context, string, string) (*grpcclient.QuestionnaireOutput, error) {
				return nil, errors.New("grpc down")
			},
		}
		cache := NewLocalCache(LocalCacheOptions{TTL: time.Minute, MaxEntries: 8})
		service := NewQueryService(client, cache, true)

		if _, err := service.Get(context.Background(), "q1", ""); err == nil {
			t.Fatal("expected grpc error")
		}
		if _, err := service.Get(context.Background(), "q1", ""); err == nil {
			t.Fatal("expected grpc error on second call")
		}
		if got := atomic.LoadInt32(&client.getCalls); got != 2 {
			t.Fatalf("get calls = %d, want 2", got)
		}
	})
}

func TestQueryServiceGetSingleflightCoalescesConcurrentMiss(t *testing.T) {
	client := &stubQuestionnaireClient{}
	start := make(chan struct{})
	client.getFn = func(ctx context.Context, code, version string) (*grpcclient.QuestionnaireOutput, error) {
		<-start
		return &grpcclient.QuestionnaireOutput{Code: code, Version: version, Title: "once"}, nil
	}

	cache := NewLocalCache(LocalCacheOptions{TTL: time.Minute, MaxEntries: 8})
	service := NewQueryService(client, cache, true)

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if _, err := service.Get(context.Background(), "q1", ""); err != nil {
				t.Errorf("get failed: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&client.getCalls); got != 1 {
		t.Fatalf("get calls = %d, want 1", got)
	}
}

func TestQueryServiceGetWithoutCacheAlwaysCallsGRPC(t *testing.T) {
	client := &stubQuestionnaireClient{}
	service := NewQueryService(client, nil, false)

	if _, err := service.Get(context.Background(), "q1", ""); err != nil {
		t.Fatalf("first get failed: %v", err)
	}
	if _, err := service.Get(context.Background(), "q1", ""); err != nil {
		t.Fatalf("second get failed: %v", err)
	}
	if got := atomic.LoadInt32(&client.getCalls); got != 2 {
		t.Fatalf("get calls = %d, want 2", got)
	}
}
