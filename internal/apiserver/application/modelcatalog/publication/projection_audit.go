package publication

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

const projectionDriftCode = "payload.projection.drift"

// AuditSnapshotProjection replays compatibility payload projection and fails when
// the result is not deterministic (MC-R017 batch 2).
func AuditSnapshotProjection(
	ctx context.Context,
	model *domain.AssessmentModel,
	handler definition.Handler,
	snapshot *port.AssessmentSnapshot,
) []domain.DomainValidationIssue {
	if model == nil || model.DefinitionV2 == nil || snapshot == nil {
		return nil
	}
	replay, err := handler.BuildSnapshotPayload(ctx, model)
	if err != nil {
		return []domain.DomainValidationIssue{{
			Field:   "definition",
			Code:    "payload.projection.replay_failed",
			Message: fmt.Sprintf("replay compatibility payload: %v", err),
			Level:   domain.ValidationLevelError,
		}}
	}
	replayHash := modeldefinition.PayloadProjectionHash(replay.Payload)
	snapshotHash := modeldefinition.PayloadProjectionHash(snapshot.Payload)
	if replayHash != snapshotHash {
		return []domain.DomainValidationIssue{{
			Field:   "definition",
			Code:    projectionDriftCode,
			Message: "compatibility payload projection is not deterministic",
			Level:   domain.ValidationLevelError,
		}}
	}
	return nil
}
