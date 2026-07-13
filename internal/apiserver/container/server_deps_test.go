package container

import (
	"context"
	"testing"

	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	iammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/iam"
	domainoperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

func TestContainerBuildServerGRPCBootstrapDeps(t *testing.T) {
	t.Parallel()

	roleUpdater := &serverBootstrapRoleUpdaterStub{}
	authzSnapshot := &iaminfra.AuthzSnapshotLoader{}

	c := NewContainer(nil, nil, nil)
	c.IAMModule = iammod.NewTestModule(iammod.TestModuleOptions{
		TokenVerifier:       &iaminfra.TokenVerifier{},
		AuthzSnapshotLoader: authzSnapshot,
	})
	c.ActorModule = &ActorModule{
		OperatorRoleProjectionUpdater: roleUpdater,
	}

	deps := c.BuildServerGRPCBootstrapDeps()
	if deps.TokenVerifier != nil {
		t.Fatalf("TokenVerifier = %#v, want nil passthrough from zero-value verifier", deps.TokenVerifier)
	}
	if deps.AuthzSnapshotLoader != authzSnapshot {
		t.Fatalf("AuthzSnapshotLoader = %#v, want %#v", deps.AuthzSnapshotLoader, authzSnapshot)
	}
	if deps.OperatorRoleProjectionUpdater != roleUpdater {
		t.Fatalf("OperatorRoleProjectionUpdater = %#v, want %#v", deps.OperatorRoleProjectionUpdater, roleUpdater)
	}
}

func TestContainerBuildServerRuntimeDeps(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{}, nil)
	c.cache.BindGovernance(cachebootstrap.GovernanceBindings{})

	planCommand := &planCommandServiceStub{}
	statsSync := &statisticsSyncServiceStub{}
	behaviorProjector := &behaviorProjectorServiceStub{}
	answerSheetRelay := &outboxRelayStub{}
	assessmentRelay := &outboxRelayStub{}
	consistencyReconcile := &evaluationConsistencyReconcileServiceStub{}

	c.PlanModule = &PlanModule{CommandService: planCommand}
	c.StatisticsModule = &StatisticsModule{
		SyncService:              statsSync,
		BehaviorProjectorService: behaviorProjector,
	}
	c.SurveyModule = &SurveyModule{
		MongoDomainEventRelay: answerSheetRelay,
	}
	c.EvaluationModule = &EvaluationModule{
		AssessmentOutboxRelay: assessmentRelay,
		SchedulerService:      consistencyReconcile,
	}

	deps := c.BuildServerRuntimeDeps()
	if deps.LockBuilder != c.CacheBuilder(redisruntime.FamilyLock) {
		t.Fatalf("LockBuilder = %#v, want %#v", deps.LockBuilder, c.CacheBuilder(redisruntime.FamilyLock))
	}
	if deps.LockManager != c.CacheLockManager() {
		t.Fatalf("LockManager = %#v, want %#v", deps.LockManager, c.CacheLockManager())
	}
	if deps.WarmupCoordinator != c.WarmupCoordinator() {
		t.Fatalf("WarmupCoordinator = %#v, want %#v", deps.WarmupCoordinator, c.WarmupCoordinator())
	}
	if deps.PlanCommandService != planCommand {
		t.Fatalf("PlanCommandService = %#v, want %#v", deps.PlanCommandService, planCommand)
	}
	if deps.StatisticsSyncService != statsSync {
		t.Fatalf("StatisticsSyncService = %#v, want %#v", deps.StatisticsSyncService, statsSync)
	}
	if deps.BehaviorProjectorService != behaviorProjector {
		t.Fatalf("BehaviorProjectorService = %#v, want %#v", deps.BehaviorProjectorService, behaviorProjector)
	}
	if deps.MongoDomainEventRelay != answerSheetRelay {
		t.Fatalf("MongoDomainEventRelay = %#v, want %#v", deps.MongoDomainEventRelay, answerSheetRelay)
	}
	if deps.AssessmentOutboxRelay != assessmentRelay {
		t.Fatalf("AssessmentOutboxRelay = %#v, want %#v", deps.AssessmentOutboxRelay, assessmentRelay)
	}
	if deps.EvaluationConsistencyReconcileService != consistencyReconcile {
		t.Fatalf("EvaluationConsistencyReconcileService = %#v, want %#v", deps.EvaluationConsistencyReconcileService, consistencyReconcile)
	}
}

type fakeOperatorRepo struct{}

func (*fakeOperatorRepo) Save(context.Context, *domainoperator.Operator) error   { return nil }
func (*fakeOperatorRepo) Update(context.Context, *domainoperator.Operator) error { return nil }
func (*fakeOperatorRepo) FindByID(context.Context, domainoperator.ID) (*domainoperator.Operator, error) {
	return nil, nil
}
func (*fakeOperatorRepo) FindByUser(context.Context, int64, int64) (*domainoperator.Operator, error) {
	return nil, nil
}
func (*fakeOperatorRepo) ListByOrg(context.Context, int64, int, int) ([]*domainoperator.Operator, error) {
	return nil, nil
}
func (*fakeOperatorRepo) ListByRole(context.Context, int64, domainoperator.Role, int, int) ([]*domainoperator.Operator, error) {
	return nil, nil
}
func (*fakeOperatorRepo) Delete(context.Context, domainoperator.ID) error { return nil }
func (*fakeOperatorRepo) Count(context.Context, int64) (int64, error)     { return 0, nil }

type outboxRelayStub struct{}

func (*outboxRelayStub) DispatchDue(context.Context) error { return nil }

type planCommandServiceStub struct{}

func (*planCommandServiceStub) CreatePlan(context.Context, planApp.CreatePlanDTO) (*planApp.PlanResult, error) {
	return nil, nil
}
func (*planCommandServiceStub) PausePlan(context.Context, int64, string) (*planApp.PlanResult, error) {
	return nil, nil
}
func (*planCommandServiceStub) ResumePlan(context.Context, int64, string, map[string]string) (*planApp.PlanResult, error) {
	return nil, nil
}
func (*planCommandServiceStub) FinishPlan(context.Context, int64, string) (*planApp.PlanResult, error) {
	return nil, nil
}
func (*planCommandServiceStub) CancelPlan(context.Context, int64, string) (*planApp.PlanMutationResult, error) {
	return nil, nil
}
func (*planCommandServiceStub) EnrollTestee(context.Context, planApp.EnrollTesteeDTO) (*planApp.EnrollmentResult, error) {
	return nil, nil
}
func (*planCommandServiceStub) TerminateEnrollment(context.Context, int64, string, string) (*planApp.EnrollmentTerminationResult, error) {
	return nil, nil
}
func (*planCommandServiceStub) SchedulePendingTasks(context.Context, int64, string) (*planApp.TaskScheduleResult, error) {
	return nil, nil
}
func (*planCommandServiceStub) OpenTask(context.Context, int64, string) (*planApp.TaskResult, error) {
	return nil, nil
}
func (*planCommandServiceStub) CompleteTask(context.Context, int64, string, string) (*planApp.TaskResult, error) {
	return nil, nil
}
func (*planCommandServiceStub) ExpireTask(context.Context, int64, string) (*planApp.TaskResult, error) {
	return nil, nil
}
func (*planCommandServiceStub) CancelTask(context.Context, int64, string) (*planApp.TaskMutationResult, error) {
	return nil, nil
}

type statisticsSyncServiceStub struct{}

func (*statisticsSyncServiceStub) SyncDailyStatistics(context.Context, int64, statisticsApp.SyncDailyOptions) error {
	return nil
}
func (*statisticsSyncServiceStub) SyncOrgSnapshotStatistics(context.Context, int64) error { return nil }
func (*statisticsSyncServiceStub) SyncPlanStatistics(context.Context, int64) error        { return nil }

type behaviorProjectorServiceStub struct{}

func (*behaviorProjectorServiceStub) ProjectBehaviorEvent(context.Context, statisticsApp.BehaviorProjectEventInput) (statisticsApp.BehaviorProjectEventResult, error) {
	return statisticsApp.BehaviorProjectEventResult{}, nil
}
func (*behaviorProjectorServiceStub) ReconcilePendingBehaviorEvents(context.Context, int) (int, error) {
	return 0, nil
}

type evaluationConsistencyReconcileServiceStub struct{}

func (*evaluationConsistencyReconcileServiceStub) AuditOnce(context.Context, int) (int, error) {
	return 0, nil
}

var _ domainoperator.Repository = (*fakeOperatorRepo)(nil)

type serverBootstrapRoleUpdaterStub struct{}

func (*serverBootstrapRoleUpdaterStub) PersistFromSnapshot(context.Context, *operatorApp.OperatorResult, *authzapp.Snapshot) error {
	return nil
}
func (*serverBootstrapRoleUpdaterStub) PersistFromSnapshotByUser(context.Context, int64, int64, *authzapp.Snapshot) error {
	return nil
}
func (*serverBootstrapRoleUpdaterStub) SyncRoles(context.Context, int64, uint64) error { return nil }

var _ appEventing.OutboxRelay = (*outboxRelayStub)(nil)
var _ planApp.PlanCommandService = (*planCommandServiceStub)(nil)
var _ statisticsApp.StatisticsSyncService = (*statisticsSyncServiceStub)(nil)
var _ statisticsApp.BehaviorProjectorService = (*behaviorProjectorServiceStub)(nil)
var _ cachegov.Coordinator = cachegov.NewCoordinator(cachegov.Config{}, cachegov.Dependencies{})
