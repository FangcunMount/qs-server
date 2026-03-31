package access

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainOperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type service struct {
	operatorRepo  domainOperator.Repository
	clinicianRepo domainClinician.Repository
	relationRepo  domainRelation.Repository
	testeeRepo    domainTestee.Repository
}

// NewTesteeAccessService 创建 testee 访问控制服务。
func NewTesteeAccessService(
	operatorRepo domainOperator.Repository,
	clinicianRepo domainClinician.Repository,
	relationRepo domainRelation.Repository,
	testeeRepo domainTestee.Repository,
) TesteeAccessService {
	return &service{
		operatorRepo:  operatorRepo,
		clinicianRepo: clinicianRepo,
		relationRepo:  relationRepo,
		testeeRepo:    testeeRepo,
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
	if operatorItem.HasRole(domainOperator.RoleQSAdmin) {
		return &TesteeAccessScope{IsAdmin: true}, nil
	}

	clinicianItem, err := s.clinicianRepo.FindByOperator(ctx, orgID, uint64(operatorItem.ID()))
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

	testeeItem, err := s.testeeRepo.FindByID(ctx, domainTestee.ID(testeeID))
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

	allowed, err := s.relationRepo.HasActiveRelationForTestee(
		ctx,
		orgID,
		domainClinician.ID(*scope.ClinicianID),
		domainTestee.ID(testeeID),
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

	ids, err := s.relationRepo.ListActiveTesteeIDsByClinician(ctx, orgID, domainClinician.ID(*scope.ClinicianID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to list accessible testee ids")
	}

	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		result = append(result, id.Uint64())
	}
	return result, nil
}
