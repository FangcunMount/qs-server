package interpretation

import (
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"go.mongodb.org/mongo-driver/mongo"
	"time"
)

// WireInput carries composition-root inputs for report module installation.
type WireInput struct {
	MongoDB            *mongo.Database
	MongoLimiter       backpressure.Acquirer
	OpsHandle          *redisruntime.Handle
	ReportStatusConfig reportstatus.Config
	OutboxProfile      appEventing.ProfileBinding
	RunLeaseDuration   time.Duration
}

// Wire builds and bootstraps the report module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	return Bootstrap(BootstrapInput(in))
}
