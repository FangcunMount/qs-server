package access

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type service struct {
	operatorReader  actorreadmodel.OperatorReader
	clinicianReader actorreadmodel.ClinicianReader
	relationReader  actorreadmodel.RelationReader
	testeeReader    actorreadmodel.TesteeReader
	snapshot        iambridge.AuthzSnapshotReader
}

// NewTesteeAccessService 创建 testee 访问控制服务。
func NewTesteeAccessService(
	operatorReader actorreadmodel.OperatorReader,
	clinicianReader actorreadmodel.ClinicianReader,
	relationReader actorreadmodel.RelationReader,
	testeeReader actorreadmodel.TesteeReader,
	snapshot iambridge.AuthzSnapshotReader,
) TesteeAccessService {
	return &service{
		operatorReader:  operatorReader,
		clinicianReader: clinicianReader,
		relationReader:  relationReader,
		testeeReader:    testeeReader,
		snapshot:        snapshot,
	}
}

func (s *service) ResolveAccessScope(ctx context.Context, orgID int64, operatorUserID int64) (*TesteeAccessScope, error) {
	if orgID <= 0 {
		return nil, errors.WithCode(code.ErrPermissionDenied, "protected route requires org scope from JWT")
	}
	if operatorUserID <= 0 {
		return nil, errors.WithCode(code.ErrPermissionDenied, "protected route requires user identity from JWT")
	}

	operatorItem, err := s.operatorReader.FindOperatorByUser(ctx, orgID, operatorUserID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrPermissionDenied, "operator not found in current organization")
		}
		return nil, errors.Wrap(err, "failed to find operator")
	}
	if !operatorItem.IsActive {
		return nil, errors.WithCode(code.ErrPermissionDenied, "operator is inactive")
	}
	snap, err := s.resolveAuthzSnapshot(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	if snap.IsQSAdmin() {
		return &TesteeAccessScope{IsAdmin: true}, nil
	}

	clinicianItem, err := s.clinicianReader.FindClinicianByOperator(ctx, orgID, operatorItem.ID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrPermissionDenied, "operator is not bound to clinician")
		}
		return nil, errors.Wrap(err, "failed to find clinician by operator")
	}
	if !clinicianItem.IsActive {
		return nil, errors.WithCode(code.ErrPermissionDenied, "clinician is inactive")
	}

	clinicianID := clinicianItem.ID
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
	if err := ensureAccessIDFromUint64("testee_id", testeeID); err != nil {
		return err
	}
	testeeItem, err := s.testeeReader.GetTestee(ctx, testeeID)
	if err != nil {
		return errors.Wrap(err, "failed to find testee")
	}
	if testeeItem.OrgID != orgID {
		return errors.WithCode(code.ErrPermissionDenied, "testee does not belong to current organization")
	}
	if scope.IsAdmin {
		return nil
	}
	if scope.ClinicianID == nil {
		return errors.WithCode(code.ErrPermissionDenied, "clinician scope is required")
	}
	allowed, err := s.relationReader.HasActiveRelationForTestee(
		ctx,
		orgID,
		*scope.ClinicianID,
		testeeID,
		accessRelationTypesToStrings(domainRelation.AccessGrantRelationTypes()),
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
	ids, err := s.relationReader.ListActiveTesteeIDsByClinician(
		ctx,
		orgID,
		*scope.ClinicianID,
		accessRelationTypesToStrings(domainRelation.AccessGrantRelationTypes()),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list accessible testee ids")
	}

	seen := make(map[uint64]struct{}, len(ids))
	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func ensureAccessIDFromUint64(field string, value uint64) error {
	_, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		return errors.WithCode(code.ErrInvalidArgument, "%s exceeds int64", field)
	}
	return nil
}

func (s *service) resolveAuthzSnapshot(ctx context.Context, orgID int64, operatorUserID int64) (iambridge.AuthzSnapshot, error) {
	if snap, ok := authzapp.FromContext(ctx); ok && snap != nil {
		return snap, nil
	}
	if s.snapshot == nil {
		return nil, errors.WithCode(code.ErrPermissionDenied, "authorization snapshot required")
	}

	snap, err := s.snapshot.LoadAuthzSnapshot(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load authorization snapshot")
	}
	if snap == nil {
		return nil, errors.WithCode(code.ErrPermissionDenied, "authorization snapshot required")
	}
	return snap, nil
}

func accessRelationTypesToStrings(items []domainRelation.RelationType) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, string(item))
	}
	return result
}
