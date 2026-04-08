package plan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type fakeLifecycleService struct {
	cancelPlanFn func(ctx context.Context, orgID int64, planID string) error
}

func (f *fakeLifecycleService) CreatePlan(context.Context, CreatePlanDTO) (*PlanResult, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLifecycleService) PausePlan(context.Context, int64, string) (*PlanResult, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLifecycleService) ResumePlan(context.Context, int64, string, map[string]string) (*PlanResult, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLifecycleService) CancelPlan(ctx context.Context, orgID int64, planID string) error {
	if f.cancelPlanFn != nil {
		return f.cancelPlanFn(ctx, orgID, planID)
	}
	return nil
}

type fakeEnrollmentService struct{}

func (f *fakeEnrollmentService) EnrollTestee(context.Context, EnrollTesteeDTO) (*EnrollmentResult, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeEnrollmentService) TerminateEnrollment(context.Context, int64, string, string) error {
	return errors.New("not implemented")
}

type fakeTaskSchedulerService struct {
	scheduleFn func(ctx context.Context, orgID int64, before string) ([]*TaskResult, error)
}

func (f *fakeTaskSchedulerService) SchedulePendingTasks(ctx context.Context, orgID int64, before string) ([]*TaskResult, error) {
	if f.scheduleFn != nil {
		return f.scheduleFn(ctx, orgID, before)
	}
	return nil, nil
}

type fakeTaskManagementService struct{}

func (f *fakeTaskManagementService) OpenTask(context.Context, int64, string, OpenTaskDTO) (*TaskResult, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTaskManagementService) CompleteTask(context.Context, int64, string, string) (*TaskResult, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTaskManagementService) ExpireTask(context.Context, int64, string) (*TaskResult, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTaskManagementService) CancelTask(context.Context, int64, string) error {
	return errors.New("not implemented")
}

type fakePlanRepo struct {
	plan *domainplan.AssessmentPlan
}

func (f *fakePlanRepo) FindByID(context.Context, domainplan.AssessmentPlanID) (*domainplan.AssessmentPlan, error) {
	if f.plan == nil {
		return nil, errors.New("plan not found")
	}
	return f.plan, nil
}

func (f *fakePlanRepo) FindByScaleCode(context.Context, string) ([]*domainplan.AssessmentPlan, error) {
	return nil, nil
}

func (f *fakePlanRepo) FindActivePlans(context.Context) ([]*domainplan.AssessmentPlan, error) {
	return nil, nil
}

func (f *fakePlanRepo) FindByTesteeID(context.Context, testee.ID) ([]*domainplan.AssessmentPlan, error) {
	return nil, nil
}

func (f *fakePlanRepo) FindList(context.Context, int64, string, string, int, int) ([]*domainplan.AssessmentPlan, int64, error) {
	return nil, 0, nil
}

func (f *fakePlanRepo) Save(context.Context, *domainplan.AssessmentPlan) error {
	return nil
}

type fakeTaskRepo struct {
	tasksByPlan map[string][]*domainplan.AssessmentTask
}

func (f *fakeTaskRepo) FindByID(context.Context, domainplan.AssessmentTaskID) (*domainplan.AssessmentTask, error) {
	return nil, errors.New("task not found")
}

func (f *fakeTaskRepo) FindByPlanID(_ context.Context, planID domainplan.AssessmentPlanID) ([]*domainplan.AssessmentTask, error) {
	return append([]*domainplan.AssessmentTask(nil), f.tasksByPlan[planID.String()]...), nil
}

func (f *fakeTaskRepo) FindByPlanIDAndTesteeIDs(context.Context, domainplan.AssessmentPlanID, []testee.ID) ([]*domainplan.AssessmentTask, error) {
	return nil, nil
}

func (f *fakeTaskRepo) FindByTesteeID(context.Context, testee.ID) ([]*domainplan.AssessmentTask, error) {
	return nil, nil
}

func (f *fakeTaskRepo) FindByTesteeIDAndPlanID(context.Context, testee.ID, domainplan.AssessmentPlanID) ([]*domainplan.AssessmentTask, error) {
	return nil, nil
}

func (f *fakeTaskRepo) FindPendingTasks(context.Context, int64, time.Time) ([]*domainplan.AssessmentTask, error) {
	return nil, nil
}

func (f *fakeTaskRepo) FindExpiredTasks(context.Context) ([]*domainplan.AssessmentTask, error) {
	return nil, nil
}

func (f *fakeTaskRepo) FindList(context.Context, int64, *domainplan.AssessmentPlanID, *testee.ID, *domainplan.TaskStatus, int, int) ([]*domainplan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (f *fakeTaskRepo) FindListByTesteeIDs(context.Context, int64, *domainplan.AssessmentPlanID, []testee.ID, *domainplan.TaskStatus, int, int) ([]*domainplan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (f *fakeTaskRepo) Save(context.Context, *domainplan.AssessmentTask) error {
	return nil
}

func (f *fakeTaskRepo) SaveBatch(context.Context, []*domainplan.AssessmentTask) error {
	return nil
}

func TestCommandServiceSchedulePendingTasksCollectsStats(t *testing.T) {
	service := NewCommandService(
		&fakeLifecycleService{},
		&fakeEnrollmentService{},
		&fakeTaskSchedulerService{
			scheduleFn: func(ctx context.Context, orgID int64, before string) ([]*TaskResult, error) {
				if orgID != 18 {
					t.Fatalf("unexpected org id: %d", orgID)
				}
				if before != "2026-04-02 10:00:00" {
					t.Fatalf("unexpected before: %s", before)
				}
				CollectTaskScheduleStats(ctx, TaskScheduleStats{
					PendingCount: 2,
					OpenedCount:  1,
					ExpiredCount: 3,
				})
				return []*TaskResult{{ID: "task-1"}}, nil
			},
		},
		&fakeTaskManagementService{},
		nil,
		nil,
	)

	result, err := service.SchedulePendingTasks(context.Background(), 18, "2026-04-02 10:00:00")
	if err != nil {
		t.Fatalf("SchedulePendingTasks returned error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected schedule result")
	}
	if len(result.Tasks) != 1 || result.Tasks[0].ID != "task-1" {
		t.Fatalf("unexpected tasks: %#v", result.Tasks)
	}
	if result.Stats.PendingCount != 2 || result.Stats.OpenedCount != 1 || result.Stats.ExpiredCount != 3 {
		t.Fatalf("unexpected stats: %#v", result.Stats)
	}
}

func TestCommandServiceCancelPlanCountsAffectedTasks(t *testing.T) {
	planAggregate, err := domainplan.NewAssessmentPlan(7, "scale-code", domainplan.PlanScheduleByWeek, 1, 3)
	if err != nil {
		t.Fatalf("failed to create plan aggregate: %v", err)
	}

	testeeID := testee.ID(meta.ID(1001))
	taskLifecycle := domainplan.NewTaskLifecycle()

	pendingTask := domainplan.NewAssessmentTask(planAggregate.GetID(), 1, 7, testeeID, "scale-code", time.Now())
	openedTask := domainplan.NewAssessmentTask(planAggregate.GetID(), 2, 7, testeeID, "scale-code", time.Now().Add(time.Hour))
	completedTask := domainplan.NewAssessmentTask(planAggregate.GetID(), 3, 7, testeeID, "scale-code", time.Now().Add(2*time.Hour))

	if err := taskLifecycle.Open(context.Background(), openedTask, "token-open", "https://example.com/open", time.Now().Add(4*time.Hour), ""); err != nil {
		t.Fatalf("failed to open task: %v", err)
	}
	if err := taskLifecycle.Open(context.Background(), completedTask, "token-complete", "https://example.com/complete", time.Now().Add(4*time.Hour), ""); err != nil {
		t.Fatalf("failed to open completed task: %v", err)
	}
	if err := taskLifecycle.Complete(context.Background(), completedTask, 9001); err != nil {
		t.Fatalf("failed to complete task: %v", err)
	}

	called := false
	service := NewCommandService(
		&fakeLifecycleService{
			cancelPlanFn: func(ctx context.Context, orgID int64, planID string) error {
				called = true
				if orgID != 7 || planID != planAggregate.GetID().String() {
					t.Fatalf("unexpected cancel inputs: org=%d plan=%s", orgID, planID)
				}
				return nil
			},
		},
		&fakeEnrollmentService{},
		&fakeTaskSchedulerService{},
		&fakeTaskManagementService{},
		&fakePlanRepo{plan: planAggregate},
		&fakeTaskRepo{
			tasksByPlan: map[string][]*domainplan.AssessmentTask{
				planAggregate.GetID().String(): {pendingTask, openedTask, completedTask},
			},
		},
	)

	result, err := service.CancelPlan(context.Background(), 7, planAggregate.GetID().String())
	if err != nil {
		t.Fatalf("CancelPlan returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected lifecycle cancel to be called")
	}
	if result == nil {
		t.Fatalf("expected cancel result")
	}
	if result.AffectedTaskCount != 2 {
		t.Fatalf("unexpected affected task count: %d", result.AffectedTaskCount)
	}
}
