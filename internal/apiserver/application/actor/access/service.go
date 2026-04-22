package access

import (
	"context"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainOperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type service struct {
	operatorRepo  domainOperator.Repository
	clinicianRepo domainClinician.Repository
	relationRepo  domainRelation.Repository
	testeeRepo    domainTestee.Repository
	snapshot      *iam.AuthzSnapshotLoader
}

// NewTesteeAccessService 创建 testee 访问控制服务。
func NewTesteeAccessService(
	operatorRepo domainOperator.Repository,
	clinicianRepo domainClinician.Repository,
	relationRepo domainRelation.Repository,
	testeeRepo domainTestee.Repository,
	snapshot *iam.AuthzSnapshotLoader,
) TesteeAccessService {
	return &service{
		operatorRepo:  operatorRepo,
		clinicianRepo: clinicianRepo,
		relationRepo:  relationRepo,
		testeeRepo:    testeeRepo,
		snapshot:      snapshot,
	}
}

func (s *service) ResolveAccessScope(ctx context.Context, orgID int64, operatorUserID int64) (*TesteeAccessScope, error) {
	if orgID <= 0 {
		return nil, errors.WithCode(code.ErrPermissionDenied, "protected route requires org scope from JWT")
	}
	if operatorUserID <= 0 {
		return nil, errors.WithCode(code.ErrPermissionDenied, "protected route requires user identity from JWT")
	}

	operatorItem, err := s.operatorRepo.FindByUser(ctx, orgID, operatorUserID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrPermissionDenied, "operator not found in current organization")
		}
		return nil, errors.Wrap(err, "failed to find operator")
	}
	if !operatorItem.IsActive() {
		return nil, errors.WithCode(code.ErrPermissionDenied, "operator is inactive")
	}
	snap, err := s.resolveAuthzSnapshot(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	if snap.IsQSAdmin() {
		return &TesteeAccessScope{IsAdmin: true}, nil
	}

	clinicianItem, err := s.clinicianRepo.FindByOperator(ctx, orgID, operatorItem.ID().Uint64())
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrPermissionDenied, "operator is not bound to clinician")
		}
		return nil, errors.Wrap(err, "failed to find clinician by operator")
	}
	if !clinicianItem.IsActive() {
		return nil, errors.WithCode(code.ErrPermissionDenied, "clinician is inactive")
	}

	clinicianID := clinicianItem.ID().Uint64()
	return &TesteeAccessScope{
		IsAdmin:     false,
		ClinicianID: &clinicianID,
	}, nil
}

func (s *service) ValidateTesteeAccess(ctx context.Context, orgID int64, operatorUserID int64, testeeID uint64) error {
	scope, err := s.ResolveAccessScope(ctx, orgID, operatorUserID)
	if err != nil {
		return err
	}
	targetTesteeID, err := accessTesteeIDFromUint64("testee_id", testeeID)
	if err != nil {
		return err
	}

	testeeItem, err := s.testeeRepo.FindByID(ctx, targetTesteeID)
	if err != nil {
		return errors.Wrap(err, "failed to find testee")
	}
	if testeeItem.OrgID() != orgID {
		return errors.WithCode(code.ErrPermissionDenied, "testee does not belong to current organization")
	}
	if scope.IsAdmin {
		return nil
	}
	if scope.ClinicianID == nil {
		return errors.WithCode(code.ErrPermissionDenied, "clinician scope is required")
	}
	clinicianID, err := accessClinicianIDFromUint64("clinician_id", *scope.ClinicianID)
	if err != nil {
		return err
	}

	allowed, err := s.relationRepo.HasActiveRelationForTestee(
		ctx,
		orgID,
		clinicianID,
		targetTesteeID,
		domainRelation.AccessGrantRelationTypes(),
	)
	if err != nil {
		return errors.Wrap(err, "failed to validate testee relation access")
	}
	if !allowed {
		return errors.WithCode(code.ErrPermissionDenied, "testee is not assigned to current clinician")
	}
	return nil
}

func (s *service) ListAccessibleTesteeIDs(ctx context.Context, orgID int64, operatorUserID int64) ([]uint64, error) {
	scope, err := s.ResolveAccessScope(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	if scope.IsAdmin {
		return nil, nil
	}
	if scope.ClinicianID == nil {
		return []uint64{}, nil
	}
	clinicianID, err := accessClinicianIDFromUint64("clinician_id", *scope.ClinicianID)
	if err != nil {
		return nil, err
	}

	ids, err := s.relationRepo.ListActiveTesteeIDsByClinician(
		ctx,
		orgID,
		clinicianID,
		domainRelation.AccessGrantRelationTypes(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list accessible testee ids")
	}

	seen := make(map[uint64]struct{}, len(ids))
	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		rawID := id.Uint64()
		if _, ok := seen[rawID]; ok {
			continue
		}
		seen[rawID] = struct{}{}
		result = append(result, rawID)
	}
	return result, nil
}

func accessClinicianIDFromUint64(field string, value uint64) (domainClinician.ID, error) {
	id, err := domainTesteeIDFromUint64(field, value)
	if err != nil {
		return 0, err
	}
	return domainClinician.ID(id), nil
}

func accessTesteeIDFromUint64(field string, value uint64) (domainTestee.ID, error) {
	id, err := domainTesteeIDFromUint64(field, value)
	if err != nil {
		return 0, err
	}
	return domainTestee.ID(id), nil
}

func domainTesteeIDFromUint64(field string, value uint64) (domainTestee.ID, error) {
	id, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		return 0, errors.WithCode(code.ErrInvalidArgument, "%s exceeds int64", field)
	}
	return domainTestee.ID(id), nil
}

func (s *service) resolveAuthzSnapshot(ctx context.Context, orgID int64, operatorUserID int64) (*authzapp.Snapshot, error) {
	if snap, ok := authzapp.FromContext(ctx); ok && snap != nil {
		return snap, nil
	}
	if s.snapshot == nil {
		return nil, errors.WithCode(code.ErrPermissionDenied, "authorization snapshot required")
	}

	snap, err := s.snapshot.Load(ctx, strconv.FormatInt(orgID, 10), strconv.FormatInt(operatorUserID, 10))
	if err != nil {
		return nil, errors.Wrap(err, "failed to load authorization snapshot")
	}
	return snap, nil
}
