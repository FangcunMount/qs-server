package questionnaire

import (
	"context"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	"github.com/yshujie/questionnaire-scale/pkg/util/idutil"
)

// Creator 问卷创建器
type Creator struct {
	quesRepo port.QuestionnaireRepository
	quesDoc  port.QuestionnaireDocument
}

// NewCreator 创建问卷创建器
func NewCreator(
	quesRepo port.QuestionnaireRepository,
	quesDoc port.QuestionnaireDocument,
) *Creator {
	return &Creator{quesRepo: quesRepo, quesDoc: quesDoc}
}

// CreateQuestionnaire 创建问卷
func (c *Creator) CreateQuestionnaire(ctx context.Context, req port.QuestionnaireCreateRequest) (*port.QuestionnaireResponse, error) {
	// 1. 创建问卷领域模型
	quesDomain := &questionnaire.Questionnaire{
		Code:        idutil.GetUUID36("ques")[:8],
		Title:       req.Title,
		Description: req.Description,
		ImgUrl:      req.ImgUrl,
		Version:     1,
		Status:      questionnaire.STATUS_DRAFT.Value(),
	}

	// 2. 保存到 mysql
	if err := c.quesRepo.Save(ctx, quesDomain); err != nil {
		return nil, err
	}

	// 3. 保存到 mongodb
	if err := c.quesDoc.Save(ctx, quesDomain); err != nil {
		return nil, err
	}

	// 4. 返回问卷响应
	return &port.QuestionnaireResponse{
		ID:        quesDomain.ID.Value(),
		Code:      quesDomain.Code,
		Title:     quesDomain.Title,
		ImgUrl:    quesDomain.ImgUrl,
		Version:   quesDomain.Version,
		Status:    quesDomain.Status,
		CreatedAt: quesDomain.CreatedAt.Format(time.RFC3339),
		UpdatedAt: quesDomain.UpdatedAt.Format(time.RFC3339),
	}, nil
}
