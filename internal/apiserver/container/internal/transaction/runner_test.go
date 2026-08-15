package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type transactionLimiterSpy struct {
	acquired int
	released int
	err      error
}

type labeledTransactionError struct{}

func (labeledTransactionError) Error() string { return "commit result is unknown" }
func (labeledTransactionError) HasErrorLabel(label string) bool {
	return label == "UnknownTransactionCommitResult"
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
	runner := NewMongoRunner(client.Database("test"), MongoRunnerOptions{Boundary: "test", Limiter: limiter})

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

func TestMongoRunnerRequiresStableBoundaryBeforeStartingSession(t *testing.T) {
	client, err := mongo.Connect(t.Context(), options.Client())
	if err != nil {
		t.Fatalf("mongo.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	runner := NewMongoRunner(client.Database("test"), MongoRunnerOptions{})

	err = runner.WithinTransaction(t.Context(), func(context.Context) error {
		t.Fatal("callback must not run without a stable boundary")
		return nil
	})
	if err == nil || err.Error() != "mongo transaction boundary is required" {
		t.Fatalf("WithinTransaction() error = %v, want boundary validation error", err)
	}
}

func TestMongoRunnerRequiresLimiterBeforeStartingSession(t *testing.T) {
	client, err := mongo.Connect(t.Context(), options.Client())
	if err != nil {
		t.Fatalf("mongo.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	runner := NewMongoRunner(client.Database("test"), MongoRunnerOptions{Boundary: "test"})

	err = runner.WithinTransaction(t.Context(), func(context.Context) error {
		t.Fatal("callback must not run without a limiter")
		return nil
	})
	if err == nil || err.Error() != `mongo transaction limiter is required for boundary "test"` {
		t.Fatalf("WithinTransaction() error = %v, want limiter validation error", err)
	}
}

func TestMongoTransactionOutcomeRecognizesWrappedCommitUnknown(t *testing.T) {
	if got := mongoTransactionOutcome(errors.Join(errors.New("wrapper"), labeledTransactionError{})); got != "commit_unknown" {
		t.Fatalf("mongoTransactionOutcome() = %q, want commit_unknown", got)
	}
	if got := mongoTransactionOutcome(errors.New("callback failed")); got != "rolled_back" {
		t.Fatalf("mongoTransactionOutcome() = %q, want rolled_back", got)
	}
}

func TestMongoCallbackRetryMetricCountsAttemptsBeyondFirst(t *testing.T) {
	metric := mongoTransactionCallbackRetries.WithLabelValues("metric_test")
	before := testutil.ToFloat64(metric)
	observeMongoCallbackAttempts("metric_test", 3)
	if delta := testutil.ToFloat64(metric) - before; delta != 2 {
		t.Fatalf("callback retry metric delta = %f, want 2", delta)
	}
}
