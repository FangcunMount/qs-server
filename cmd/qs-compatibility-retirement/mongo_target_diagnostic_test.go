package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

func targetDiagnosticTiming(ctx context.Context, now time.Time) context.Context {
	return context.WithValue(ctx, scanTimingKey{}, scanTiming{RunStarted: now.Add(-10 * time.Second), MySQLFinished: now.Add(-4 * time.Second)})
}

func TestMongoTargetDiagnosticUsesTypedErrorAndMonotonicObservations(t *testing.T) {
	now := time.Now()
	ctx := targetDiagnosticTiming(context.Background(), now)
	err := mongoIndexDiagnosticWrapped{mongo.CommandError{Code: 50, Message: "PRIVATE_URI_BSON_TOKEN", Name: "PRIVATE_USER"}}
	want := "QS_MONGO_TARGET_DIAGNOSTIC phase=find kind=server code=50 run_ctx=active page_ctx=active pass=1 page=3 records=2000 source_bytes=456789 page_elapsed_ms=2000 run_elapsed_ms=10000 mysql_elapsed_ms=6000"
	line := mongoTargetDiagnosticLine(ctx, ctx, "find", 1, 3, snapshot{Records: 2000, Bytes: 456789}, now.Add(-2*time.Second), now, err)
	if line != want || strings.Contains(line, "PRIVATE") || strings.ContainsAny(line, "\r\n") {
		t.Fatal("fixed body-free wire diagnostic changed")
	}
	for _, phase := range []string{"find", "iterate", "close"} {
		line = mongoTargetDiagnosticLine(ctx, ctx, phase, 1, 3, snapshot{Records: 2000, Bytes: 456789}, now.Add(-2*time.Second), now, err)
		if strings.Replace(line, "phase="+phase, "phase=find", 1) != want {
			t.Fatal("phase changed the typed observations")
		}
	}
}

func TestMongoTargetDiagnosticDistinguishesObservedRunPageAndSocketTimeout(t *testing.T) {
	now := time.Now()
	run, runCancel := context.WithDeadline(context.Background(), now.Add(-time.Second))
	defer runCancel()
	page, pageCancel := context.WithDeadline(context.Background(), now.Add(-time.Second))
	defer pageCancel()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name           string
		run, page      context.Context
		err            error
		kind, contexts string
	}{
		{"run-deadline", run, run, mongoIndexDiagnosticWrapped{context.DeadlineExceeded}, "kind=context_deadline code=0", "run_ctx=deadline page_ctx=deadline"},
		{"page-deadline", context.Background(), page, mongoIndexDiagnosticWrapped{context.DeadlineExceeded}, "kind=context_deadline code=0", "run_ctx=active page_ctx=deadline"},
		{"socket-timeout", context.Background(), context.Background(), mongoIndexDiagnosticWrapped{mongoIndexDiagnosticTimeout{}}, "kind=network_timeout code=0", "run_ctx=active page_ctx=active"},
		{"server-max-time", context.Background(), context.Background(), mongo.CommandError{Code: 50, Message: "PRIVATE"}, "kind=server code=50", "run_ctx=active page_ctx=active"},
		{"cancelled", context.Background(), cancelled, context.Canceled, "kind=context_cancelled code=0", "run_ctx=active page_ctx=cancelled"},
		{"unknown", context.Background(), context.Background(), errors.New("PRIVATE_URI"), "kind=other code=0", "run_ctx=active page_ctx=active"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			ctx := targetDiagnosticTiming(item.run, now)
			line := mongoTargetDiagnosticLine(ctx, item.page, "iterate", 2, 1, snapshot{}, now.Add(-time.Second), now, item.err)
			if !strings.Contains(line, item.kind) || !strings.Contains(line, item.contexts) || strings.Contains(line, "PRIVATE") {
				t.Fatal("typed timeout and sampled context states conflated")
			}
		})
	}
}

func TestMongoTargetDiagnosticRejectsMissingTimingAndUnboundedFields(t *testing.T) {
	now := time.Now()
	ctx := targetDiagnosticTiming(context.Background(), now)
	err := mongo.CommandError{Code: 13, Message: "PRIVATE"}
	cases := []struct {
		ctx, pageCtx context.Context
		phase        string
		pass, page   int
		snapshot     snapshot
		started      time.Time
		err          error
	}{
		{nil, ctx, "find", 1, 1, snapshot{}, now, err},
		{ctx, nil, "find", 1, 1, snapshot{}, now, err},
		{context.Background(), ctx, "find", 1, 1, snapshot{}, now, err},
		{ctx, ctx, "unknown\nPRIVATE", 1, 1, snapshot{}, now, err},
		{ctx, ctx, "find", 0, 1, snapshot{}, now, err},
		{ctx, ctx, "find", 3, 1, snapshot{}, now, err},
		{ctx, ctx, "find", 1, 0, snapshot{}, now, err},
		{ctx, ctx, "find", 1, 1002, snapshot{}, now, err},
		{ctx, ctx, "find", 1, 1, snapshot{Records: 1000001}, now, err},
		{ctx, ctx, "find", 1, 1, snapshot{Bytes: 1 + 2<<30}, now, err},
		{ctx, ctx, "find", 1, 1, snapshot{}, time.Time{}, err},
		{ctx, ctx, "find", 1, 1, snapshot{}, now.Add(time.Second), err},
		{ctx, ctx, "find", 1, 1, snapshot{}, now, nil},
	}
	for i, item := range cases {
		if mongoTargetDiagnosticLine(item.ctx, item.pageCtx, item.phase, item.pass, item.page, item.snapshot, item.started, now, item.err) != "" {
			t.Fatalf("invalid observation %d generated a diagnosis", i)
		}
	}
}
