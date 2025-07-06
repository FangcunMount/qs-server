package port

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale"
)

// Repository 医学量表仓储接口
type MedicalScaleRepositoryMongo interface {
	Create(ctx context.Context, qDomain *medicalscale.MedicalScale) error
	FindByCode(ctx context.Context, code string) (*medicalscale.MedicalScale, error)
	FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) ([]*medicalscale.MedicalScale, error)
	Update(ctx context.Context, qDomain *medicalscale.MedicalScale) error
	ExistsByCode(ctx context.Context, code string) (bool, error)
}
