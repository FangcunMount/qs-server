package interpretation

import (
	mongoEventTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	mysqlEventTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// WireInput carries composition-root inputs for report module installation.
type WireInput struct {
	MySQLDB                 *gorm.DB
	MySQLLimiter            backpressure.Acquirer
	MongoDB                 *mongo.Database
	MongoLimiter            backpressure.Acquirer
	OpsHandle               *redisruntime.Handle
	ReportStatusConfig      reportstatus.Config
	OutboxProfile           appEventing.ProfileBinding
	RunLeaseDuration        time.Duration
	AIWorkflow              *apiserveroptions.AIWorkflowOptions
	EventTransactionOwner   *mongoEventTx.Owner
	CommandTransactionOwner *mysqlEventTx.Committer
	TransactionRoutes       eventcatalog.TopicResolver
}

// Wire builds and bootstraps the report module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	return Bootstrap(BootstrapInput(in))
}
