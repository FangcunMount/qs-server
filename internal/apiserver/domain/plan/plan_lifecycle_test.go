package plan

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type lifecycleTaskRepoStub struct {
	tasks []*AssessmentTask
}

func (r *lifecycleTaskRepoStub) FindByID(context.Context, AssessmentTaskID) (*AssessmentTask, error) {
	return nil, nil
}

func (r *lifecycleTaskRepoStub) FindByPlanID(context.Context, AssessmentPlanID) ([]*AssessmentTask, error) {
	return r.tasks, nil
}

func (r *lifecycleTaskRepoStub) FindByPlanIDAndTesteeIDs(context.Context, AssessmentPlanID, []testee.ID) ([]*AssessmentTask, error) {
	return nil, nil
}

func (r *lifecycleTaskRepoStub) FindByTesteeID(context.Context, testee.ID) ([]*AssessmentTask, error) {
	return r.tasks, nil
}

func (r *lifecycleTaskRepoStub) FindByTesteeIDAndPlanID(_ context.Context, testeeID testee.ID, planID AssessmentPlanID) ([]*AssessmentTask, error) {
	var filtered []*AssessmentTask
	for _, task := range r.tasks {
		if task.GetTesteeID() == testeeID && task.GetPlanID() == planID {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

func (r *lifecycleTaskRepoStub) FindPendingTasks(context.Context, int64, time.Time) ([]*AssessmentTask, error) {
	return nil, nil
}

func (r *lifecycleTaskRepoStub) FindExpiredTasks(context.Context) ([]*AssessmentTask, error) {
	return nil, nil
}

func (r *lifecycleTaskRepoStub) FindList(context.Context, int64, *AssessmentPlanID, *testee.ID, *TaskStatus, int, int) ([]*AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (r *lifecycleTaskRepoStub) FindListByTesteeIDs(context.Context, int64, *AssessmentPlanID, []testee.ID, *TaskStatus, int, int) ([]*AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (r *lifecycleTaskRepoStub) FindWindow(context.Context, int64, AssessmentPlanID, []testee.ID, *TaskStatus, *time.Time, int, int) ([]*AssessmentTask, bool, error) {
	return nil, false, nil
}

func (r *lifecycleTaskRepoStub) Save(context.Context, *AssessmentTask) error {
	return nil
}

func (r *lifecycleTaskRepoStub) SaveBatch(context.Context, []*AssessmentTask) error {
	return nil
}

func TestPlanLifecycleCancelCancelsOutstandingTasks(t *testing.T) {
	ctx := context.Background()
	p, err := NewAssessmentPlan(1, "scale-code", PlanScheduleByWeek, 1, 4)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}

	testeeID := testee.NewID(1001)
	pendingTask := NewAssessmentTask(p.GetID(), 1, 1, testeeID, "scale-code", time.Now())
	openedTask := NewAssessmentTask(p.GetID(), 2, 1, testeeID, "scale-code", time.Now().Add(time.Hour))
	completedTask := NewAssessmentTask(p.GetID(), 3, 1, testeeID, "scale-code", time.Now().Add(2*time.Hour))
	expiredTask := NewAssessmentTask(p.GetID(), 4, 1, testeeID, "scale-code", time.Now().Add(3*time.Hour))

	taskLifecycle := NewTaskLifecycle()
	if err := taskLifecycle.Open(ctx, openedTask, "open-token", "https://example.com/open", time.Now().Add(6*time.Hour)); err != nil {
		t.Fatalf("failed to open task: %v", err)
	}
	if err := taskLifecycle.Open(ctx, completedTask, "completed-token", "https://example.com/completed", time.Now().Add(6*time.Hour)); err != nil {
		t.Fatalf("failed to open task: %v", err)
	}
	if err := taskLifecycle.Complete(ctx, completedTask, assessment.NewID(9001)); err != nil {
		t.Fatalf("failed to complete task: %v", err)
	}
	if err := taskLifecycle.Open(ctx, expiredTask, "expired-token", "https://example.com/expired", time.Now().Add(2*time.Hour)); err != nil {
		t.Fatalf("failed to open task: %v", err)
	}
	if err := taskLifecycle.Expire(ctx, expiredTask); err != nil {
		t.Fatalf("failed to expire task: %v", err)
	}

	repo := &lifecycleTaskRepoStub{
		tasks: []*AssessmentTask{pendingTask, openedTask, completedTask, expiredTask},
	}
	lifecycle := NewPlanLifecycle(repo, NewTaskGenerator(), taskLifecycle)

	canceledTasks, err := lifecycle.Cancel(ctx, p)
	if err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}

	if !p.IsCanceled() {
		t.Fatalf("expected plan status canceled, got %s", p.GetStatus())
	}
	if len(canceledTasks) != 2 {
		t.Fatalf("expected 2 canceled tasks, got %d", len(canceledTasks))
	}
	if !pendingTask.IsCanceled() {
		t.Fatalf("expected pending task to be canceled")
	}
	if !openedTask.IsCanceled() {
		t.Fatalf("expected opened task to be canceled")
	}
	if !completedTask.IsCompleted() {
		t.Fatalf("expected completed task to remain completed")
	}
	if !expiredTask.IsExpired() {
		t.Fatalf("expected expired task to remain expired")
	}
}

func TestPlanLifecycleFinishFinishesPlanAndCancelsOutstandingTasks(t *testing.T) {
	ctx := context.Background()
	p, err := NewAssessmentPlan(1, "scale-code", PlanScheduleByWeek, 1, 4)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}

	testeeID := testee.NewID(1011)
	pendingTask := NewAssessmentTask(p.GetID(), 1, 1, testeeID, "scale-code", time.Now())
	openedTask := NewAssessmentTask(p.GetID(), 2, 1, testeeID, "scale-code", time.Now().Add(time.Hour))
	completedTask := NewAssessmentTask(p.GetID(), 3, 1, testeeID, "scale-code", time.Now().Add(2*time.Hour))

	taskLifecycle := NewTaskLifecycle()
	if err := taskLifecycle.Open(ctx, openedTask, "open-token", "https://example.com/open", time.Now().Add(6*time.Hour)); err != nil {
		t.Fatalf("failed to open task: %v", err)
	}
	if err := taskLifecycle.Open(ctx, completedTask, "completed-token", "https://example.com/completed", time.Now().Add(6*time.Hour)); err != nil {
		t.Fatalf("failed to open task: %v", err)
	}
	if err := taskLifecycle.Complete(ctx, completedTask, assessment.NewID(9011)); err != nil {
		t.Fatalf("failed to complete task: %v", err)
	}

	repo := &lifecycleTaskRepoStub{
		tasks: []*AssessmentTask{pendingTask, openedTask, completedTask},
	}
	lifecycle := NewPlanLifecycle(repo, NewTaskGenerator(), taskLifecycle)

	canceledTasks, err := lifecycle.Finish(ctx, p)
	if err != nil {
		t.Fatalf("Finish returned error: %v", err)
	}

	if !p.IsFinished() {
		t.Fatalf("expected plan status finished, got %s", p.GetStatus())
	}
	if len(canceledTasks) != 2 {
		t.Fatalf("expected 2 canceled tasks, got %d", len(canceledTasks))
	}
	if !pendingTask.IsCanceled() {
		t.Fatalf("expected pending task to be canceled")
	}
	if !openedTask.IsCanceled() {
		t.Fatalf("expected opened task to be canceled")
	}
	if !completedTask.IsCompleted() {
		t.Fatalf("expected completed task to remain completed")
	}
}

func TestPlanLifecycleResumeReusesCanceledTasks(t *testing.T) {
	ctx := context.Background()
	startDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)

	p, err := NewAssessmentPlan(1, "scale-code", PlanScheduleByWeek, 1, 3)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}
	if err := p.pause(); err != nil {
		t.Fatalf("failed to pause plan: %v", err)
	}

	testeeID := testee.NewID(1002)
	taskLifecycle := NewTaskLifecycle()

	completedTask := NewAssessmentTask(p.GetID(), 1, 1, testeeID, "scale-code", startDate)
	if err := taskLifecycle.Open(ctx, completedTask, "completed-token", "https://example.com/completed", time.Now().Add(6*time.Hour)); err != nil {
		t.Fatalf("failed to open completed task: %v", err)
	}
	if err := taskLifecycle.Complete(ctx, completedTask, assessment.NewID(9101)); err != nil {
		t.Fatalf("failed to complete task: %v", err)
	}

	canceledTask2 := NewAssessmentTask(p.GetID(), 2, 1, testeeID, "scale-code", startDate.AddDate(0, 0, 7))
	if err := taskLifecycle.Cancel(ctx, canceledTask2); err != nil {
		t.Fatalf("failed to cancel task 2: %v", err)
	}
	originalTask2ID := canceledTask2.GetID()

	canceledTask3 := NewAssessmentTask(p.GetID(), 3, 1, testeeID, "scale-code", startDate.AddDate(0, 0, 14))
	if err := taskLifecycle.Cancel(ctx, canceledTask3); err != nil {
		t.Fatalf("failed to cancel task 3: %v", err)
	}
	originalTask3ID := canceledTask3.GetID()

	repo := &lifecycleTaskRepoStub{
		tasks: []*AssessmentTask{completedTask, canceledTask2, canceledTask3},
	}
	lifecycle := NewPlanLifecycle(repo, NewTaskGenerator(), taskLifecycle)

	resumeResult, err := lifecycle.Resume(ctx, p, map[testee.ID]time.Time{testeeID: startDate})
	if err != nil {
		t.Fatalf("Resume returned error: %v", err)
	}

	if !p.IsActive() {
		t.Fatalf("expected plan to become active after resume, got %s", p.GetStatus())
	}
	if len(resumeResult.TasksToSave) != 2 {
		t.Fatalf("expected 2 tasks to save after resume, got %d", len(resumeResult.TasksToSave))
	}
	if canceledTask2.GetID() != originalTask2ID || canceledTask3.GetID() != originalTask3ID {
		t.Fatalf("expected resume to reuse existing task IDs")
	}
	if !canceledTask2.IsPending() || !canceledTask3.IsPending() {
		t.Fatalf("expected canceled tasks to be reset to pending")
	}
	if canceledTask2.GetOpenAt() != nil || canceledTask2.GetExpireAt() != nil || canceledTask2.GetEntryURL() != "" {
		t.Fatalf("expected task 2 runtime state to be cleared on resume")
	}
	if canceledTask3.GetOpenAt() != nil || canceledTask3.GetExpireAt() != nil || canceledTask3.GetEntryURL() != "" {
		t.Fatalf("expected task 3 runtime state to be cleared on resume")
	}
}

func TestAssessmentPlanLifecycleEmitsNoPlanEvents(t *testing.T) {
	p, err := NewAssessmentPlan(1, "scale-code", PlanScheduleByWeek, 1, 2)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}

	if len(p.Events()) != 0 {
		t.Fatalf("expected no plan lifecycle events on create, got %d", len(p.Events()))
	}

	if err := p.pause(); err != nil {
		t.Fatalf("pause returned error: %v", err)
	}
	if len(p.Events()) != 0 {
		t.Fatalf("expected no plan lifecycle events on pause, got %d", len(p.Events()))
	}

	if err := p.resume(); err != nil {
		t.Fatalf("resume returned error: %v", err)
	}
	if len(p.Events()) != 0 {
		t.Fatalf("expected no plan lifecycle events on resume, got %d", len(p.Events()))
	}

	p.finish()
	if len(p.Events()) != 0 {
		t.Fatalf("expected no plan lifecycle events on finish, got %d", len(p.Events()))
	}

	canceledPlan, err := NewAssessmentPlan(1, "scale-code", PlanScheduleByWeek, 1, 1)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}

	canceledPlan.cancel()
	if len(canceledPlan.Events()) != 0 {
		t.Fatalf("expected no plan lifecycle events on cancel, got %d", len(canceledPlan.Events()))
	}
}

func TestTaskLifecycleCancelRaisesCanceledEvent(t *testing.T) {
	task := NewAssessmentTask(
		NewAssessmentPlanID(),
		1,
		1,
		testee.NewID(3001),
		"scale-code",
		time.Now(),
	)
	taskLifecycle := NewTaskLifecycle()

	if err := taskLifecycle.Cancel(context.Background(), task); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}

	if !task.IsCanceled() {
		t.Fatalf("expected task to be canceled")
	}
	assertSingleEventType(t, task.Events(), EventTypeTaskCanceled)
}

func TestAssessmentPlanRestoreFromRepositoryClearsEvents(t *testing.T) {
	p, err := NewAssessmentPlan(1, "scale-code", PlanScheduleByWeek, 1, 1)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}
	p.pause()

	p.RestoreFromRepository(p.GetID(), PlanStatusActive)

	if len(p.Events()) != 0 {
		t.Fatalf("expected RestoreFromRepository to clear pending events, got %d", len(p.Events()))
	}
}

func assertSingleEventType(t *testing.T, events []event.DomainEvent, want string) {
	t.Helper()

	if len(events) != 1 {
		t.Fatalf("expected exactly 1 event, got %d", len(events))
	}
	if events[0].EventType() != want {
		t.Fatalf("expected event type %q, got %q", want, events[0].EventType())
	}
}
