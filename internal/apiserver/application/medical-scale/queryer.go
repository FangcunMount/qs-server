package medicalscale

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/mapper"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale/port"
	errorCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// Queryer 医学量表查询器
type Queryer struct {
	repo   port.MedicalScaleQueryer
	mapper mapper.MedicalScaleMapper
}

// NewQueryer 创建医学量表查询器
func NewQueryer(repo port.MedicalScaleQueryer) *Queryer {
	return &Queryer{
		repo:   repo,
		mapper: mapper.NewMedicalScaleMapper(),
	}
}

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
	return q.repo.GetMedicalScaleByCode(ctx, code)
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
	return q.repo.GetMedicalScaleByQuestionnaireCode(ctx, questionnaireCode)
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
	return q.repo.ListMedicalScales(ctx, page, pageSize, conditions)
}
