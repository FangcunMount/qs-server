package medicalscale

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/mapper"
	medicalScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/medical-scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/medical-scale/port"
	"github.com/FangcunMount/qs-server/pkg/util/codeutil"
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

// 确保 Creator 实现了 MedicalScaleCreator 接口
var _ port.MedicalScaleCreator = (*Creator)(nil)

// CreateMedicalScale 创建医学量表（实现接口方法）
func (c *Creator) CreateMedicalScale(ctx context.Context, dto *dto.MedicalScaleDTO) (*dto.MedicalScaleDTO, error) {
	return c.Create(ctx, dto)
}

// Create 创建医学量表
func (c *Creator) Create(ctx context.Context, dto *dto.MedicalScaleDTO) (*dto.MedicalScaleDTO, error) {
	// 1. 生成医学量表编码
	code, err := codeutil.GenerateCode()
	if err != nil {
		return nil, err
	}

	// 2. 创建医学量表领域模型
	msBO := medicalScale.NewMedicalScale(
		code,
		dto.Title,
		medicalScale.WithDescription(dto.Description),
		medicalScale.WithQuestionnaireCode(dto.QuestionnaireCode),
	)

	// 4. 保存到 mongodb
	if err := c.mRepoMongo.Create(ctx, msBO); err != nil {
		return nil, err
	}

	// 5. 转换为 DTO 并返回
	return c.mapper.ToDTO(msBO), nil
}
