package questionnaire

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/mapper"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire/port"
	"github.com/FangcunMount/qs-server/pkg/util/codeutil"
)

// Creator 问卷创建器
type Creator struct {
	qRepoMySQL port.QuestionnaireRepositoryMySQL
	qRepoMongo port.QuestionnaireRepositoryMongo
	mapper     mapper.QuestionnaireMapper
}

// NewCreator 创建问卷创建器
func NewCreator(
	qRepoMySQL port.QuestionnaireRepositoryMySQL,
	qRepoMongo port.QuestionnaireRepositoryMongo,
) *Creator {
	return &Creator{
		qRepoMySQL: qRepoMySQL,
		qRepoMongo: qRepoMongo,
		mapper:     mapper.NewQuestionnaireMapper(),
	}
}

// CreateQuestionnaire 创建问卷
func (c *Creator) CreateQuestionnaire(ctx context.Context, questionnaireDTO *dto.QuestionnaireDTO) (*dto.QuestionnaireDTO, error) {
	// 1. 生成问卷编码
	code, err := codeutil.GenerateCode()
	if err != nil {
		return nil, err
	}

	// 2. 创建问卷领域模型
	qBo := questionnaire.NewQuestionnaire(
		questionnaire.NewQuestionnaireCode(code),
		questionnaireDTO.Title,
		questionnaire.WithDescription(questionnaireDTO.Description),
		questionnaire.WithImgUrl(questionnaireDTO.ImgUrl),
		questionnaire.WithVersion(questionnaire.NewQuestionnaireVersion("1.0")),
		questionnaire.WithStatus(questionnaire.STATUS_DRAFT),
	)

	// 3. 保存到 mysql
	if err := c.qRepoMySQL.Create(ctx, qBo); err != nil {
		return nil, err
	}

	// 4. 保存到 mongodb
	if err := c.qRepoMongo.Create(ctx, qBo); err != nil {
		return nil, err
	}

	// 5. 转换为 DTO 并返回
	return c.mapper.ToDTO(qBo), nil
}
