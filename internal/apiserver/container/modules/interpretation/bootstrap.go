package interpretation

import (
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"

	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_classification"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

// BootstrapInput carries container integration inputs for report module bootstrap.
type BootstrapInput struct {
	MongoDB          *mongo.Database
	TopicResolver    eventcatalog.TopicResolver
	MongoLimiter     backpressure.Acquirer
	ModelDescriptors []evaldomain.ModelDescriptor
	TypologyRegistry typologyEvaluation.ModuleRegistry
	OpsHandle        *cacheplane.Handle
}

// Bootstrap assembles the report module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	if in.TypologyRegistry.Len() == 0 && len(in.ModelDescriptors) > 0 {
		return nil, fmt.Errorf("typology registry is required when model descriptors are configured")
	}
	return New(Deps(in))
}
