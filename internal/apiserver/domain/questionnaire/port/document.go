package port

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
)

type QuestionnaireDocument interface {
	Save(ctx context.Context, qDomain *questionnaire.Questionnaire) error
	FindByID(ctx context.Context, id uint64) (*questionnaire.Questionnaire, error)
	FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error)
	Update(ctx context.Context, qDomain *questionnaire.Questionnaire) error
	Remove(ctx context.Context, id uint64) error
	HardDelete(ctx context.Context, id uint64) error
	ExistsByCode(ctx context.Context, code string) (bool, error)
	FindActiveQuestionnaires(ctx context.Context) ([]*questionnaire.Questionnaire, error)
}
