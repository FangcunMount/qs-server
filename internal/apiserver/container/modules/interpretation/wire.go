package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"go.mongodb.org/mongo-driver/mongo"
)

// WireInput carries composition-root inputs for report module installation.
type WireInput struct {
	MongoDB            *mongo.Database
	TopicResolver      eventcatalog.TopicResolver
	MongoLimiter       backpressure.Acquirer
	OpsHandle          *redisruntime.Handle
	ReportStatusConfig reportstatus.Config
}

// Wire builds and bootstraps the report module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	return Bootstrap(BootstrapInput(in))
}
