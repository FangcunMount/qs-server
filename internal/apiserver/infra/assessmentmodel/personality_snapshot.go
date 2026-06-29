package assessmentmodel

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	personalitydomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality"
)

// BuildPersonalityPublishedSnapshot delegates to the domain publish builder.
func BuildPersonalityPublishedSnapshot(model *domain.AssessmentModel) (*domain.PublishedModelSnapshot, error) {
	return personalitydomain.BuildPublishedSnapshot(model)
}
