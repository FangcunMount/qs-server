package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// managementService 答卷管理服务实现
// 行为者：管理员
type managementService struct {
	repo answersheet.Repository
}

// NewManagementService 创建答卷管理服务
func NewManagementService(
	repo answersheet.Repository,
) AnswerSheetManagementService {
	return &managementService{
		repo: repo,
	}
}

// GetByID 根据ID获取答卷详情
func (s *managementService) GetByID(ctx context.Context, id uint64) (*AnswerSheetResult, error) {
	// 1. 验证输入参数
	if id == 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "答卷ID不能为空")
	}

	// 2. 获取答卷
	sheet, err := s.repo.FindByID(ctx, meta.ID(id))
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetNotFound, "获取答卷失败")
	}

	return toAnswerSheetResult(sheet), nil
}

// List 查询答卷列表
func (s *managementService) List(ctx context.Context, dto ListAnswerSheetsDTO) (*AnswerSheetSummaryListResult, error) {
	// 1. 验证分页参数
	if dto.Page <= 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "每页数量不能超过100")
	}

	// 构建查询条件
	conditions := make(map[string]interface{})
	if dto.QuestionnaireCode != "" {
		conditions["questionnaire_code"] = dto.QuestionnaireCode
	}
	if dto.FillerID != nil && *dto.FillerID > 0 {
		conditions["filler_id"] = *dto.FillerID
	}
	if dto.StartTime != nil {
		conditions["start_time"] = dto.StartTime
	}
	if dto.EndTime != nil {
		conditions["end_time"] = dto.EndTime
	}
	// 合并其他条件
	for k, v := range dto.Conditions {
		conditions[k] = v
	}

	// 3. 查询答卷摘要列表
	sheets, err := s.repo.FindSummaryListByQuestionnaire(ctx, dto.QuestionnaireCode, dto.Page, dto.PageSize)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询答卷列表失败")
	}

	// 4. 获取总数
	total, err := s.repo.CountWithConditions(ctx, conditions)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取答卷总数失败")
	}

	return toSummaryListResult(sheets, total), nil
}

// Delete 删除答卷
func (s *managementService) Delete(ctx context.Context, id uint64) error {
	// 1. 验证输入参数
	if id == 0 {
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "答卷ID不能为空")
	}

	// 2. 检查答卷是否存在
	_, err := s.repo.FindByID(ctx, meta.ID(id))
	if err != nil {
		return errors.WrapC(err, errorCode.ErrAnswerSheetNotFound, "答卷不存在")
	}

	// 3. 删除答卷
	if err := s.repo.Delete(ctx, meta.ID(id)); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "删除答卷失败")
	}

	return nil
}
