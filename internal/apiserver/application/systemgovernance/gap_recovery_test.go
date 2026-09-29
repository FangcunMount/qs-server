package systemgovernance

import (
	"context"
	"testing"
	"time"
)

type gapRecoveryStoreProbe struct{ calls int }

func (p *gapRecoveryStoreProbe) AuthorizeGapRecovery(_ context.Context, _ int64, _ uint64, _ GapRecoveryRequest) (*GapRecoveryDecision, error) {
	p.calls++
	return &GapRecoveryDecision{Authorized: true, Code: "authorized"}, nil
}

func (p *gapRecoveryStoreProbe) ResolveGapRecovery(_ context.Context, _ int64, _ uint64, _ GapRecoveryRequest) (*GapRecoveryDecision, bool, error) {
	p.calls++
	return &GapRecoveryDecision{Authorized: true, Code: "authorized"}, true, nil
}

func TestGapRecoveryFailsClosedWithoutStoreOrConfirmation(t *testing.T) {
	req := GapRecoveryRequest{
		RequestID: "recovery-1", AssessmentID: 42, EventID: "original-event",
		ExpectedVersion: 3, Reason: "reviewed", SubmittedBefore: time.Now().Add(-time.Hour),
	}
	ctx := context.Background()
	disabled := NewFacade(FacadeDeps{})
	if _, err := disabled.AuthorizeGapRecovery(ctx, 88, 701, req); err == nil {
		t.Fatal("disabled recovery authorized")
	}
	if _, _, err := disabled.ResolveGapRecovery(ctx, 88, 701, req); err == nil {
		t.Fatal("disabled recovery resolved")
	}
	probe := &gapRecoveryStoreProbe{}
	readOnly := NewFacade(FacadeDeps{GapRecoveryStore: probe})
	if _, err := readOnly.AuthorizeGapRecovery(ctx, 88, 701, req); err == nil || probe.calls != 0 {
		t.Fatal("disabled write switch reached storage")
	}
	if decision, found, err := readOnly.ResolveGapRecovery(ctx, 88, 701, req); err != nil || !found || decision == nil || probe.calls != 1 {
		t.Fatalf("disabled write switch lost original receipt: decision=%+v found=%v err=%v calls=%d", decision, found, err, probe.calls)
	}
	probe.calls = 0
	facade := NewFacade(FacadeDeps{GapRecoveryStore: probe, GapRecoveryEnabled: true})
	if _, err := facade.AuthorizeGapRecovery(ctx, 88, 701, req); err == nil || probe.calls != 0 {
		t.Fatal("unconfirmed recovery reached storage")
	}
	req.Confirm = true
	if _, err := facade.AuthorizeGapRecovery(ctx, 0, 701, req); err == nil || probe.calls != 0 {
		t.Fatal("unscoped recovery reached storage")
	}
	req.SubmittedBefore = time.Now()
	if _, err := facade.AuthorizeGapRecovery(ctx, 88, 701, req); err == nil || probe.calls != 0 {
		t.Fatal("assessment still inside grace window reached storage")
	}
	req.SubmittedBefore = time.Now().Add(-time.Hour)
	if _, err := facade.AuthorizeGapRecovery(ctx, 88, 701, req); err != nil || probe.calls != 1 {
		t.Fatalf("confirmed recovery err/calls = %v/%d", err, probe.calls)
	}
}
