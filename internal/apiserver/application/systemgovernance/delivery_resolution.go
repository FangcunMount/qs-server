package systemgovernance

import (
	"context"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// DeliveryResolver owns the audit and dead-letter update in one transaction.
// It is separate from ActionExecutor.Run, whose claim and completion are two
// transactions and therefore unsuitable for settling an uncertain delivery.
type DeliveryResolver interface {
	ResolveDelivery(context.Context, int64, uint64, DeliveryResolutionRequest) (*ActionRunResult, error)
	GetDeliveryResolution(context.Context, int64, string) (*ActionRunResult, error)
}

func (f *facade) ResolveDelivery(ctx context.Context, orgID int64, actorUserID uint64, req DeliveryResolutionRequest) (*ActionRunResult, error) {
	if f == nil || f.deps.DeliveryResolver == nil {
		return nil, errActionsUnavailable()
	}
	if !req.Confirm || orgID <= 0 || actorUserID == 0 || req.DeadLetterID == 0 ||
		req.ExpectedDeliveryAttempts < 1 || strings.TrimSpace(req.RequestID) == "" ||
		strings.TrimSpace(req.OriginalReplayRequestID) == "" || strings.TrimSpace(req.EventID) == "" ||
		strings.TrimSpace(req.Reason) == "" || req.RequestID == req.OriginalReplayRequestID {
		return nil, errors.WithCode(code.ErrInvalidArgument, "confirmed resolution and complete physical delivery identity are required")
	}
	return f.deps.DeliveryResolver.ResolveDelivery(ctx, orgID, actorUserID, req)
}

func (f *facade) GetDeliveryResolution(ctx context.Context, orgID int64, requestID string) (*ActionRunResult, error) {
	if f == nil || f.deps.DeliveryResolver == nil {
		return nil, errActionsUnavailable()
	}
	if orgID <= 0 || strings.TrimSpace(requestID) == "" {
		return nil, errors.WithCode(code.ErrInvalidArgument, "resolution request_id is required")
	}
	return f.deps.DeliveryResolver.GetDeliveryResolution(ctx, orgID, requestID)
}
