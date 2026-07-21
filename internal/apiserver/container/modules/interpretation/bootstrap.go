package interpretation

import (
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"go.mongodb.org/mongo-driver/mongo"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
)

// BootstrapInput carries container integration inputs for report module bootstrap.
type BootstrapInput struct {
	MongoDB            *mongo.Database
	MongoLimiter       backpressure.Acquirer
	OpsHandle          *redisruntime.Handle
	ReportStatusConfig reportstatus.Config
	OutboxProfile      appEventing.ProfileBinding
	RunLeaseDuration   time.Duration
}

// Bootstrap assembles the report module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	leaseDuration := in.RunLeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = apiserveroptions.NewInterpretationLeaseGovernanceOptions().RunLeaseDuration()
	}
	deps := Deps(in)
	deps.RunLeaseDuration = leaseDuration
	return New(deps)
}
