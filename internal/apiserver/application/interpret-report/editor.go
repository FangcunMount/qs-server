package interpretreport

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/mapper"
	interpretreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpret-report"
	interpretport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpret-report/port"
	errCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Editor 解读报告编辑器
type Editor struct {
	repo   interpretport.InterpretReportRepositoryMongo
	mapper *mapper.InterpretReportMapper
}

// NewEditor 创建解读报告编辑器
func NewEditor(repo interpretport.InterpretReportRepositoryMongo) *Editor {
	return &Editor{
		repo:   repo,
		mapper: mapper.NewInterpretReportMapper(),
	}
}

// 确保实现了接口
var _ interpretport.InterpretReportEditor = (*Editor)(nil)

// UpdateInterpretReport 更新解读报告
func (e *Editor) UpdateInterpretReport(ctx context.Context, reportDTO *dto.InterpretReportDTO) (*dto.InterpretReportDTO, error) {
	// 验证输入参数
	if err := e.validateUpdateInput(reportDTO); err != nil {
		return nil, err
	}

	// 查找现有解读报告
	existingReport, err := e.repo.FindByAnswerSheetId(ctx, reportDTO.AnswerSheetId)
	if err != nil {
		return nil, errors.WithCode(errCode.ErrInterpretReportNotFound, "解读报告不存在: %v", err)
	}

	// 更新解读报告
	e.updateReportFields(existingReport, reportDTO)

	// 保存更新
	if err := e.repo.Update(ctx, existingReport); err != nil {
		return nil, errors.WithCode(errCode.ErrDatabase, "更新解读报告失败: %v", err)
	}

	// 转换为DTO并返回
	return e.mapper.ToDTO(existingReport), nil
}

// updateReportFields 更新解读报告字段
func (e *Editor) updateReportFields(existingReport *interpretreport.InterpretReport, reportDTO *dto.InterpretReportDTO) {
	// 更新基本信息
	if reportDTO.Title != "" && reportDTO.Title != existingReport.GetTitle() {
		existingReport.UpdateTitle(reportDTO.Title)
	}

	if reportDTO.Description != "" && reportDTO.Description != existingReport.GetDescription() {
		existingReport.UpdateDescription(reportDTO.Description)
	}

	// 更新解读项
	if len(reportDTO.InterpretItems) > 0 {
		// 转换DTO解读项为领域对象
		newItems := make([]interpretreport.InterpretItem, len(reportDTO.InterpretItems))
		for i, itemDTO := range reportDTO.InterpretItems {
			newItems[i] = e.mapper.InterpretItemToDomain(itemDTO)
		}
		existingReport.SetInterpretItems(newItems)
	}
}

// validateUpdateInput 验证更新输入参数
func (e *Editor) validateUpdateInput(reportDTO *dto.InterpretReportDTO) error {
	if reportDTO == nil {
		return errors.WithCode(errCode.ErrInvalidArgument, "解读报告信息不能为空")
	}

	if reportDTO.AnswerSheetId == 0 {
		return errors.WithCode(errCode.ErrInvalidArgument, "答卷ID不能为空")
	}

	// 如果提供了解读项，验证其有效性
	if len(reportDTO.InterpretItems) > 0 {
		for i, item := range reportDTO.InterpretItems {
			if item.FactorCode == "" {
				return errors.WithCode(errCode.ErrInvalidArgument, "第%d个解读项的因子代码不能为空", i+1)
			}
			if item.Title == "" {
				return errors.WithCode(errCode.ErrInvalidArgument, "第%d个解读项的标题不能为空", i+1)
			}
			if item.Content == "" {
				return errors.WithCode(errCode.ErrInvalidArgument, "第%d个解读项的内容不能为空", i+1)
			}
		}
	}

	return nil
}
