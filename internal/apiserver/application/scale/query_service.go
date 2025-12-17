package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 量表查询服务实现
// 行为者：所有用户
type queryService struct {
	repo scale.Repository
}

// NewQueryService 创建量表查询服务
func NewQueryService(repo scale.Repository) ScaleQueryService {
	return &queryService{
		repo: repo,
	}
}

// GetByCode 根据编码获取量表
func (s *queryService) GetByCode(ctx context.Context, code string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 从仓储获取量表
	m, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	return toScaleResult(m), nil
}

// GetByQuestionnaireCode 根据问卷编码获取量表
func (s *queryService) GetByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if questionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷编码不能为空")
	}

	// 2. 从仓储获取量表
	m, err := s.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	return toScaleResult(m), nil
}

// List 查询量表摘要列表
func (s *queryService) List(ctx context.Context, dto ListScalesDTO) (*ScaleSummaryListResult, error) {
	// 1. 验证分页参数
	if dto.Page <= 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "每页数量不能超过100")
	}

	// 2. 获取量表摘要列表
	items, err := s.repo.FindSummaryList(ctx, dto.Page, dto.PageSize, dto.Conditions)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取量表列表失败")
	}

	// 3. 获取总数
	total, err := s.repo.CountWithConditions(ctx, dto.Conditions)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取量表总数失败")
	}

	return toSummaryListResult(items, total), nil
}

// GetPublishedByCode 获取已发布的量表
func (s *queryService) GetPublishedByCode(ctx context.Context, code string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 检查量表状态
	if !m.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表未发布")
	}

	return toScaleResult(m), nil
}

// ListPublished 查询已发布量表摘要列表
func (s *queryService) ListPublished(ctx context.Context, dto ListScalesDTO) (*ScaleSummaryListResult, error) {
	// 1. 验证分页参数
	if dto.Page <= 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "每页数量不能超过100")
	}

	// 2. 添加状态过滤条件
	conditions := dto.Conditions
	if conditions == nil {
		conditions = make(map[string]string)
	}
	conditions["status"] = scale.StatusPublished.String()

	// 3. 获取量表摘要列表
	items, err := s.repo.FindSummaryList(ctx, dto.Page, dto.PageSize, conditions)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取量表列表失败")
	}

	// 4. 获取总数
	total, err := s.repo.CountWithConditions(ctx, conditions)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取量表总数失败")
	}

	return toSummaryListResult(items, total), nil
}

// GetFactors 获取量表的因子列表
func (s *queryService) GetFactors(ctx context.Context, scaleCode string) ([]FactorResult, error) {
	// 1. 验证输入参数
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 从仓储获取量表
	m, err := s.repo.FindByCode(ctx, scaleCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 转换因子列表
	factors := m.GetFactors()
	result := make([]FactorResult, 0, len(factors))
	for _, factor := range factors {
		result = append(result, toFactorResult(factor))
	}

	return result, nil
}
