package statistics

import (
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
)

// BootstrapInput carries container integration inputs for statistics module bootstrap.
type BootstrapInput struct {
	MySQLDB               *gorm.DB
	RedisClient           redis.UniversalClient
	CacheBuilder          *keyspace.Builder
	AnswerSheetReader     surveyreadmodel.AnswerSheetReader
	AnswerSheetScanSource statisticsApp.AnswerSheetScanSource
	MongoDB               *mongo.Database
	RepairWindowDays      int
	QueryPolicy           cachepolicy.CachePolicy
	SystemStatisticsOpts  statisticsApp.SystemStatisticsOptions
	HotsetRecorder        cachetarget.HotsetRecorder
	LockManager           locklease.Manager
	VersionStore          cachequery.VersionTokenStore
	Observer              *observability.ComponentObserver
	MySQLLimiter          backpressure.Acquirer
	WarmupCoordinator     cachegov.Coordinator
	StatusService         cachegov.StatusService
}

// Bootstrap assembles the statistics module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps(in))
}
