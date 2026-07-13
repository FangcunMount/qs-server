package interpretation

import (
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

// BootstrapInput carries container integration inputs for report module bootstrap.
type BootstrapInput struct {
	MongoDB            *mongo.Database
	MongoLimiter       backpressure.Acquirer
	OpsHandle          *redisruntime.Handle
	ReportStatusConfig reportstatus.Config
	OutboxProfile      appEventing.ProfileBinding
}

// Bootstrap assembles the report module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps(in))
}
