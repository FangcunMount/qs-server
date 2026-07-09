package assessmentstore

import (
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// NewPublicationPublisher builds the scale publication coordinator.
func NewPublicationPublisher(modelRepo modelcatalogport.ModelRepository, publishedRepo modelcatalogport.PublishedModelRepository) publication.Publisher {
	return publication.Publisher{
		Registry:  appdefinition.NewRegistry(DefinitionHandler{}),
		ModelRepo: modelRepo,
		Repo:      publishedRepo,
	}
}
