package plan

import (
	"context"
	"testing"
	"time"

	testeeDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type enrollmentPlanRepoStub struct {
	plan *domainPlan.AssessmentPlan
}

func (r *enrollmentPlanRepoStub) FindByID(context.Context, domainPlan.AssessmentPlanID) (*domainPlan.AssessmentPlan, error) {
	return r.plan, nil
}

func (r *enrollmentPlanRepoStub) FindByScaleCode(context.Context, string) ([]*domainPlan.AssessmentPlan, error) {
	return nil, nil
}

func (r *enrollmentPlanRepoStub) FindActivePlans(context.Context) ([]*domainPlan.AssessmentPlan, error) {
	return nil, nil
}

func (r *enrollmentPlanRepoStub) FindByTesteeID(context.Context, testeeDomain.ID) ([]*domainPlan.AssessmentPlan, error) {
	return nil, nil
}

func (r *enrollmentPlanRepoStub) FindList(context.Context, int64, string, string, int, int) ([]*domainPlan.AssessmentPlan, int64, error) {
	return nil, 0, nil
}

func (r *enrollmentPlanRepoStub) Save(context.Context, *domainPlan.AssessmentPlan) error {
	return nil
}

type enrollmentTaskRepoStub struct {
	existingEnrollmentTasks []*domainPlan.AssessmentTask
	planTasks               []*domainPlan.AssessmentTask
	savedBatch              []*domainPlan.AssessmentTask
	saved                   []*domainPlan.AssessmentTask
}

func (r *enrollmentTaskRepoStub) FindByID(context.Context, domainPlan.AssessmentTaskID) (*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *enrollmentTaskRepoStub) FindByPlanID(context.Context, domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	return append([]*domainPlan.AssessmentTask(nil), r.planTasks...), nil
}

func (r *enrollmentTaskRepoStub) FindByPlanIDAndTesteeIDs(context.Context, domainPlan.AssessmentPlanID, []testeeDomain.ID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *enrollmentTaskRepoStub) FindByTesteeID(context.Context, testeeDomain.ID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *enrollmentTaskRepoStub) FindByTesteeIDAndPlanID(context.Context, testeeDomain.ID, domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	return append([]*domainPlan.AssessmentTask(nil), r.existingEnrollmentTasks...), nil
}

func (r *enrollmentTaskRepoStub) FindPendingTasks(context.Context, int64, time.Time) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *enrollmentTaskRepoStub) FindExpiredTasks(context.Context) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *enrollmentTaskRepoStub) FindList(context.Context, int64, *domainPlan.AssessmentPlanID, *testeeDomain.ID, *domainPlan.TaskStatus, int, int) ([]*domainPlan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (r *enrollmentTaskRepoStub) FindListByTesteeIDs(context.Context, int64, *domainPlan.AssessmentPlanID, []testeeDomain.ID, *domainPlan.TaskStatus, int, int) ([]*domainPlan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (r *enrollmentTaskRepoStub) FindWindow(context.Context, int64, domainPlan.AssessmentPlanID, []testeeDomain.ID, *domainPlan.TaskStatus, *time.Time, int, int) ([]*domainPlan.AssessmentTask, bool, error) {
	return nil, false, nil
}

func (r *enrollmentTaskRepoStub) Save(_ context.Context, task *domainPlan.AssessmentTask) error {
	r.saved = append(r.saved, task)
	return nil
}

func (r *enrollmentTaskRepoStub) SaveBatch(_ context.Context, tasks []*domainPlan.AssessmentTask) error {
	r.savedBatch = append([]*domainPlan.AssessmentTask(nil), tasks...)
	return nil
}

type enrollmentEventPublisherStub struct {
	events []event.DomainEvent
}

func (p *enrollmentEventPublisherStub) Publish(_ context.Context, evt event.DomainEvent) error {
	p.events = append(p.events, evt)
	return nil
}

func (p *enrollmentEventPublisherStub) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

func TestEnrollmentServiceDoesNotPublishPlanEventOnEnroll(t *testing.T) {
	ctx := context.Background()
	planAggregate, err := domainPlan.NewAssessmentPlan(9, "scale-code", domainPlan.PlanScheduleByWeek, 1, 2)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}
	planAggregate.ClearEvents()

	taskRepo := &enrollmentTaskRepoStub{}
	publisher := &enrollmentEventPublisherStub{}
	service := NewEnrollmentService(
		&enrollmentPlanRepoStub{plan: planAggregate},
		taskRepo,
		publisher,
	)

	result, err := service.EnrollTestee(ctx, EnrollTesteeDTO{
		OrgID:     9,
		PlanID:    planAggregate.GetID().String(),
		TesteeID:  "3001",
		StartDate: "2026-04-03",
	})
	if err != nil {
		t.Fatalf("EnrollTestee returned error: %v", err)
	}
	if result.CreatedTaskCount != 2 || result.Idempotent {
		t.Fatalf("unexpected enrollment result: %#v", result)
	}
	if len(taskRepo.savedBatch) != 2 {
		t.Fatalf("expected SaveBatch to persist 2 tasks, got %d", len(taskRepo.savedBatch))
	}
	if len(publisher.events) != 0 {
		t.Fatalf("expected no plan lifecycle event on enroll, got %d", len(publisher.events))
	}
}

func TestEnrollmentServiceSchedulesGeneratedTasksAtSevenPM(t *testing.T) {
	ctx := context.Background()
	planAggregate, err := domainPlan.NewAssessmentPlan(9, "scale-code", domainPlan.PlanScheduleByDay, 1, 2)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}
	planAggregate.ClearEvents()

	taskRepo := &enrollmentTaskRepoStub{}
	service := NewEnrollmentService(
		&enrollmentPlanRepoStub{plan: planAggregate},
		taskRepo,
		&enrollmentEventPublisherStub{},
	)

	result, err := service.EnrollTestee(ctx, EnrollTesteeDTO{
		OrgID:     9,
		PlanID:    planAggregate.GetID().String(),
		TesteeID:  "3010",
		StartDate: "2026-04-03",
	})
	if err != nil {
		t.Fatalf("EnrollTestee returned error: %v", err)
	}

	wantPlannedAt := []string{
		"2026-04-03 19:00:00",
		"2026-04-04 19:00:00",
	}
	if len(result.Tasks) != len(wantPlannedAt) {
		t.Fatalf("expected %d tasks, got %d", len(wantPlannedAt), len(result.Tasks))
	}
	for i, want := range wantPlannedAt {
		if result.Tasks[i].PlannedAt != want {
			t.Fatalf("expected task %d planned_at %s, got %s", i+1, want, result.Tasks[i].PlannedAt)
		}
	}
}

func TestEnrollmentServiceSchedulesGeneratedTasksUsingPlanTriggerTime(t *testing.T) {
	ctx := context.Background()
	planAggregate, err := domainPlan.NewAssessmentPlan(
		9,
		"scale-code",
		domainPlan.PlanScheduleByDay,
		1,
		2,
		domainPlan.WithTriggerTime("08:30"),
	)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}
	planAggregate.ClearEvents()

	taskRepo := &enrollmentTaskRepoStub{}
	service := NewEnrollmentService(
		&enrollmentPlanRepoStub{plan: planAggregate},
		taskRepo,
		&enrollmentEventPublisherStub{},
	)

	result, err := service.EnrollTestee(ctx, EnrollTesteeDTO{
		OrgID:     9,
		PlanID:    planAggregate.GetID().String(),
		TesteeID:  "3011",
		StartDate: "2026-04-03",
	})
	if err != nil {
		t.Fatalf("EnrollTestee returned error: %v", err)
	}

	wantPlannedAt := []string{
		"2026-04-03 08:30:00",
		"2026-04-04 08:30:00",
	}
	if len(result.Tasks) != len(wantPlannedAt) {
		t.Fatalf("expected %d tasks, got %d", len(wantPlannedAt), len(result.Tasks))
	}
	for i, want := range wantPlannedAt {
		if result.Tasks[i].PlannedAt != want {
			t.Fatalf("expected task %d planned_at %s, got %s", i+1, want, result.Tasks[i].PlannedAt)
		}
	}
}

func TestEnrollmentServiceIdempotentEnrollPublishesNoPlanEvent(t *testing.T) {
	ctx := context.Background()
	planAggregate, err := domainPlan.NewAssessmentPlan(9, "scale-code", domainPlan.PlanScheduleByWeek, 1, 2)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}
	planAggregate.ClearEvents()

	testeeID := testeeDomain.NewID(3002)
	startDate, err := parseDate("2026-04-03")
	if err != nil {
		t.Fatalf("parseDate returned error: %v", err)
	}
	existingTasks := domainPlan.NewTaskGenerator().GenerateTasks(planAggregate, testeeID, startDate)

	taskRepo := &enrollmentTaskRepoStub{existingEnrollmentTasks: existingTasks}
	publisher := &enrollmentEventPublisherStub{}
	service := NewEnrollmentService(
		&enrollmentPlanRepoStub{plan: planAggregate},
		taskRepo,
		publisher,
	)

	result, err := service.EnrollTestee(ctx, EnrollTesteeDTO{
		OrgID:     9,
		PlanID:    planAggregate.GetID().String(),
		TesteeID:  "3002",
		StartDate: "2026-04-03",
	})
	if err != nil {
		t.Fatalf("EnrollTestee returned error: %v", err)
	}
	if !result.Idempotent || result.CreatedTaskCount != 0 {
		t.Fatalf("unexpected idempotent enrollment result: %#v", result)
	}
	if len(taskRepo.savedBatch) != 0 {
		t.Fatalf("expected no SaveBatch call for idempotent enroll, got %d tasks", len(taskRepo.savedBatch))
	}
	if len(publisher.events) != 0 {
		t.Fatalf("expected no plan lifecycle event for idempotent enroll, got %d", len(publisher.events))
	}
}

func TestEnrollmentServiceTerminatePublishesOnlyTaskCanceledEvents(t *testing.T) {
	ctx := context.Background()
	planAggregate, err := domainPlan.NewAssessmentPlan(9, "scale-code", domainPlan.PlanScheduleByWeek, 1, 2)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}
	planAggregate.ClearEvents()

	testeeID := testeeDomain.NewID(3003)
	taskLifecycle := domainPlan.NewTaskLifecycle()
	pendingTask := domainPlan.NewAssessmentTask(planAggregate.GetID(), 1, 9, testeeID, "scale-code", time.Now())
	openedTask := domainPlan.NewAssessmentTask(planAggregate.GetID(), 2, 9, testeeID, "scale-code", time.Now().Add(time.Hour))
	if err := taskLifecycle.Open(ctx, openedTask, "token", "https://example.com/entry", time.Now().Add(2*time.Hour)); err != nil {
		t.Fatalf("failed to open task: %v", err)
	}
	pendingTask.ClearEvents()
	openedTask.ClearEvents()

	taskRepo := &enrollmentTaskRepoStub{
		planTasks: []*domainPlan.AssessmentTask{pendingTask, openedTask},
	}
	publisher := &enrollmentEventPublisherStub{}
	service := NewEnrollmentService(
		&enrollmentPlanRepoStub{plan: planAggregate},
		taskRepo,
		publisher,
	)

	if err := service.TerminateEnrollment(ctx, 9, planAggregate.GetID().String(), "3003"); err != nil {
		t.Fatalf("TerminateEnrollment returned error: %v", err)
	}
	if len(taskRepo.saved) != 2 {
		t.Fatalf("expected 2 saved tasks, got %d", len(taskRepo.saved))
	}
	if len(publisher.events) != 2 {
		t.Fatalf("expected 2 published task.canceled events, got %d", len(publisher.events))
	}
	for i, evt := range publisher.events {
		if evt.EventType() != domainPlan.EventTypeTaskCanceled {
			t.Fatalf("expected event %d to be task.canceled, got %s", i, evt.EventType())
		}
	}
}
