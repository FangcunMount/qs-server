package modelcatalog

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
)

// BuildPersonalityPublishedSnapshot delegates to the domain publish builder.
func BuildPersonalityPublishedSnapshot(model *domain.AssessmentModel) (*domain.PublishedModelSnapshot, error) {
	return publishing.BuildPublishedSnapshot(model)
}
