package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/mapper"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	errorCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// Editor 问卷编辑器
type Editor struct {
	qRepoMySQL port.QuestionnaireRepositoryMySQL
	qRepoMongo port.QuestionnaireRepositoryMongo
	mapper     mapper.QuestionnaireMapper
}

// NewEditor 创建问卷编辑器
func NewEditor(
	qRepoMySQL port.QuestionnaireRepositoryMySQL,
	qRepoMongo port.QuestionnaireRepositoryMongo,
) *Editor {
	return &Editor{
		qRepoMySQL: qRepoMySQL,
		qRepoMongo: qRepoMongo,
		mapper:     mapper.NewQuestionnaireMapper(),
	}
}

// validateQuestionnaireDTO 验证问卷 DTO
func (e *Editor) validateQuestionnaireDTO(dto *dto.QuestionnaireDTO) error {
	if dto == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷数据不能为空")
	}
	if dto.Code == "" {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if dto.Title == "" {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷标题不能为空")
	}
	return nil
}

// EditBasicInfo 编辑问卷基本信息
func (e *Editor) EditBasicInfo(
	ctx context.Context,
	questionnaireDTO *dto.QuestionnaireDTO,
) (*dto.QuestionnaireDTO, error) {
	// 1. 验证输入参数
	if err := e.validateQuestionnaireDTO(questionnaireDTO); err != nil {
		return nil, err
	}

	// 2. 获取现有问卷
	qBo, err := e.qRepoMySQL.FindByCode(ctx, questionnaireDTO.Code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态
	if qBo.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 更新基本信息
	baseInfoService := questionnaire.BaseInfoService{}
	baseInfoService.UpdateTitle(qBo, questionnaireDTO.Title)
	baseInfoService.UpdateDescription(qBo, questionnaireDTO.Description)
	baseInfoService.UpdateCoverImage(qBo, questionnaireDTO.ImgUrl)

	// 5. 保存到数据库
	if err := e.qRepoMySQL.Update(ctx, qBo); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷基本信息失败")
	}

	// 6. 同步到文档数据库
	if err := e.qRepoMongo.Update(ctx, qBo); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "同步问卷基本信息失败")
	}

	// 7. 转换为 DTO 并返回
	return e.mapper.ToDTO(qBo), nil
}

// validateQuestions 验证问题列表
func (e *Editor) validateQuestions(questions []dto.QuestionDTO) error {
	if len(questions) == 0 {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "问题列表不能为空")
	}

	for i, q := range questions {
		if q.Code == "" {
			return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "第 %d 个问题的编码不能为空", i+1)
		}
		if q.Title == "" {
			return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "第 %d 个问题的标题不能为空", i+1)
		}
		if q.Type == "" {
			return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "第 %d 个问题的类型不能为空", i+1)
		}
	}
	return nil
}

// UpdateQuestions 更新问题
func (e *Editor) UpdateQuestions(
	ctx context.Context,
	code string,
	questionDTOs []dto.QuestionDTO,
) (*dto.QuestionnaireDTO, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if err := e.validateQuestions(questionDTOs); err != nil {
		return nil, err
	}

	// 2. 获取现有问卷
	qBo, err := e.qRepoMySQL.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态
	if qBo.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 转换 DTO 到领域对象
	questions := make([]question.Question, 0, len(questionDTOs))
	for _, qDTO := range questionDTOs {
		q, err := e.mapper.QuestionFromDTO(&qDTO)
		if err != nil {
			return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "转换问题失败: %s", qDTO.Code)
		}
		questions = append(questions, q)
	}

	// 5. 更新问题
	questionService := questionnaire.QuestionService{}
	// 5.1 清除现有问题
	questionService.RemoveAllQuestions(qBo)
	// 5.2 按顺序添加新问题
	for _, q := range questions {
		questionService.AddQuestion(qBo, q)
	}

	// 6. 保存到数据库
	if err := e.qRepoMongo.Update(ctx, qBo); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷问题失败")
	}

	// 7. 转换为 DTO 并返回
	return e.mapper.ToDTO(qBo), nil
}
