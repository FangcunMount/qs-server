package clinician

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

type relationshipService struct {
	relationRepo  domainRelation.Repository
	clinicianRepo domainClinician.Repository
	testeeRepo    domainTestee.Repository
	uow           *mysql.UnitOfWork
}

// NewRelationshipService 创建从业者关系服务。
func NewRelationshipService(
	relationRepo domainRelation.Repository,
	clinicianRepo domainClinician.Repository,
	testeeRepo domainTestee.Repository,
	uow *mysql.UnitOfWork,
) ClinicianRelationshipService {
	return &relationshipService{
		relationRepo:  relationRepo,
		clinicianRepo: clinicianRepo,
		testeeRepo:    testeeRepo,
		uow:           uow,
	}
}

func (s *relationshipService) AssignTestee(ctx context.Context, dto AssignTesteeDTO) (*RelationResult, error) {
	var result *domainRelation.ClinicianTesteeRelation

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		relationshipType := domainRelation.RelationType(dto.RelationType)
		if relationshipType == "" {
			relationshipType = domainRelation.RelationTypePrimary
		}

		sourceType := domainRelation.SourceType(dto.SourceType)
		if sourceType == "" {
			sourceType = domainRelation.SourceTypeManual
		}

		clinicianItem, err := s.clinicianRepo.FindByID(txCtx, domainClinician.ID(dto.ClinicianID))
		if err != nil {
			return errors.Wrap(err, "failed to find clinician")
		}
		if clinicianItem.OrgID() != dto.OrgID {
			return errors.WithCode(code.ErrInvalidArgument, "clinician does not belong to the requested organization")
		}

		testeeItem, err := s.testeeRepo.FindByID(txCtx, domainTestee.ID(dto.TesteeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}
		if testeeItem.OrgID() != dto.OrgID {
			return errors.WithCode(code.ErrInvalidArgument, "testee does not belong to the requested organization")
		}

		result, err = s.relationRepo.FindActive(
			txCtx,
			dto.OrgID,
			domainClinician.ID(dto.ClinicianID),
			domainTestee.ID(dto.TesteeID),
			relationshipType,
		)
		if err == nil {
			return nil
		}
		if !errors.IsCode(err, code.ErrUserNotFound) {
			return errors.Wrap(err, "failed to find active relation")
		}

		result = domainRelation.NewClinicianTesteeRelation(
			dto.OrgID,
			domainClinician.ID(dto.ClinicianID),
			domainTestee.ID(dto.TesteeID),
			relationshipType,
			sourceType,
			dto.SourceID,
			true,
			time.Now(),
			nil,
		)
		if err := s.relationRepo.Save(txCtx, result); err != nil {
			return errors.Wrap(err, "failed to save relation")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return toRelationResult(result), nil
}

func (s *relationshipService) ListAssignedTestees(ctx context.Context, dto ListAssignedTesteeDTO) (*AssignedTesteeListResult, error) {
	relations, err := s.relationRepo.ListActiveByClinician(
		ctx,
		dto.OrgID,
		domainClinician.ID(dto.ClinicianID),
		dto.Offset,
		dto.Limit,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list relations")
	}

	totalCount, err := s.relationRepo.CountActiveByClinician(ctx, dto.OrgID, domainClinician.ID(dto.ClinicianID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to count relations")
	}

	items := make([]*AssignedTesteeResult, 0, len(relations))
	for _, item := range relations {
		testeeItem, err := s.testeeRepo.FindByID(ctx, item.TesteeID())
		if err != nil {
			if errors.IsCode(err, code.ErrUserNotFound) {
				continue
			}
			return nil, errors.Wrap(err, "failed to find assigned testee")
		}
		items = append(items, toAssignedTesteeResult(testeeItem))
	}

	return &AssignedTesteeListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}
