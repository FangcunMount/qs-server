package statisticsv2

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type warmerStub struct {
	called bool
	err    error
}

func (w *warmerStub) Warm(context.Context, int64, time.Time) error {
	w.called = true
	return w.err
}

func TestPublisherSwitchesGenerationBeforeWarmup(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	warmer := &warmerStub{err: errors.New("warm failed")}
	publisher := NewPublisher(NewGenerationPublisher(client), warmer)
	if err := publisher.Publish(context.Background(), 9, time.Now()); err == nil {
		t.Fatal("warmup failure must be returned")
	}
	if !warmer.called {
		t.Fatal("warmer was not called")
	}
	value, err := client.Get(context.Background(), GenerationKey(9)).Int64()
	if err != nil || value != 1 {
		t.Fatalf("generation=%d err=%v", value, err)
	}
}
