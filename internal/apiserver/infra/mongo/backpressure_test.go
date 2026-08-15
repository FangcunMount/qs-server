package mongo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestBaseRepositoryUsesInjectedLimiter(t *testing.T) {
	wantErr := errors.New("backpressure timeout")
	repo := BaseRepository{
		limiter: failingAcquirer{err: wantErr},
	}

	if _, err := repo.CountDocuments(context.Background(), nil); !errors.Is(err, wantErr) {
		t.Fatalf("CountDocuments() error = %v, want %v", err, wantErr)
	}
}

func TestBaseRepositoryDoesNotReacquireLimiterInsideMongoSession(t *testing.T) {
	limiter := &countingAcquirer{}
	repo := BaseRepository{limiter: limiter}
	client, err := mongo.Connect(t.Context(), options.Client())
	if err != nil {
		t.Fatalf("mongo.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	session, err := client.StartSession()
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	t.Cleanup(func() { session.EndSession(context.Background()) })

	ctx, release, err := repo.acquire(mongo.NewSessionContext(t.Context(), session))
	if err != nil {
		t.Fatalf("acquire() error = %v", err)
	}
	if mongo.SessionFromContext(ctx) == nil {
		t.Fatal("acquire() lost Mongo session context")
	}
	release()
	if limiter.acquired != 0 || limiter.released != 0 {
		t.Fatalf("limiter counts = %d/%d, want 0/0 inside transaction", limiter.acquired, limiter.released)
	}
}

func TestBaseRepositoriesShareBackpressureCapacityConcurrently(t *testing.T) {
	const (
		maxInflight = 3
		requests    = 24
	)
	limiter := backpressure.NewLimiter(maxInflight, 50*time.Millisecond)
	repositories := []BaseRepository{
		{limiter: limiter},
		{limiter: limiter},
	}

	start := make(chan struct{})
	releaseAll := make(chan struct{})
	outcomes := make(chan error, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, release, err := repositories[index%len(repositories)].acquire(context.Background())
			outcomes <- err
			if err == nil {
				<-releaseAll
				release()
			}
		}(i)
	}
	close(start)

	acquired := 0
	timedOut := 0
	for i := 0; i < requests; i++ {
		err := <-outcomes
		switch {
		case err == nil:
			acquired++
		case errors.Is(err, context.DeadlineExceeded):
			timedOut++
		default:
			t.Fatalf("Acquire() error = %v, want nil or deadline exceeded", err)
		}
	}
	if acquired != maxInflight || timedOut != requests-maxInflight {
		t.Fatalf("acquired = %d, timed out = %d, want %d and %d", acquired, timedOut, maxInflight, requests-maxInflight)
	}

	close(releaseAll)
	wg.Wait()
	_, release, err := repositories[0].acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() after releases error = %v", err)
	}
	release()
}

type failingAcquirer struct {
	err error
}

type countingAcquirer struct {
	acquired int
	released int
}

func (c *countingAcquirer) Acquire(ctx context.Context) (context.Context, func(), error) {
	c.acquired++
	return ctx, func() { c.released++ }, nil
}

func (f failingAcquirer) Acquire(ctx context.Context) (context.Context, func(), error) {
	return ctx, func() {}, f.err
}
