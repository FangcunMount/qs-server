package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	"github.com/yshujie/questionnaire-scale/pkg/util/codeutil"
)

// Creator 问卷创建器
type Creator struct {
	qRepoMySQL port.QuestionnaireRepositoryMySQL
	qRepoMongo port.QuestionnaireRepositoryMongo
}

// NewCreator 创建问卷创建器
func NewCreator(
	qRepoMySQL port.QuestionnaireRepositoryMySQL,
	qRepoMongo port.QuestionnaireRepositoryMongo,
) *Creator {
	return &Creator{qRepoMySQL: qRepoMySQL, qRepoMongo: qRepoMongo}
}

// CreateQuestionnaire 创建问卷
func (c *Creator) CreateQuestionnaire(ctx context.Context, title, description, imgUrl string) (*questionnaire.Questionnaire, error) {
	// 1. 生成问卷编码
	code, err := codeutil.GenerateCode()
	if err != nil {
		return nil, err
	}

	// 2. 创建问卷领域模型
	qBo := questionnaire.NewQuestionnaire(questionnaire.NewQuestionnaireCode(code), title)
	qBo.SetDescription(description)
	qBo.SetImgUrl(imgUrl)
	qBo.SetVersion(questionnaire.NewQuestionnaireVersion("1.0"))
	qBo.SetStatus(questionnaire.STATUS_DRAFT)

	// 3. 保存到 mysql
	if err := c.qRepoMySQL.Save(ctx, qBo); err != nil {
		return nil, err
	}

	// 4. 保存到 mongodb
	if err := c.qRepoMongo.Save(ctx, qBo); err != nil {
		return nil, err
	}

	// 5. 返回问卷领域对象
	return qBo, nil
}
