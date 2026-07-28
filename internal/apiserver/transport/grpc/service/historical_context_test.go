package service

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
)

func TestWithHistoricalExecutionContextFailsClosed(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	at := time.Date(2025, 1, 2, 9, 0, 0, 0, location)
	historical := historicalseed.Context{
		BatchID: "batch", ScenarioID: "scenario", OrgID: 7, Version: historicalseed.Version1,
		Timeline: historicalseed.Timeline{AnswerSheetFilledAt: &at},
	}

	if _, err := withHistoricalExecutionContext(context.Background(), historicalseed.ToProto(historical), 7, nil); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("disabled policy code = %v, want PermissionDenied (err=%v)", status.Code(err), err)
	}

	verifier := &historicalseed.Verifier{
		Enabled: true, AllowedOrgIDs: map[uint64]struct{}{7: {}}, Location: location,
		Earliest: time.Date(2025, 1, 1, 0, 0, 0, 0, location),
		Latest:   time.Date(2026, 7, 27, 0, 0, 0, 0, location),
	}
	ctx, err := withHistoricalExecutionContext(context.Background(), historicalseed.ToProto(historical), 7, verifier)
	if err != nil {
		t.Fatalf("withHistoricalExecutionContext() error = %v", err)
	}
	got, ok := historicalseed.FromContext(ctx)
	if !ok || got.BatchID != historical.BatchID {
		t.Fatalf("context = %#v, present=%v", got, ok)
	}

	if _, err := withHistoricalExecutionContext(context.Background(), historicalseed.ToProto(historical), 8, verifier); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("request org mismatch code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestWithHistoricalExecutionContextLeavesOrdinaryRequestUntouched(t *testing.T) {
	base := context.Background()
	got, err := withHistoricalExecutionContext(base, nil, 0, nil)
	if err != nil || got != base {
		t.Fatalf("ordinary request changed: ctx=%v err=%v", got, err)
	}
}
