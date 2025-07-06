package interpretreport

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/mapper"
	interpretport "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/interpret-report/port"
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
	v1 "github.com/yshujie/questionnaire-scale/pkg/meta/v1"
)

// Creator 解读报告创建器
type Creator struct {
	repo   interpretport.InterpretReportRepositoryMongo
	mapper *mapper.InterpretReportMapper
}

// NewCreator 创建解读报告创建器
func NewCreator(repo interpretport.InterpretReportRepositoryMongo) *Creator {
	return &Creator{
		repo:   repo,
		mapper: mapper.NewInterpretReportMapper(),
	}
}

// 确保实现了接口
var _ interpretport.InterpretReportCreator = (*Creator)(nil)

// CreateInterpretReport 创建解读报告
func (c *Creator) CreateInterpretReport(ctx context.Context, reportDTO *dto.InterpretReportDTO) (*dto.InterpretReportDTO, error) {
	// 验证输入参数
	if err := c.validateCreateInput(reportDTO); err != nil {
		return nil, err
	}

	// 检查是否已存在相同答卷的解读报告
	exists, err := c.repo.ExistsByAnswerSheetId(ctx, reportDTO.AnswerSheetId)
	if err != nil {
		return nil, errors.WithCode(errCode.ErrDatabase, "检查解读报告是否存在失败: %v", err)
	}
	if exists {
		return nil, errors.WithCode(errCode.ErrInterpretReportAlreadyExists, "该答卷的解读报告已存在")
	}

	// 转换DTO为领域对象
	report := c.mapper.ToDomain(reportDTO)
	if report == nil {
		return nil, errors.WithCode(errCode.ErrInterpretReportInvalid, "无法创建解读报告领域对象")
	}

	// 设置ID（如果没有的话）
	if report.GetID().Value() == 0 {
		report.SetID(v1.NewID(0)) // 让数据库自动生成ID
	}

	// 保存到数据库
	if err := c.repo.Create(ctx, report); err != nil {
		return nil, errors.WithCode(errCode.ErrDatabase, "保存解读报告失败: %v", err)
	}

	// 转换为DTO并返回
	return c.mapper.ToDTO(report), nil
}

// validateCreateInput 验证创建输入参数
func (c *Creator) validateCreateInput(reportDTO *dto.InterpretReportDTO) error {
	if reportDTO == nil {
		return errors.WithCode(errCode.ErrInvalidArgument, "解读报告信息不能为空")
	}

	if reportDTO.AnswerSheetId == 0 {
		return errors.WithCode(errCode.ErrInvalidArgument, "答卷ID不能为空")
	}

	if reportDTO.MedicalScaleCode == "" {
		return errors.WithCode(errCode.ErrInvalidArgument, "医学量表代码不能为空")
	}

	if reportDTO.Title == "" {
		return errors.WithCode(errCode.ErrInvalidArgument, "解读报告标题不能为空")
	}

	// 验证解读项
	if len(reportDTO.InterpretItems) == 0 {
		return errors.WithCode(errCode.ErrInvalidArgument, "解读项不能为空")
	}

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

	return nil
}
