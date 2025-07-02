package port

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
)

type QuestionnaireCreator interface {
	CreateQuestionnaire(ctx context.Context, title, description, imgUrl string) (*questionnaire.Questionnaire, error)
}

type QuestionnaireQueryer interface {
	GetQuestionnaire(ctx context.Context, id uint64) (*questionnaire.Questionnaire, error)
	GetQuestionnaireByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error)
	ListQuestionnaires(ctx context.Context, page, pageSize int) ([]*questionnaire.Questionnaire, int64, error)
}

type QuestionnaireEditor interface {
	EditBasicInfo(ctx context.Context, code questionnaire.QuestionnaireCode, title, description, imgUrl string) (*questionnaire.Questionnaire, error)
}

type QuestionnairePublisher interface {
	Publish(ctx context.Context, code questionnaire.QuestionnaireCode) (*questionnaire.Questionnaire, error)
	Unpublish(ctx context.Context, code questionnaire.QuestionnaireCode) (*questionnaire.Questionnaire, error)
}
