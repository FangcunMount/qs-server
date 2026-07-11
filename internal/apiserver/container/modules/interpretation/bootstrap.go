package interpretation

import (
	"go.mongodb.org/mongo-driver/mongo"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

// BootstrapInput carries container integration inputs for report module bootstrap.
type BootstrapInput struct {
	MongoDB            *mongo.Database
	TopicResolver      eventcatalog.TopicResolver
	MongoLimiter       backpressure.Acquirer
	ModelDescriptors   []evaldomain.ModelDescriptor
	OpsHandle          *cacheplane.Handle
	ReportStatusConfig reportstatus.Config
}

// Bootstrap assembles the report module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps(in))
}
