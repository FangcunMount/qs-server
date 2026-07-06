package modelcatalog

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	personalitydomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality"
)

// BuildPersonalityPublishedSnapshot delegates to the domain publish builder.
func BuildPersonalityPublishedSnapshot(model *domain.AssessmentModel) (*domain.PublishedModelSnapshot, error) {
	return personalitydomain.BuildPublishedSnapshot(model)
}
