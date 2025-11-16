package medicalscale

import (
	"context"

	"github.com/FangcunMount/compose-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/mapper"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/medical-scale/port"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Queryer 医学量表查询器，实现 MedicalScaleQueryer 接口
type Queryer struct {
	repo   port.MedicalScaleRepositoryMongo
	mapper mapper.MedicalScaleMapper
}

// NewQueryer 创建医学量表查询器
func NewQueryer(repo port.MedicalScaleRepositoryMongo) *Queryer {
	return &Queryer{
		repo:   repo,
		mapper: mapper.NewMedicalScaleMapper(),
	}
}

// 确保 Queryer 实现了 MedicalScaleQueryer 接口
var _ port.MedicalScaleQueryer = (*Queryer)(nil)

// validateCode 验证医学量表编码
func (q *Queryer) validateCode(code string) error {
	if code == "" {
		return errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "医学量表编码不能为空")
	}
	return nil
}

// validatePagination 验证分页参数
func (q *Queryer) validatePagination(page, pageSize int) error {
	if page <= 0 {
		return errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "页码必须大于0")
	}
	if pageSize <= 0 {
		return errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "每页数量必须大于0")
	}
	if pageSize > 100 {
		return errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "每页数量不能超过100")
	}
	return nil
}

// GetMedicalScaleByCode 根据编码获取医学量表
func (q *Queryer) GetMedicalScaleByCode(
	ctx context.Context,
	code string,
) (*dto.MedicalScaleDTO, error) {
	// 1. 验证输入参数
	if err := q.validateCode(code); err != nil {
		return nil, err
	}

	// 2. 从仓储获取医学量表
	medicalScale, err := q.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取医学量表失败")
	}

	// 3. 转换为 DTO 并返回
	return q.mapper.ToDTO(medicalScale), nil
}

// GetMedicalScaleByQuestionnaireCode 根据问卷代码获取医学量表
func (q *Queryer) GetMedicalScaleByQuestionnaireCode(
	ctx context.Context,
	questionnaireCode string,
) (*dto.MedicalScaleDTO, error) {
	// 1. 验证输入参数
	if err := q.validateCode(questionnaireCode); err != nil {
		return nil, errors.WithCode(errorCode.ErrMedicalScaleInvalidInput, "问卷编码不能为空")
	}

	// 2. 从仓储获取医学量表
	medicalScale, err := q.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取医学量表失败")
	}

	// 3. 转换为 DTO 并返回
	return q.mapper.ToDTO(medicalScale), nil
}

// ListMedicalScales 获取医学量表列表
func (q *Queryer) ListMedicalScales(
	ctx context.Context,
	page, pageSize int,
	conditions map[string]string,
) ([]*dto.MedicalScaleDTO, int64, error) {
	// 1. 验证分页参数
	if err := q.validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}

	// 2. 获取医学量表列表
	medicalScales, err := q.repo.FindList(ctx, page, pageSize, conditions)
	if err != nil {
		return nil, 0, errors.WrapC(err, errorCode.ErrDatabase, "获取医学量表列表失败")
	}

	// 3. 获取总数
	total, err := q.repo.CountWithConditions(ctx, conditions)
	if err != nil {
		return nil, 0, errors.WrapC(err, errorCode.ErrDatabase, "获取医学量表总数失败")
	}

	// 4. 转换为 DTO 列表
	dtos := make([]*dto.MedicalScaleDTO, 0, len(medicalScales))
	for _, medicalScale := range medicalScales {
		dtos = append(dtos, q.mapper.ToDTO(medicalScale))
	}

	return dtos, total, nil
}
