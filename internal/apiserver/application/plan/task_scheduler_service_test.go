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
	pendingTasks        []*domainPlan.AssessmentTask
	scopedTasks         []*domainPlan.AssessmentTask
	expiredTasks        []*domainPlan.AssessmentTask
	savedTasks          []*domainPlan.AssessmentTask
	findPendingCalled   bool
	findScopedCalled    bool
	lastScopedPlanID    domainPlan.AssessmentPlanID
	lastScopedTesteeIDs []testee.ID
}

func (r *schedulerTaskRepoStub) FindByID(context.Context, domainPlan.AssessmentTaskID) (*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *schedulerTaskRepoStub) FindByPlanID(context.Context, domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *schedulerTaskRepoStub) FindByPlanIDAndTesteeIDs(_ context.Context, planID domainPlan.AssessmentPlanID, testeeIDs []testee.ID) ([]*domainPlan.AssessmentTask, error) {
	r.findScopedCalled = true
	r.lastScopedPlanID = planID
	r.lastScopedTesteeIDs = append([]testee.ID(nil), testeeIDs...)
	return r.scopedTasks, nil
}

func (r *schedulerTaskRepoStub) FindByTesteeID(context.Context, testee.ID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *schedulerTaskRepoStub) FindByTesteeIDAndPlanID(context.Context, testee.ID, domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *schedulerTaskRepoStub) FindPendingTasks(context.Context, int64, time.Time) ([]*domainPlan.AssessmentTask, error) {
	r.findPendingCalled = true
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

func TestTaskSchedulerServiceSchedulesScopedTasksWithoutGlobalPendingScan(t *testing.T) {
	p, err := domainPlan.NewAssessmentPlan(1, "scale-code", domainPlan.PlanScheduleByWeek, 1, 1)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}

	before := time.Now()
	scopedTask := domainPlan.NewAssessmentTask(
		p.GetID(),
		1,
		1,
		testee.NewID(3001),
		"scale-code",
		before.Add(-time.Minute),
	)
	futureScopedTask := domainPlan.NewAssessmentTask(
		p.GetID(),
		1,
		2,
		testee.NewID(3001),
		"scale-code",
		before.Add(time.Hour),
	)

	taskRepo := &schedulerTaskRepoStub{
		pendingTasks: []*domainPlan.AssessmentTask{
			domainPlan.NewAssessmentTask(p.GetID(), 1, 9, testee.NewID(9999), "scale-code", before.Add(-time.Minute)),
		},
		scopedTasks: []*domainPlan.AssessmentTask{scopedTask, futureScopedTask},
	}
	planRepo := &schedulerPlanRepoByIDStub{plan: p}
	entryGenerator := &entryGeneratorStub{}

	service := NewTaskSchedulerService(taskRepo, planRepo, entryGenerator, event.NewNopEventPublisher())
	ctx := WithTaskSchedulerScope(context.Background(), p.GetID().String(), []string{"3001"})
	results, err := service.SchedulePendingTasks(ctx, 1, before.Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("SchedulePendingTasks returned error: %v", err)
	}

	if taskRepo.findPendingCalled {
		t.Fatalf("expected scoped scheduling to avoid global pending scan")
	}
	if !taskRepo.findScopedCalled {
		t.Fatalf("expected scoped scheduling to query scoped tasks")
	}
	if taskRepo.lastScopedPlanID != p.GetID() {
		t.Fatalf("expected scoped plan id %s, got %s", p.GetID().String(), taskRepo.lastScopedPlanID.String())
	}
	if len(taskRepo.lastScopedTesteeIDs) != 1 || taskRepo.lastScopedTesteeIDs[0] != testee.NewID(3001) {
		t.Fatalf("expected scoped testee ids [3001], got %+v", taskRepo.lastScopedTesteeIDs)
	}
	if len(results) != 1 {
		t.Fatalf("expected only one due scoped task to be opened, got %d", len(results))
	}
	if results[0].ID != scopedTask.GetID().String() {
		t.Fatalf("expected opened task %s, got %s", scopedTask.GetID().String(), results[0].ID)
	}
}
