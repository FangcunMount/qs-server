package medicalscale

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/mapper"
	medicalScale "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale/factor"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale/factor/ability"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/calculation"
	errorCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/internal/pkg/interpretation"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// Editor 医学量表编辑器
type Editor struct {
	repo   port.MedicalScaleRepositoryMongo
	mapper mapper.MedicalScaleMapper
}

// NewEditor 创建医学量表编辑器
func NewEditor(repo port.MedicalScaleRepositoryMongo) *Editor {
	return &Editor{
		repo:   repo,
		mapper: mapper.NewMedicalScaleMapper(),
	}
}

// validateMedicalScaleDTO 验证医学量表 DTO
func (e *Editor) validateMedicalScaleDTO(dto *dto.MedicalScaleDTO) error {
	if dto == nil {
		return errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "医学量表数据不能为空")
	}
	if dto.Code == "" {
		return errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "医学量表编码不能为空")
	}
	if dto.Title == "" {
		return errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "医学量表标题不能为空")
	}
	return nil
}

// EditBasicInfo 编辑医学量表基本信息
func (e *Editor) EditBasicInfo(
	ctx context.Context,
	medicalScaleDTO *dto.MedicalScaleDTO,
) (*dto.MedicalScaleDTO, error) {
	// 1. 验证输入参数
	if err := e.validateMedicalScaleDTO(medicalScaleDTO); err != nil {
		return nil, err
	}

	// 2. 获取现有医学量表
	msBO, err := e.repo.FindByCode(ctx, medicalScaleDTO.Code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取医学量表失败")
	}

	// 3. 更新基本信息
	baseInfoService := medicalScale.BaseInfoService{}
	baseInfoService.UpdateTitle(msBO, medicalScaleDTO.Title)
	baseInfoService.UpdateDescription(msBO, medicalScaleDTO.Description)

	// 4. 保存到数据库
	if err := e.repo.Update(ctx, msBO); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存医学量表基本信息失败")
	}

	// 5. 转换为 DTO 并返回
	return e.mapper.ToDTO(msBO), nil
}

// validateFactors 验证因子列表
func (e *Editor) validateFactors(factors []dto.FactorDTO) error {
	if len(factors) == 0 {
		return errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "因子列表不能为空")
	}

	for i, f := range factors {
		if f.Code == "" {
			return errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "第 %d 个因子的编码不能为空", i+1)
		}
		if f.Title == "" {
			return errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "第 %d 个因子的标题不能为空", i+1)
		}
		if f.FactorType == "" {
			return errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "第 %d 个因子的类型不能为空", i+1)
		}
	}
	return nil
}

// UpdateFactors 更新因子
func (e *Editor) UpdateFactors(
	ctx context.Context,
	code string,
	factorDTOs []dto.FactorDTO,
) (*dto.MedicalScaleDTO, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "医学量表编码不能为空")
	}
	if err := e.validateFactors(factorDTOs); err != nil {
		return nil, err
	}

	// 2. 获取现有医学量表
	msBO, err := e.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取医学量表失败")
	}

	// 4. 转换 DTO 到领域对象
	factors := make([]factor.Factor, 0, len(factorDTOs))
	for _, fDTO := range factorDTOs {
		// 创建计算规则
		var calculationRule *calculation.CalculationRule
		if fDTO.CalculationRule != nil {
			calculationRule = calculation.NewCalculationRule(
				calculation.FormulaType(fDTO.CalculationRule.FormulaType),
				fDTO.CalculationRule.SourceCodes,
			)
		}

		// 创建计算能力
		var calculationAbility *ability.CalculationAbility
		if calculationRule != nil {
			calculationAbility = &ability.CalculationAbility{}
			calculationAbility.SetCalculationRule(calculationRule)
		}

		// 创建解读能力
		var interpretationAbility *ability.InterpretationAbility
		if fDTO.InterpretRule != nil {
			interpretRule := interpretation.NewInterpretRule(
				interpretation.NewScoreRange(fDTO.InterpretRule.ScoreRange.MinScore, fDTO.InterpretRule.ScoreRange.MaxScore),
				fDTO.InterpretRule.Content,
			)
			interpretationAbility = &ability.InterpretationAbility{}
			interpretationAbility.SetInterpretationRule(&interpretRule)
		}

		// 创建因子选项
		var opts []factor.FactorOption
		if calculationAbility != nil {
			opts = append(opts, factor.WithCalculation(calculationAbility))
		}
		if interpretationAbility != nil {
			opts = append(opts, factor.WithInterpretation(interpretationAbility))
		}

		// 创建因子
		f := factor.NewFactor(fDTO.Code, fDTO.Title, factor.FactorType(fDTO.FactorType), opts...)
		factors = append(factors, f)
	}

	// 5. 更新因子
	factorService := medicalScale.FactorService{}
	// 5.1 清除现有因子
	factorService.RemoveAllFactors(msBO)
	// 5.2 按顺序添加新因子
	for _, f := range factors {
		factorService.AddFactor(msBO, f)
	}

	// 6. 保存到数据库
	if err := e.repo.Update(ctx, msBO); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存医学量表因子失败")
	}

	// 7. 转换为 DTO 并返回
	return e.mapper.ToDTO(msBO), nil
}
