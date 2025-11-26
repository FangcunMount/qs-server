package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 问卷查询服务实现
// 行为者：所有用户
type queryService struct {
	repo questionnaire.Repository
}

// NewQueryService 创建问卷查询服务
func NewQueryService(
	repo questionnaire.Repository,
) QuestionnaireQueryService {
	return &queryService{
		repo: repo,
	}
}

// GetByCode 根据编码获取问卷
func (s *queryService) GetByCode(ctx context.Context, code string) (*QuestionnaireResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}

	// 2. 从 MongoDB 获取问卷
	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	return toQuestionnaireResult(q), nil
}

// List 查询问卷列表
func (s *queryService) List(ctx context.Context, dto ListQuestionnairesDTO) (*QuestionnaireListResult, error) {
	// 1. 验证分页参数
	if dto.Page <= 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量不能超过100")
	}

	// 2. 获取问卷列表
	questionnaires, err := s.repo.FindList(ctx, dto.Page, dto.PageSize, dto.Conditions)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷列表失败")
	}

	// 3. 获取总数
	total, err := s.repo.CountWithConditions(ctx, dto.Conditions)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷总数失败")
	}

	return toQuestionnaireListResult(questionnaires, total), nil
}

// GetPublishedByCode 获取已发布的问卷
func (s *queryService) GetPublishedByCode(ctx context.Context, code string) (*QuestionnaireResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}

	// 2. 获取问卷
	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 检查问卷状态
	if !q.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷未发布")
	}

	return toQuestionnaireResult(q), nil
}

// ListPublished 查询已发布问卷列表
func (s *queryService) ListPublished(ctx context.Context, dto ListQuestionnairesDTO) (*QuestionnaireListResult, error) {
	// 1. 验证分页参数
	if dto.Page <= 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量不能超过100")
	}

	// 2. 添加状态过滤条件
	if dto.Conditions == nil {
		dto.Conditions = make(map[string]string)
	}
	dto.Conditions["status"] = string(questionnaire.STATUS_PUBLISHED)

	// 3. 获取问卷列表
	questionnaires, err := s.repo.FindList(ctx, dto.Page, dto.PageSize, dto.Conditions)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷列表失败")
	}

	// 4. 获取总数
	total, err := s.repo.CountWithConditions(ctx, dto.Conditions)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷总数失败")
	}

	return toQuestionnaireListResult(questionnaires, total), nil
}
