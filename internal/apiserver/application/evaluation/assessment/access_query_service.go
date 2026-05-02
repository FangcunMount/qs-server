package assessment

import (
	"context"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type assessmentAccessQueryService struct {
	managementService AssessmentManagementService
	checker           TesteeAccessChecker
}

func NewAssessmentAccessQueryService(
	managementService AssessmentManagementService,
	checker TesteeAccessChecker,
) AssessmentAccessQueryService {
	return &assessmentAccessQueryService{
		managementService: managementService,
		checker:           checker,
	}
}

func (s *assessmentAccessQueryService) LoadAccessibleAssessment(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
	assessmentID uint64,
) (*AccessibleAssessmentContext, error) {
	if s.managementService == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "assessment management service is not configured")
	}
	if s.checker == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "testee access checker is not configured")
	}
	result, err := s.managementService.GetByID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	if err := s.checker.ValidateTesteeAccess(ctx, orgID, operatorUserID, result.TesteeID); err != nil {
		return nil, err
	}
	return &AccessibleAssessmentContext{
		AssessmentID: assessmentID,
		Assessment:   result,
	}, nil
}

func (s *assessmentAccessQueryService) ValidateTesteeAccess(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
	testeeID uint64,
) error {
	if s.checker == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "testee access checker is not configured")
	}
	return s.checker.ValidateTesteeAccess(ctx, orgID, operatorUserID, testeeID)
}

func (s *assessmentAccessQueryService) ScopeListAssessments(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
	dto ListAssessmentsDTO,
) (ListAssessmentsDTO, error) {
	if s.checker == nil {
		return dto, errors.WithCode(code.ErrModuleInitializationFailed, "testee access checker is not configured")
	}
	if dto.TesteeID != nil {
		testeeID := *dto.TesteeID
		if err := s.checker.ValidateTesteeAccess(ctx, orgID, operatorUserID, testeeID); err != nil {
			return dto, err
		}
		return dto, nil
	}
	if dto.Conditions != nil && dto.Conditions["testee_id"] != "" {
		testeeID, err := parseUintCondition(dto.Conditions["testee_id"])
		if err != nil {
			return dto, errors.WithCode(code.ErrInvalidArgument, "无效的受试者ID")
		}
		dto.TesteeID = &testeeID
		if err := s.checker.ValidateTesteeAccess(ctx, orgID, operatorUserID, testeeID); err != nil {
			return dto, err
		}
		return dto, nil
	}
	scope, err := s.checker.ResolveAccessScope(ctx, orgID, operatorUserID)
	if err != nil {
		return dto, err
	}
	if scope != nil && scope.IsAdmin {
		return dto, nil
	}
	allowedTesteeIDs, err := s.checker.ListAccessibleTesteeIDs(ctx, orgID, operatorUserID)
	if err != nil {
		return dto, err
	}
	dto.AccessibleTesteeIDs = allowedTesteeIDs
	dto.RestrictToAccessScope = true
	return dto, nil
}

func (s *assessmentAccessQueryService) ScopeFactorTrend(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
	dto GetFactorTrendDTO,
) (GetFactorTrendDTO, error) {
	if dto.Limit <= 0 {
		dto.Limit = 10
	}
	if err := s.ValidateTesteeAccess(ctx, orgID, operatorUserID, dto.TesteeID); err != nil {
		return dto, err
	}
	return dto, nil
}

func (s *assessmentAccessQueryService) ScopeListReports(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
	dto ListReportsDTO,
) (ListReportsDTO, error) {
	if s.checker == nil {
		return dto, errors.WithCode(code.ErrModuleInitializationFailed, "testee access checker is not configured")
	}
	if dto.TesteeID != 0 {
		if err := s.checker.ValidateTesteeAccess(ctx, orgID, operatorUserID, dto.TesteeID); err != nil {
			return dto, err
		}
		return dto, nil
	}
	scope, err := s.checker.ResolveAccessScope(ctx, orgID, operatorUserID)
	if err != nil {
		return dto, err
	}
	if scope != nil && scope.IsAdmin {
		return dto, errors.WithCode(code.ErrBind, "受试者ID不能为空")
	}
	allowedTesteeIDs, err := s.checker.ListAccessibleTesteeIDs(ctx, orgID, operatorUserID)
	if err != nil {
		return dto, err
	}
	dto.AccessibleTesteeIDs = allowedTesteeIDs
	dto.RestrictToAccessScope = true
	return dto, nil
}

func parseUintCondition(raw string) (uint64, error) {
	return strconv.ParseUint(raw, 10, 64)
}
