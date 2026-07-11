package assessment

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
)

type assessmentAccessQueryService struct {
	operatorQueryService AssessmentOperatorQueryService
	checker              TesteeAccessChecker
}

func NewAssessmentAccessQueryService(
	operatorQueryService AssessmentOperatorQueryService,
	checker TesteeAccessChecker,
) AssessmentAccessQueryService {
	return &assessmentAccessQueryService{
		operatorQueryService: operatorQueryService,
		checker:              checker,
	}
}

func (s *assessmentAccessQueryService) LoadAccessibleAssessment(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
	assessmentID uint64,
) (*AccessibleAssessmentContext, error) {
	if s.operatorQueryService == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment operator query service is not configured")
	}
	if s.checker == nil {
		return nil, evalerrors.ModuleNotConfigured("testee access checker is not configured")
	}
	result, err := s.operatorQueryService.GetByID(ctx, assessmentID)
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
		return evalerrors.ModuleNotConfigured("testee access checker is not configured")
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
		return dto, evalerrors.ModuleNotConfigured("testee access checker is not configured")
	}
	if dto.TesteeID != nil {
		testeeID := *dto.TesteeID
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

func (s *assessmentAccessQueryService) ScopeTesteeList(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
	testeeID uint64,
) (TesteeListAccessScope, error) {
	result := TesteeListAccessScope{TesteeID: testeeID}
	if s.checker == nil {
		return result, evalerrors.ModuleNotConfigured("testee access checker is not configured")
	}
	if testeeID != 0 {
		if err := s.checker.ValidateTesteeAccess(ctx, orgID, operatorUserID, testeeID); err != nil {
			return result, err
		}
		return result, nil
	}
	scope, err := s.checker.ResolveAccessScope(ctx, orgID, operatorUserID)
	if err != nil {
		return result, err
	}
	if scope != nil && scope.IsAdmin {
		return result, evalerrors.Bind("受试者ID不能为空")
	}
	allowedTesteeIDs, err := s.checker.ListAccessibleTesteeIDs(ctx, orgID, operatorUserID)
	if err != nil {
		return result, err
	}
	result.AccessibleTesteeIDs = allowedTesteeIDs
	result.RestrictToAccessScope = true
	return result, nil
}
