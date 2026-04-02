package plan

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

type enrollmentPlanRepoStub struct {
	plan *AssessmentPlan
}

func (r *enrollmentPlanRepoStub) FindByID(context.Context, AssessmentPlanID) (*AssessmentPlan, error) {
	return r.plan, nil
}

func (r *enrollmentPlanRepoStub) FindByScaleCode(context.Context, string) ([]*AssessmentPlan, error) {
	return nil, nil
}

func (r *enrollmentPlanRepoStub) FindActivePlans(context.Context) ([]*AssessmentPlan, error) {
	return nil, nil
}

func (r *enrollmentPlanRepoStub) FindByTesteeID(context.Context, testee.ID) ([]*AssessmentPlan, error) {
	return nil, nil
}

func (r *enrollmentPlanRepoStub) FindList(context.Context, int64, string, string, int, int) ([]*AssessmentPlan, int64, error) {
	return nil, 0, nil
}

func (r *enrollmentPlanRepoStub) Save(context.Context, *AssessmentPlan) error {
	return nil
}

func TestPlanEnrollmentEnrollTesteeIsIdempotent(t *testing.T) {
	ctx := context.Background()
	startDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)

	p, err := NewAssessmentPlan(1, "scale-code", PlanScheduleByWeek, 1, 3)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}

	testeeID := testee.NewID(3001)
	existingTasks := NewTaskGenerator().GenerateTasks(p, testeeID, startDate)
	taskRepo := &lifecycleTaskRepoStub{tasks: existingTasks}
	enrollment := NewPlanEnrollment(&enrollmentPlanRepoStub{plan: p}, taskRepo, NewTaskGenerator(), NewPlanValidator())

	result, err := enrollment.EnrollTestee(ctx, p.GetID(), testeeID, startDate)
	if err != nil {
		t.Fatalf("EnrollTestee returned error: %v", err)
	}

	if !result.Idempotent {
		t.Fatalf("expected enrollment to be idempotent")
	}
	if len(result.TasksToSave) != 0 {
		t.Fatalf("expected no new tasks to save, got %d", len(result.TasksToSave))
	}
	if len(result.Tasks) != len(existingTasks) {
		t.Fatalf("expected %d tasks, got %d", len(existingTasks), len(result.Tasks))
	}
	for i := range existingTasks {
		if result.Tasks[i].GetID() != existingTasks[i].GetID() {
			t.Fatalf("expected existing task ID %s, got %s", existingTasks[i].GetID().String(), result.Tasks[i].GetID().String())
		}
	}
}

func TestPlanEnrollmentEnrollTesteeRejectsDifferentStartDateForExistingEnrollment(t *testing.T) {
	ctx := context.Background()
	originalStartDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)
	retryStartDate := originalStartDate.AddDate(0, 0, 7)

	p, err := NewAssessmentPlan(1, "scale-code", PlanScheduleByWeek, 1, 2)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}

	testeeID := testee.NewID(3002)
	existingTasks := NewTaskGenerator().GenerateTasks(p, testeeID, originalStartDate)
	taskRepo := &lifecycleTaskRepoStub{tasks: existingTasks}
	enrollment := NewPlanEnrollment(&enrollmentPlanRepoStub{plan: p}, taskRepo, NewTaskGenerator(), NewPlanValidator())

	if _, err := enrollment.EnrollTestee(ctx, p.GetID(), testeeID, retryStartDate); err == nil {
		t.Fatalf("expected different start date to be rejected for existing enrollment")
	}
}

func TestPlanEnrollmentEnrollTesteeCreatesOnlyMissingTasks(t *testing.T) {
	ctx := context.Background()
	startDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)

	p, err := NewAssessmentPlan(1, "scale-code", PlanScheduleByWeek, 1, 3)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}

	testeeID := testee.NewID(3003)
	allTasks := NewTaskGenerator().GenerateTasks(p, testeeID, startDate)
	taskRepo := &lifecycleTaskRepoStub{tasks: []*AssessmentTask{allTasks[0], allTasks[1]}}
	enrollment := NewPlanEnrollment(&enrollmentPlanRepoStub{plan: p}, taskRepo, NewTaskGenerator(), NewPlanValidator())

	result, err := enrollment.EnrollTestee(ctx, p.GetID(), testeeID, startDate)
	if err != nil {
		t.Fatalf("EnrollTestee returned error: %v", err)
	}

	if result.Idempotent {
		t.Fatalf("expected partial enrollment to require saving missing tasks")
	}
	if len(result.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(result.Tasks))
	}
	if len(result.TasksToSave) != 1 {
		t.Fatalf("expected exactly 1 missing task to save, got %d", len(result.TasksToSave))
	}
	if result.TasksToSave[0].GetSeq() != 3 {
		t.Fatalf("expected missing task seq 3, got %d", result.TasksToSave[0].GetSeq())
	}
}
