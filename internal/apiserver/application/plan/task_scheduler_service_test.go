package plan

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type schedulerTaskRepoStub struct {
	pendingTasks []*domainPlan.AssessmentTask
	expiredTasks []*domainPlan.AssessmentTask
	savedTasks   []*domainPlan.AssessmentTask
}

func (r *schedulerTaskRepoStub) FindByID(context.Context, domainPlan.AssessmentTaskID) (*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *schedulerTaskRepoStub) FindByPlanID(context.Context, domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *schedulerTaskRepoStub) FindByPlanIDAndTesteeIDs(context.Context, domainPlan.AssessmentPlanID, []testee.ID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *schedulerTaskRepoStub) FindByTesteeID(context.Context, testee.ID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *schedulerTaskRepoStub) FindByTesteeIDAndPlanID(context.Context, testee.ID, domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *schedulerTaskRepoStub) FindPendingTasks(context.Context, int64, time.Time) ([]*domainPlan.AssessmentTask, error) {
	return r.pendingTasks, nil
}

func (r *schedulerTaskRepoStub) FindExpiredTasks(context.Context) ([]*domainPlan.AssessmentTask, error) {
	return r.expiredTasks, nil
}

func (r *schedulerTaskRepoStub) FindList(context.Context, int64, *domainPlan.AssessmentPlanID, *testee.ID, *domainPlan.TaskStatus, int, int) ([]*domainPlan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (r *schedulerTaskRepoStub) FindListByTesteeIDs(context.Context, int64, *domainPlan.AssessmentPlanID, []testee.ID, *domainPlan.TaskStatus, int, int) ([]*domainPlan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (r *schedulerTaskRepoStub) Save(_ context.Context, task *domainPlan.AssessmentTask) error {
	r.savedTasks = append(r.savedTasks, task)
	return nil
}

func (r *schedulerTaskRepoStub) SaveBatch(context.Context, []*domainPlan.AssessmentTask) error {
	return nil
}

type schedulerPlanRepoByIDStub struct {
	plan *domainPlan.AssessmentPlan
}

func (r *schedulerPlanRepoByIDStub) FindByID(_ context.Context, id domainPlan.AssessmentPlanID) (*domainPlan.AssessmentPlan, error) {
	if r.plan != nil && r.plan.GetID() == id {
		return r.plan, nil
	}
	return nil, nil
}

func (r *schedulerPlanRepoByIDStub) FindByScaleCode(context.Context, string) ([]*domainPlan.AssessmentPlan, error) {
	return nil, nil
}

func (r *schedulerPlanRepoByIDStub) FindActivePlans(context.Context) ([]*domainPlan.AssessmentPlan, error) {
	return nil, nil
}

func (r *schedulerPlanRepoByIDStub) FindByTesteeID(context.Context, testee.ID) ([]*domainPlan.AssessmentPlan, error) {
	return nil, nil
}

func (r *schedulerPlanRepoByIDStub) FindList(context.Context, int64, string, string, int, int) ([]*domainPlan.AssessmentPlan, int64, error) {
	return nil, 0, nil
}

func (r *schedulerPlanRepoByIDStub) Save(context.Context, *domainPlan.AssessmentPlan) error {
	return nil
}

type entryGeneratorStub struct {
	calls int
}

func (g *entryGeneratorStub) GenerateEntry(context.Context, *domainPlan.AssessmentTask) (string, string, time.Time, error) {
	g.calls++
	return "token", "https://example.com/entry", time.Now().Add(time.Hour), nil
}

func TestTaskSchedulerServiceCancelsPendingTaskForInactivePlan(t *testing.T) {
	p, err := domainPlan.NewAssessmentPlan(1, "scale-code", domainPlan.PlanScheduleByWeek, 1, 1)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}
	p.RestoreFromRepository(p.GetID(), domainPlan.PlanStatusCanceled)

	task := domainPlan.NewAssessmentTask(
		p.GetID(),
		1,
		1,
		testee.NewID(2001),
		"scale-code",
		time.Now().Add(-time.Minute),
	)

	taskRepo := &schedulerTaskRepoStub{
		pendingTasks: []*domainPlan.AssessmentTask{task},
	}
	planRepo := &schedulerPlanRepoByIDStub{plan: p}
	entryGenerator := &entryGeneratorStub{}

	service := NewTaskSchedulerService(taskRepo, planRepo, entryGenerator, event.NewNopEventPublisher())
	results, err := service.SchedulePendingTasks(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("SchedulePendingTasks returned error: %v", err)
	}

	if len(results) != 0 {
		t.Fatalf("expected no opened tasks, got %d", len(results))
	}
	if entryGenerator.calls != 0 {
		t.Fatalf("expected entry generator to be skipped, got %d calls", entryGenerator.calls)
	}
	if !task.IsCanceled() {
		t.Fatalf("expected pending task to be canceled when parent plan is inactive")
	}
	if len(taskRepo.savedTasks) != 1 || taskRepo.savedTasks[0] != task {
		t.Fatalf("expected canceled task to be persisted once")
	}
}
