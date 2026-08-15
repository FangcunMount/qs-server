package interpretation

import (
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// BootstrapInput carries container integration inputs for report module bootstrap.
type BootstrapInput struct {
	MySQLDB            *gorm.DB
	MySQLLimiter       backpressure.Acquirer
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
