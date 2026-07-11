package interpretation

import (
	evalregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"go.mongodb.org/mongo-driver/mongo"
)

// WireInput carries composition-root inputs for report module installation.
type WireInput struct {
	MongoDB          *mongo.Database
	TopicResolver    eventcatalog.TopicResolver
	MongoLimiter     backpressure.Acquirer
	OpsHandle        *cacheplane.Handle
	ModelDescriptors []evaldomain.ModelDescriptor
	TypologyRegistry evalregistry.TypologyRegistry
}

// Wire builds and bootstraps the report module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	return Bootstrap(BootstrapInput{
		MongoDB:          in.MongoDB,
		TopicResolver:    in.TopicResolver,
		MongoLimiter:     in.MongoLimiter,
		ModelDescriptors: in.ModelDescriptors,
		TypologyRegistry: in.TypologyRegistry,
		OpsHandle:        in.OpsHandle,
	})
}

// Ports exposes report integration ports for downstream modules.
type Ports struct {
	Reader evaluationreadmodel.ReportReader
}

// ExportPorts projects report module ports for evaluation wiring.
func ExportPorts(module *Module) Ports {
	if module == nil {
		return Ports{}
	}
	return Ports{
		Reader: module.Reader(),
	}
}
