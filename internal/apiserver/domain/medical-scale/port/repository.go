package port

import (
	"context"

	medicalScale "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale"
)

// Repository 医学量表仓储接口
type MedicalScaleRepositoryMongo interface {
	Create(ctx context.Context, qDomain *medicalScale.MedicalScale) error
	FindByCode(ctx context.Context, code string) (*medicalScale.MedicalScale, error)
	FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*medicalScale.MedicalScale, error)
	FindList(ctx context.Context, page, pageSize int, conditions map[string]string) ([]*medicalScale.MedicalScale, error)
	CountWithConditions(ctx context.Context, conditions map[string]string) (int64, error)
	Update(ctx context.Context, qDomain *medicalScale.MedicalScale) error
	ExistsByCode(ctx context.Context, code string) (bool, error)
}
