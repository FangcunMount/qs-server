package questionnaire

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/mapper"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire/port"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Creator 问卷创建器
type Creator struct {
	qRepo  port.QuestionnaireRepositoryMongo
	mapper mapper.QuestionnaireMapper
}

// NewCreator 创建问卷创建器
func NewCreator(
	qRepo port.QuestionnaireRepositoryMongo,
) *Creator {
	return &Creator{
		qRepo:  qRepo,
		mapper: mapper.NewQuestionnaireMapper(),
	}
}

// CreateQuestionnaire 创建问卷
func (c *Creator) CreateQuestionnaire(ctx context.Context, questionnaireDTO *dto.QuestionnaireDTO) (*dto.QuestionnaireDTO, error) {
	// 1. 生成问卷编码
	code, err := meta.GenerateCode()
	if err != nil {
		return nil, err
	}

	// 2. 创建问卷领域模型
	qBo := questionnaire.NewQuestionnaire(
		meta.NewCode(code.String()),
		questionnaireDTO.Title,
		questionnaire.WithDescription(questionnaireDTO.Description),
		questionnaire.WithImgUrl(questionnaireDTO.ImgUrl),
		questionnaire.WithVersion(questionnaire.NewQuestionnaireVersion("1.0")),
		questionnaire.WithStatus(questionnaire.STATUS_DRAFT),
	)

	// 3. 保存到 MongoDB
	if err := c.qRepo.Create(ctx, qBo); err != nil {
		return nil, err
	}

	// 4. 转换为 DTO 并返回
	return c.mapper.ToDTO(qBo), nil
}
