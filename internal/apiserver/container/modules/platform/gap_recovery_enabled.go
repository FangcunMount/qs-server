package platform

import (
	"context"
	stderrors "errors"

	apperrors "github.com/FangcunMount/component-base/pkg/errors"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"gorm.io/gorm"
)

type gapRecoveryStore struct {
	ledger *mysqlstandard.GapRecoveryLedger
}

func buildGapRecoveryStore(db *gorm.DB) systemgov.GapRecoveryStore {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil
	}
	ledger, err := mysqlstandard.NewGapRecoveryLedger(sqlDB)
	if err != nil {
		return nil
	}
	return gapRecoveryStore{ledger: ledger}
}

func gapRecoveryInput(orgID int64, actorID uint64, req systemgov.GapRecoveryRequest) mysqlstandard.GapRecoveryRequest {
	return mysqlstandard.GapRecoveryRequest{
		OrgID: orgID, ActorID: actorID, RequestID: req.RequestID,
		AssessmentID: req.AssessmentID, EventID: req.EventID,
		ExpectedVersion: req.ExpectedVersion, Reason: req.Reason,
		SubmittedBefore: req.SubmittedBefore,
	}
}

func gapRecoveryDecision(result mysqlstandard.GapRecoveryResult) *systemgov.GapRecoveryDecision {
	return &systemgov.GapRecoveryDecision{
		Authorized: result.Authorized, Code: result.Code,
		OutboxVersionBefore: result.OutboxVersionBefore,
		OutboxVersionAfter:  result.OutboxVersionAfter,
	}
}

func gapRecoveryError(err error) error {
	if stderrors.Is(err, mysqlstandard.ErrGapRecoveryInputConflict) {
		return apperrors.WithCode(code.ErrConflict, "recovery request ID has different original input")
	}
	return err
}

func (s gapRecoveryStore) AuthorizeGapRecovery(ctx context.Context, orgID int64, actorID uint64, req systemgov.GapRecoveryRequest) (*systemgov.GapRecoveryDecision, error) {
	result, err := s.ledger.Authorize(ctx, gapRecoveryInput(orgID, actorID, req))
	if err != nil {
		return nil, gapRecoveryError(err)
	}
	return gapRecoveryDecision(result), nil
}

func (s gapRecoveryStore) ResolveGapRecovery(ctx context.Context, orgID int64, actorID uint64, req systemgov.GapRecoveryRequest) (*systemgov.GapRecoveryDecision, bool, error) {
	result, found, err := s.ledger.Resolve(ctx, gapRecoveryInput(orgID, actorID, req))
	if err != nil || !found {
		return nil, found, gapRecoveryError(err)
	}
	return gapRecoveryDecision(result), true, nil
}

func (s gapRecoveryStore) ReadGapRecoverySummary(ctx context.Context, orgID int64) (systemgov.GapRecoverySummary, error) {
	result, err := s.ledger.ReadSummary(ctx, orgID)
	if err != nil {
		return systemgov.GapRecoverySummary{}, err
	}
	return systemgov.GapRecoverySummary{
		Authorized: result.Authorized, Denied: result.Denied, WaitingRelay: result.WaitingRelay,
	}, nil
}
