package container

import (
	auth "github.com/FangcunMount/iam-contracts/pkg/sdk/auth/verifier"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	answersheetApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	domainoperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

// ServerGRPCBootstrapDeps describes the narrow container-owned dependencies
// needed to build the process gRPC server.
type ServerGRPCBootstrapDeps struct {
	AuthzSnapshotLoader *iaminfra.AuthzSnapshotLoader
	ActiveOperatorRepo  domainoperator.Repository
	TokenVerifier       *auth.TokenVerifier
}

// ServerRuntimeDeps describes the narrow container-owned dependencies needed by
// background runtimes started from the apiserver process.
type ServerRuntimeDeps struct {
	LockBuilder               *rediskey.Builder
	LockManager               *redislock.Manager
	WarmupCoordinator         cachegov.Coordinator
	PlanCommandService        planApp.PlanCommandService
	StatisticsSyncService     statisticsApp.StatisticsSyncService
	BehaviorProjectorService  statisticsApp.BehaviorProjectorService
	AnswerSheetSubmittedRelay answersheetApp.SubmittedEventRelay
	AssessmentOutboxRelay     appEventing.OutboxRelay
}

func (c *Container) BuildServerGRPCBootstrapDeps() ServerGRPCBootstrapDeps {
	var deps ServerGRPCBootstrapDeps
	if c == nil {
		return deps
	}
	if c.IAMModule != nil {
		deps.AuthzSnapshotLoader = c.IAMModule.AuthzSnapshotLoader()
		deps.TokenVerifier = c.IAMModule.SDKTokenVerifier()
	}
	if c.ActorModule != nil {
		deps.ActiveOperatorRepo = c.ActorModule.OperatorRepo
	}
	return deps
}

func (c *Container) BuildServerRuntimeDeps() ServerRuntimeDeps {
	var deps ServerRuntimeDeps
	if c == nil {
		return deps
	}

	deps.LockBuilder = c.CacheBuilder(redisplane.FamilyLock)
	deps.LockManager = c.CacheLockManager()
	deps.WarmupCoordinator = c.WarmupCoordinator()

	if c.PlanModule != nil {
		deps.PlanCommandService = c.PlanModule.CommandService
	}
	if c.StatisticsModule != nil {
		deps.StatisticsSyncService = c.StatisticsModule.SyncService
		deps.BehaviorProjectorService = c.StatisticsModule.BehaviorProjectorService
	}
	if c.SurveyModule != nil && c.SurveyModule.AnswerSheet != nil {
		deps.AnswerSheetSubmittedRelay = c.SurveyModule.AnswerSheet.SubmittedEventRelay
	}
	if c.EvaluationModule != nil {
		deps.AssessmentOutboxRelay = c.EvaluationModule.AssessmentOutboxRelay
	}

	return deps
}
