package transaction

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type transactionLimiterSpy struct {
	acquired int
	released int
	err      error
}

func (s *transactionLimiterSpy) Acquire(ctx context.Context) (context.Context, func(), error) {
	s.acquired++
	if s.err != nil {
		return ctx, func() {}, s.err
	}
	return ctx, func() { s.released++ }, nil
}

func TestMongoRunnerWithLimiterRejectsBeforeStartingSession(t *testing.T) {
	wantErr := errors.New("mongo saturated")
	limiter := &transactionLimiterSpy{err: wantErr}
	client, err := mongo.Connect(t.Context(), options.Client())
	if err != nil {
		t.Fatalf("mongo.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	runner := NewMongoRunnerWithLimiter(client.Database("test"), limiter)

	err = runner.WithinTransaction(t.Context(), func(context.Context) error {
		t.Fatal("transaction callback must not run when limiter rejects")
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithinTransaction() error = %v, want %v", err, wantErr)
	}
	if limiter.acquired != 1 || limiter.released != 0 {
		t.Fatalf("limiter counts = acquired:%d released:%d, want 1/0", limiter.acquired, limiter.released)
	}
}

func TestMongoTransactionOptionsFreezePublishDurability(t *testing.T) {
	t.Parallel()

	opts := mongoTransactionOptions()
	if opts.ReadPreference == nil || opts.ReadPreference.Mode() != readpref.PrimaryMode {
		t.Fatalf("read preference = %#v, want primary", opts.ReadPreference)
	}
	if opts.ReadConcern == nil || opts.ReadConcern.Level != "snapshot" {
		t.Fatalf("read concern = %#v, want snapshot", opts.ReadConcern)
	}
	if opts.WriteConcern == nil || opts.WriteConcern.W != "majority" {
		t.Fatalf("write concern = %#v, want majority", opts.WriteConcern)
	}
}
