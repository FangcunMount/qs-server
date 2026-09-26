package systemgovernance

import (
	"context"
	"testing"
)

type deliveryResolverFunc func(context.Context, int64, uint64, DeliveryResolutionRequest) (*ActionRunResult, error)

func (f deliveryResolverFunc) ResolveDelivery(ctx context.Context, orgID int64, actorID uint64, req DeliveryResolutionRequest) (*ActionRunResult, error) {
	return f(ctx, orgID, actorID, req)
}

func (f deliveryResolverFunc) GetDeliveryResolution(_ context.Context, _ int64, _ string) (*ActionRunResult, error) {
	return nil, nil
}

func TestDeliveryResolutionRequiresConfirmationIdentityAndBoundResolver(t *testing.T) {
	request := DeliveryResolutionRequest{
		RequestID: "resolution-1", OriginalReplayRequestID: "original-1",
		DeadLetterID: 41, EventID: "event-41", ExpectedDeliveryAttempts: 8,
		Reason: "verified effects", Confirm: true,
	}
	called := 0
	facade := NewFacade(FacadeDeps{DeliveryResolver: deliveryResolverFunc(func(_ context.Context, orgID int64, actorID uint64, req DeliveryResolutionRequest) (*ActionRunResult, error) {
		called++
		if orgID != 7 || actorID != 22 || req.RequestID != "resolution-1" {
			t.Fatalf("scope and identity changed: %d/%d/%+v", orgID, actorID, req)
		}
		return &ActionRunResult{RequestID: req.RequestID, Status: "succeeded"}, nil
	})})
	for _, bad := range []DeliveryResolutionRequest{
		func() DeliveryResolutionRequest { v := request; v.Confirm = false; return v }(),
		func() DeliveryResolutionRequest { v := request; v.RequestID = v.OriginalReplayRequestID; return v }(),
		func() DeliveryResolutionRequest { v := request; v.EventID = ""; return v }(),
		func() DeliveryResolutionRequest { v := request; v.Reason = " "; return v }(),
		func() DeliveryResolutionRequest { v := request; v.ExpectedDeliveryAttempts = 0; return v }(),
	} {
		if _, err := facade.ResolveDelivery(t.Context(), 7, 22, bad); err == nil {
			t.Fatalf("accepted incomplete resolution: %+v", bad)
		}
	}
	if _, err := facade.ResolveDelivery(t.Context(), 0, 22, request); err == nil {
		t.Fatal("missing organization accepted")
	}
	if _, err := facade.ResolveDelivery(t.Context(), 7, 0, request); err == nil {
		t.Fatal("missing actor accepted")
	}
	if called != 0 {
		t.Fatal("invalid request reached resolver")
	}
	result, err := facade.ResolveDelivery(t.Context(), 7, 22, request)
	if err != nil || result.RequestID != request.RequestID || called != 1 {
		t.Fatalf("valid resolution = %+v/%v, called=%d", result, err, called)
	}
	if _, err := NewFacade(FacadeDeps{}).ResolveDelivery(t.Context(), 7, 22, request); err == nil {
		t.Fatal("unbound resolver accepted a production write")
	}
	if _, err := NewFacade(FacadeDeps{}).GetDeliveryResolution(t.Context(), 7, request.RequestID); err == nil {
		t.Fatal("unbound resolver served a receipt")
	}
}
