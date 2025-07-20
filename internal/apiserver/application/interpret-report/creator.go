package interpretreport

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/mapper"
	interpretport "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/interpret-report/port"
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
	"github.com/yshujie/questionnaire-scale/pkg/log"
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
	log.Infof("开始创建解读报告，答卷ID: %d, 医学量表代码: %s", reportDTO.AnswerSheetId, reportDTO.MedicalScaleCode)

	// 验证输入参数
	if err := c.validateCreateInput(reportDTO); err != nil {
		log.Errorf("验证输入参数失败: %v", err)
		return nil, err
	}

	// 检查是否已存在相同答卷的解读报告
	exists, err := c.repo.ExistsByAnswerSheetId(ctx, reportDTO.AnswerSheetId)
	if err != nil {
		log.Errorf("检查解读报告是否存在失败，答卷ID: %d, 错误: %v", reportDTO.AnswerSheetId, err)
		return nil, errors.WithCode(errCode.ErrDatabase, "检查解读报告是否存在失败: %v", err)
	}
	if exists {
		log.Warnf("该答卷的解读报告已存在，答卷ID: %d", reportDTO.AnswerSheetId)
		return nil, errors.WithCode(errCode.ErrInterpretReportAlreadyExists, "该答卷的解读报告已存在")
	}

	log.Infof("转换DTO为领域对象，解读项数量: %d", len(reportDTO.InterpretItems))

	// 转换DTO为领域对象
	report := c.mapper.ToDomain(reportDTO)
	if report == nil {
		log.Errorf("无法创建解读报告领域对象")
		return nil, errors.WithCode(errCode.ErrInterpretReportInvalid, "无法创建解读报告领域对象")
	}

	log.Infof("领域对象创建成功，ID: %d", report.GetID().Value())

	// 设置ID（如果没有的话）
	if report.GetID().Value() == 0 {
		report.SetID(v1.NewID(0)) // 让数据库自动生成ID
		log.Infof("设置自动生成的ID: %d", report.GetID().Value())
	}

	log.Infof("开始保存到数据库")

	// 保存到数据库
	if err := c.repo.Create(ctx, report); err != nil {
		log.Errorf("保存解读报告到数据库失败，答卷ID: %d, 错误: %v", reportDTO.AnswerSheetId, err)
		return nil, errors.WithCode(errCode.ErrDatabase, "保存解读报告失败: %v", err)
	}

	log.Infof("数据库保存成功，开始转换为DTO")

	// 转换为DTO并返回
	resultDTO := c.mapper.ToDTO(report)
	if resultDTO == nil {
		log.Errorf("转换为DTO失败")
		return nil, errors.WithCode(errCode.ErrInterpretReportInvalid, "转换为DTO失败")
	}

	log.Infof("解读报告创建成功，ID: %d", resultDTO.ID)
	return resultDTO, nil
}

// validateCreateInput 验证创建输入参数
func (c *Creator) validateCreateInput(reportDTO *dto.InterpretReportDTO) error {
	log.Infof("开始验证解读报告输入参数")

	if reportDTO == nil {
		log.Errorf("解读报告信息为空")
		return errors.WithCode(errCode.ErrInvalidArgument, "解读报告信息不能为空")
	}

	log.Infof("验证答卷ID: %d", reportDTO.AnswerSheetId)
	if reportDTO.AnswerSheetId == 0 {
		log.Errorf("答卷ID为空")
		return errors.WithCode(errCode.ErrInvalidArgument, "答卷ID不能为空")
	}

	log.Infof("验证医学量表代码: %s", reportDTO.MedicalScaleCode)
	if reportDTO.MedicalScaleCode == "" {
		log.Errorf("医学量表代码为空")
		return errors.WithCode(errCode.ErrInvalidArgument, "医学量表代码不能为空")
	}

	log.Infof("验证标题: %s", reportDTO.Title)
	if reportDTO.Title == "" {
		log.Errorf("解读报告标题为空")
		return errors.WithCode(errCode.ErrInvalidArgument, "解读报告标题不能为空")
	}

	// 验证解读项
	log.Infof("验证解读项，数量: %d", len(reportDTO.InterpretItems))
	if len(reportDTO.InterpretItems) == 0 {
		log.Errorf("解读项为空")
		return errors.WithCode(errCode.ErrInvalidArgument, "解读项不能为空")
	}

	for i, item := range reportDTO.InterpretItems {
		log.Infof("验证第%d个解读项，因子代码: %s, 标题: %s", i+1, item.FactorCode, item.Title)

		if item.FactorCode == "" {
			log.Errorf("第%d个解读项的因子代码为空", i+1)
			return errors.WithCode(errCode.ErrInvalidArgument, "第%d个解读项的因子代码不能为空", i+1)
		}
		if item.Title == "" {
			log.Errorf("第%d个解读项的标题为空", i+1)
			return errors.WithCode(errCode.ErrInvalidArgument, "第%d个解读项的标题不能为空", i+1)
		}
		if item.Content == "" {
			log.Errorf("第%d个解读项的内容为空", i+1)
			return errors.WithCode(errCode.ErrInvalidArgument, "第%d个解读项的内容不能为空", i+1)
		}
	}

	log.Infof("解读报告输入参数验证通过")
	return nil
}
