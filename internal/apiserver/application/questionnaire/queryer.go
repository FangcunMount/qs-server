package questionnaire

import (
	"context"

	"github.com/fangcun-mount/qs-server/internal/apiserver/application/dto"
	"github.com/fangcun-mount/qs-server/internal/apiserver/application/mapper"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/questionnaire"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/questionnaire/port"
	errorCode "github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
)

// Queryer 问卷查询器
type Queryer struct {
	qRepoMySQL port.QuestionnaireRepositoryMySQL
	qRepoMongo port.QuestionnaireRepositoryMongo
	mapper     mapper.QuestionnaireMapper
}

// NewQueryer 创建问卷查询器
func NewQueryer(
	qRepoMySQL port.QuestionnaireRepositoryMySQL,
	qRepoMongo port.QuestionnaireRepositoryMongo,
) *Queryer {
	return &Queryer{
		qRepoMySQL: qRepoMySQL,
		qRepoMongo: qRepoMongo,
		mapper:     mapper.NewQuestionnaireMapper(),
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

	// 2. 从 MySQL 获取问卷
	qBOFromMySQL, err := q.qRepoMySQL.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 从 MongoDB 获取问题列表
	qBOFromMongo, err := q.qRepoMongo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问题列表失败")
	}

	// 4. 合并问卷数据
	qBo := q.mergeQuestionnaireData(qBOFromMySQL, qBOFromMongo)

	// 5. 转换为 DTO 并返回
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
	questionnaires, err := q.qRepoMySQL.FindList(ctx, page, pageSize, conditions)
	if err != nil {
		return nil, 0, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷列表失败")
	}

	// 3. 获取总数
	total, err := q.qRepoMySQL.CountWithConditions(ctx, conditions)
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

// mergeQuestionnaireData 合并问卷数据
func (q *Queryer) mergeQuestionnaireData(
	mysqlData *questionnaire.Questionnaire,
	mongoData *questionnaire.Questionnaire,
) *questionnaire.Questionnaire {
	// 构建选项列表
	opts := []questionnaire.QuestionnaireOption{
		questionnaire.WithID(mysqlData.GetID()),
		questionnaire.WithDescription(mysqlData.GetDescription()),
		questionnaire.WithImgUrl(mysqlData.GetImgUrl()),
		questionnaire.WithVersion(mysqlData.GetVersion()),
		questionnaire.WithStatus(mysqlData.GetStatus()),
	}

	// 如果 MongoDB 中有问卷数据且有问题列表，则添加问题
	if mongoData != nil && mongoData.GetQuestions() != nil {
		opts = append(opts, questionnaire.WithQuestions(mongoData.GetQuestions()))
	}

	// 创建问卷对象
	return questionnaire.NewQuestionnaire(
		questionnaire.NewQuestionnaireCode(mysqlData.GetCode().Value()),
		mysqlData.GetTitle(),
		opts...,
	)
}
