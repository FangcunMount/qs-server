package assessmentstore

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
)

// ValidateScaleForPublish checks scale-specific publish rules on the definition envelope.
func ValidateScaleForPublish(model *domain.AssessmentModel) error {
	scale, err := legacyadapter.MedicalScaleFromAssessmentModel(model)
	if err != nil {
		return err
	}
	validator := scaledefinition.Validator{}
	if errs := validator.ValidateForPublish(scale); len(errs) > 0 {
		return scaledefinition.ToError(errs)
	}
	return nil
}
