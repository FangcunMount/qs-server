package port

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
)

// MedicalScaleCreator 医学量表创建接口
type MedicalScaleCreator interface {
	// CreateMedicalScale 创建医学量表
	CreateMedicalScale(ctx context.Context, medicalScaleDTO *dto.MedicalScaleDTO) (*dto.MedicalScaleDTO, error)
}

// MedicalScaleQueryer 医学量表查询接口
type MedicalScaleQueryer interface {
	// GetMedicalScaleByCode 根据医学量表代码获取医学量表
	GetMedicalScaleByCode(ctx context.Context, code string) (*dto.MedicalScaleDTO, error)
	// GetMedicalScaleByQuestionnaireCode 根据问卷代码获取医学量表列表
	GetMedicalScaleByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*dto.MedicalScaleDTO, error)
	// ListMedicalScales 列出医学量表列表
	ListMedicalScales(ctx context.Context, page, pageSize int, conditions map[string]string) ([]*dto.MedicalScaleDTO, int64, error)
}

// MedicalScaleEditor 医学量表编辑接口
type MedicalScaleEditor interface {
	// EditBasicInfo 编辑医学量表基本信息
	EditBasicInfo(ctx context.Context, medicalScaleDTO *dto.MedicalScaleDTO) (*dto.MedicalScaleDTO, error)
	// UpdateFactors 更新医学量表因子
	UpdateFactors(ctx context.Context, code string, factors []dto.FactorDTO) (*dto.MedicalScaleDTO, error)
}
