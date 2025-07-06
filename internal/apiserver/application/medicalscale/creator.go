package medicalscale

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/mapper"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale/port"
	"github.com/yshujie/questionnaire-scale/pkg/util/codeutil"
)

// Creator 医学量表创建器
type Creator struct {
	mRepoMongo port.MedicalScaleRepositoryMongo
	mapper     mapper.MedicalScaleMapper
}

// NewCreator 创建医学量表创建器
func NewCreator(mRepoMongo port.MedicalScaleRepositoryMongo) *Creator {
	return &Creator{
		mRepoMongo: mRepoMongo,
		mapper:     mapper.NewMedicalScaleMapper(),
	}
}

// Create 创建医学量表
func (c *Creator) Create(ctx context.Context, dto *dto.MedicalScaleDTO) (*dto.MedicalScaleDTO, error) {
	// 1. 生成医学量表编码
	code, err := codeutil.GenerateCode()
	if err != nil {
		return nil, err
	}

	// 2. 创建医学量表领域模型
	msBO := medicalscale.NewMedicalScale(
		code,
		dto.Title,
		medicalscale.WithDescription(dto.Description),
		medicalscale.WithQuestionnaireCode(dto.QuestionnaireCode),
	)

	// 4. 保存到 mongodb
	if err := c.mRepoMongo.Create(ctx, msBO); err != nil {
		return nil, err
	}

	// 5. 转换为 DTO 并返回
	return c.mapper.ToDTO(msBO), nil
}
