package interpretation

import (
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
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
}

// Wire builds and bootstraps the report module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	return Bootstrap(BootstrapInput{
		MongoDB:          in.MongoDB,
		TopicResolver:    in.TopicResolver,
		MongoLimiter:     in.MongoLimiter,
		ModelDescriptors: in.ModelDescriptors,
		OpsHandle:        in.OpsHandle,
	})
}

// Ports exposes report integration ports for downstream modules.
type Ports struct {
	QueryService assessmentApp.ReportQueryService
}

// ExportPorts projects report module ports for evaluation wiring.
func ExportPorts(module *Module) Ports {
	if module == nil {
		return Ports{}
	}
	return Ports{
		QueryService: module.QueryService,
	}
}
