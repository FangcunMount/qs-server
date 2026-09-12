package clinician

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"math"
)

// operatorRelationFilter applies only to an explicitly assembled backstage service.
// Internal relationship workflows never infer their authority from an Operator session.
func (s *relationshipService) operatorRelationFilter(ctx context.Context, filter actorreadmodel.RelationFilter, action string) (actorreadmodel.RelationFilter, error) {
	if !s.operatorOnly {
		return filter, nil
	}
	user := actorctx.GrantingUserID(ctx)
	if s.operatorScope == nil || filter.OrgID <= 0 || actorctx.OperatorOrgID(ctx) != filter.OrgID || user == 0 || user > math.MaxInt64 {
		return actorreadmodel.RelationFilter{}, errors.WithCode(code.ErrPermissionDenied, "valid operator company scope required")
	}
	const resource = "qs:actor:collection:testees"
	if err := appauthz.RequirePermission(ctx, resource, action); err != nil {
		return actorreadmodel.RelationFilter{}, err
	}
	scope, err := s.operatorScope.ResolveStoreRange(ctx, filter.OrgID, int64(user), resource, action)
	if err != nil {
		return actorreadmodel.RelationFilter{}, err
	}
	filter.RestrictToStoreScope = true
	filter.AllAssignedStores = scope.AllStores
	filter.AllowedStoreIDs = append([]uint64(nil), scope.StoreIDs...)
	return filter, nil
}

// NewOperatorRelationshipService is the explicit backstage boundary. Internal
// enrollment and care-context callers retain their separate service instance.
func NewOperatorRelationshipService(relations domainRelation.Repository, clinicians domainClinician.Repository, testees domainTestee.Repository, tx apptransaction.Runner, reader actorreadmodel.ReadModel, summaries actorreadmodel.AssessmentSummaryReader, scope SummaryScope) ClinicianRelationshipService {
	s := NewRelationshipServiceWithAssessmentSummary(relations, clinicians, testees, tx, reader, summaries, scope).(*relationshipService)
	s.operatorOnly = true
	s.operatorScope = scope
	return s
}

// Called inside the relationship transaction; the row lock serializes with a
// Testee store transfer until the relationship write commits.
func (s *relationshipService) authorizeRelationWrite(ctx context.Context, orgID int64, id uint64) error {
	if !s.operatorOnly {
		return nil
	}
	filter, err := s.authorizeRelationMutation(ctx, orgID)
	if err != nil {
		return err
	}
	target, err := testeeIDFromUint64("testee_id", id)
	if err != nil {
		return err
	}
	repo, ok := s.testeeRepo.(domainTestee.LockedRepository)
	if !ok {
		return errors.WithCode(code.ErrInternalServerError, "transactional testee locking unavailable")
	}
	item, err := repo.FindByIDForUpdate(ctx, orgID, target)
	if err != nil {
		return err
	}
	allowed := appauthz.StoreRange{AllStores: filter.AllAssignedStores, StoreIDs: filter.AllowedStoreIDs}
	if item == nil || item.OrgID() != orgID || !allowed.Contains(item.StoreID()) {
		return errors.WithCode(code.ErrPermissionDenied, "testee outside operator store scope")
	}
	return nil
}

func (s *relationshipService) authorizeRelationMutation(ctx context.Context, orgID int64) (actorreadmodel.RelationFilter, error) {
	snapshot, ok := appauthz.FromContext(ctx)
	if !ok || snapshot == nil || !snapshot.IsQSAdmin() {
		return actorreadmodel.RelationFilter{}, errors.WithCode(code.ErrPermissionDenied, "company administrator permission required")
	}
	return s.operatorRelationFilter(ctx, actorreadmodel.RelationFilter{OrgID: orgID}, "update")
}
