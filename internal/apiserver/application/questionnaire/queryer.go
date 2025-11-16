package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/mapper"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire/port"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Queryer 问卷查询器
type Queryer struct {
	qRepo  port.QuestionnaireRepositoryMongo
	mapper mapper.QuestionnaireMapper
}

// NewQueryer 创建问卷查询器
func NewQueryer(
	qRepo port.QuestionnaireRepositoryMongo,
) *Queryer {
	return &Queryer{
		qRepo:  qRepo,
		mapper: mapper.NewQuestionnaireMapper(),
	}
}

// validateCode 验证问卷编码
func (q *Queryer) validateCode(code string) error {
	if code == "" {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	return nil
}

// validatePagination 验证分页参数
func (q *Queryer) validatePagination(page, pageSize int) error {
	if page <= 0 {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "页码必须大于0")
	}
	if pageSize <= 0 {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量必须大于0")
	}
	if pageSize > 100 {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量不能超过100")
	}
	return nil
}

// GetQuestionnaireByCode 根据编码获取问卷
func (q *Queryer) GetQuestionnaireByCode(
	ctx context.Context,
	code string,
) (*dto.QuestionnaireDTO, error) {
	// 1. 验证输入参数
	if err := q.validateCode(code); err != nil {
		return nil, err
	}

	// 2. 从 MongoDB 获取问卷
	qBo, err := q.qRepo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 转换为 DTO 并返回
	return q.mapper.ToDTO(qBo), nil
}

// ListQuestionnaires 获取问卷列表
func (q *Queryer) ListQuestionnaires(
	ctx context.Context,
	page, pageSize int,
	conditions map[string]string,
) ([]*dto.QuestionnaireDTO, int64, error) {
	// 1. 验证分页参数
	if err := q.validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}

	// 2. 获取问卷列表
	questionnaires, err := q.qRepo.FindList(ctx, page, pageSize, conditions)
	if err != nil {
		return nil, 0, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷列表失败")
	}

	// 3. 获取总数
	total, err := q.qRepo.CountWithConditions(ctx, conditions)
	if err != nil {
		return nil, 0, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷总数失败")
	}

	// 4. 转换为 DTO 列表
	dtos := make([]*dto.QuestionnaireDTO, 0, len(questionnaires))
	for _, questionnaire := range questionnaires {
		dtos = append(dtos, q.mapper.ToDTO(questionnaire))
	}

	return dtos, total, nil
}
