package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"go.mongodb.org/mongo-driver/mongo"
)

// WireInput carries composition-root inputs for report module installation.
type WireInput struct {
	MongoDB            *mongo.Database
	TopicResolver      eventcatalog.TopicResolver
	MongoLimiter       backpressure.Acquirer
	OpsHandle          *cacheplane.Handle
	ReportStatusConfig reportstatus.Config
}

// Wire builds and bootstraps the report module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	return Bootstrap(BootstrapInput{
		MongoDB:            in.MongoDB,
		TopicResolver:      in.TopicResolver,
		MongoLimiter:       in.MongoLimiter,
		OpsHandle:          in.OpsHandle,
		ReportStatusConfig: in.ReportStatusConfig,
	})
}
