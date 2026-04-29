package container

import (
	"context"
	"testing"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	domainoperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

func TestContainerBuildServerGRPCBootstrapDeps(t *testing.T) {
	t.Parallel()

	operatorRepo := &fakeOperatorRepo{}
	authzSnapshot := &iaminfra.AuthzSnapshotLoader{}

	c := NewContainer(nil, nil, nil)
	c.IAMModule = &IAMModule{
		tokenVerifier:       &iaminfra.TokenVerifier{},
		authzSnapshotLoader: authzSnapshot,
	}
	c.ActorModule = &assembler.ActorModule{
		OperatorRepo: operatorRepo,
	}

	deps := c.BuildServerGRPCBootstrapDeps()
	if deps.TokenVerifier != nil {
		t.Fatalf("TokenVerifier = %#v, want nil passthrough from zero-value verifier", deps.TokenVerifier)
	}
	if deps.AuthzSnapshotLoader != authzSnapshot {
		t.Fatalf("AuthzSnapshotLoader = %#v, want %#v", deps.AuthzSnapshotLoader, authzSnapshot)
	}
	if deps.ActiveOperatorRepo != operatorRepo {
		t.Fatalf("ActiveOperatorRepo = %#v, want %#v", deps.ActiveOperatorRepo, operatorRepo)
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

	c.PlanModule = &assembler.PlanModule{CommandService: planCommand}
	c.StatisticsModule = &assembler.StatisticsModule{
		SyncService:              statsSync,
		BehaviorProjectorService: behaviorProjector,
	}
	c.SurveyModule = &assembler.SurveyModule{
		AnswerSheet: &assembler.AnswerSheetSubModule{SubmittedEventRelay: answerSheetRelay},
	}
	c.EvaluationModule = &assembler.EvaluationModule{AssessmentOutboxRelay: assessmentRelay}

	deps := c.BuildServerRuntimeDeps()
	if deps.LockBuilder != c.CacheBuilder(cacheplane.FamilyLock) {
		t.Fatalf("LockBuilder = %#v, want %#v", deps.LockBuilder, c.CacheBuilder(cacheplane.FamilyLock))
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
	if deps.AnswerSheetSubmittedRelay != answerSheetRelay {
		t.Fatalf("AnswerSheetSubmittedRelay = %#v, want %#v", deps.AnswerSheetSubmittedRelay, answerSheetRelay)
	}
	if deps.AssessmentOutboxRelay != assessmentRelay {
		t.Fatalf("AssessmentOutboxRelay = %#v, want %#v", deps.AssessmentOutboxRelay, assessmentRelay)
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
func (*planCommandServiceStub) OpenTask(context.Context, int64, string, planApp.OpenTaskDTO) (*planApp.TaskResult, error) {
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
func (*statisticsSyncServiceStub) SyncAccumulatedStatistics(context.Context, int64) error { return nil }
func (*statisticsSyncServiceStub) SyncPlanStatistics(context.Context, int64) error        { return nil }

type behaviorProjectorServiceStub struct{}

func (*behaviorProjectorServiceStub) ProjectBehaviorEvent(context.Context, statisticsApp.BehaviorProjectEventInput) (statisticsApp.BehaviorProjectEventResult, error) {
	return statisticsApp.BehaviorProjectEventResult{}, nil
}
func (*behaviorProjectorServiceStub) ReconcilePendingBehaviorEvents(context.Context, int) (int, error) {
	return 0, nil
}

var _ domainoperator.Repository = (*fakeOperatorRepo)(nil)
var _ appEventing.OutboxRelay = (*outboxRelayStub)(nil)
var _ planApp.PlanCommandService = (*planCommandServiceStub)(nil)
var _ statisticsApp.StatisticsSyncService = (*statisticsSyncServiceStub)(nil)
var _ statisticsApp.BehaviorProjectorService = (*behaviorProjectorServiceStub)(nil)
var _ cachegov.Coordinator = cachegov.NewCoordinator(cachegov.Config{}, cachegov.Dependencies{})
