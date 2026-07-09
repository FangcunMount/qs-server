package modelcatalog

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// BuildPersonalityPublishedSnapshot delegates to the application published-model assembler.
func BuildPersonalityPublishedSnapshot(model *domain.AssessmentModel) (*port.PublishedModel, error) {
	return publishedmodel.BuildAssessmentSnapshot(model)
}
