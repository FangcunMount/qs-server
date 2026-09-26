package systemgovernance

import (
	"context"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"gorm.io/gorm"
)

type reportGeneratedDeliveryResolver struct {
	store  *ActionAuditStore
	verify DeliveryResolutionVerifier
}

// NewReportGeneratedDeliveryResolver binds only the report-generated verifier.
// Missing databases or report evidence leave the governance action unavailable.
func NewReportGeneratedDeliveryResolver(db *gorm.DB, reports ReportGeneratedResolutionReader) app.DeliveryResolver {
	if db == nil || reports == nil {
		return nil
	}
	return &reportGeneratedDeliveryResolver{
		store: NewActionAuditStore(db), verify: NewReportGeneratedResolutionVerifier(reports),
	}
}

func (r *reportGeneratedDeliveryResolver) ResolveDelivery(ctx context.Context, orgID int64, actorUserID uint64, req app.DeliveryResolutionRequest) (*app.ActionRunResult, error) {
	return r.store.ResolveDeliveryResult(ctx, DeliveryResolutionRequest{
		OrgID: orgID, ActorUserID: actorUserID, RequestID: req.RequestID,
		OriginalReplayRequestID: req.OriginalReplayRequestID,
		DeadLetterID:            req.DeadLetterID, EventID: req.EventID,
		ExpectedDeliveryAttempts: req.ExpectedDeliveryAttempts, Reason: req.Reason,
	}, r.verify)
}

func (r *reportGeneratedDeliveryResolver) GetDeliveryResolution(ctx context.Context, orgID int64, requestID string) (*app.ActionRunResult, error) {
	return r.store.LoadDeliveryResolution(ctx, orgID, requestID)
}

var _ app.DeliveryResolver = (*reportGeneratedDeliveryResolver)(nil)
