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
	EditBasicInfo(ctx context.Context, id uint64, title, imgUrl string, version uint8) (*questionnaire.Questionnaire, error)
}

type QuestionnairePublisher interface {
	PublishQuestionnaire(ctx context.Context, id uint64) (*questionnaire.Questionnaire, error)
	UnpublishQuestionnaire(ctx context.Context, id uint64) (*questionnaire.Questionnaire, error)
}
