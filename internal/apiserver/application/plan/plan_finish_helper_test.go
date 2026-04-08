package plan

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type finalizePlanRepoStub struct {
	plan      *domainPlan.AssessmentPlan
	saveCount int
}

func (r *finalizePlanRepoStub) FindByID(_ context.Context, id domainPlan.AssessmentPlanID) (*domainPlan.AssessmentPlan, error) {
	if r.plan != nil && r.plan.GetID() == id {
		return r.plan, nil
	}
	return nil, nil
}

func (r *finalizePlanRepoStub) FindByScaleCode(context.Context, string) ([]*domainPlan.AssessmentPlan, error) {
	return nil, nil
}

func (r *finalizePlanRepoStub) FindActivePlans(context.Context) ([]*domainPlan.AssessmentPlan, error) {
	return nil, nil
}

func (r *finalizePlanRepoStub) FindByTesteeID(context.Context, testee.ID) ([]*domainPlan.AssessmentPlan, error) {
	return nil, nil
}

func (r *finalizePlanRepoStub) FindList(context.Context, int64, string, string, int, int) ([]*domainPlan.AssessmentPlan, int64, error) {
	return nil, 0, nil
}

func (r *finalizePlanRepoStub) Save(context.Context, *domainPlan.AssessmentPlan) error {
	r.saveCount++
	return nil
}

type finalizeTaskRepoStub struct {
	tasks []*domainPlan.AssessmentTask
}

func (r *finalizeTaskRepoStub) FindByID(context.Context, domainPlan.AssessmentTaskID) (*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *finalizeTaskRepoStub) FindByPlanID(_ context.Context, _ domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	return r.tasks, nil
}

func (r *finalizeTaskRepoStub) FindByPlanIDAndTesteeIDs(context.Context, domainPlan.AssessmentPlanID, []testee.ID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *finalizeTaskRepoStub) FindByTesteeID(context.Context, testee.ID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *finalizeTaskRepoStub) FindByTesteeIDAndPlanID(context.Context, testee.ID, domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *finalizeTaskRepoStub) FindPendingTasks(context.Context, int64, time.Time) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *finalizeTaskRepoStub) FindExpiredTasks(context.Context) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *finalizeTaskRepoStub) FindList(context.Context, int64, *domainPlan.AssessmentPlanID, *testee.ID, *domainPlan.TaskStatus, int, int) ([]*domainPlan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (r *finalizeTaskRepoStub) FindListByTesteeIDs(context.Context, int64, *domainPlan.AssessmentPlanID, []testee.ID, *domainPlan.TaskStatus, int, int) ([]*domainPlan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (r *finalizeTaskRepoStub) Save(context.Context, *domainPlan.AssessmentTask) error {
	return nil
}

func (r *finalizeTaskRepoStub) SaveBatch(context.Context, []*domainPlan.AssessmentTask) error {
	return nil
}

type recordingEventPublisher struct {
	eventTypes []string
}

func (p *recordingEventPublisher) Publish(_ context.Context, evt event.DomainEvent) error {
	p.eventTypes = append(p.eventTypes, evt.EventType())
	return nil
}

func (p *recordingEventPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

func TestFinalizePlanIfDonePublishesPlanFinishedEvent(t *testing.T) {
	ctx := context.Background()

	p, err := domainPlan.NewAssessmentPlan(1, "scale-code", domainPlan.PlanScheduleByWeek, 1, 1)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}
	p.ClearEvents()

	task := domainPlan.NewAssessmentTask(
		p.GetID(),
		1,
		1,
		testee.NewID(4001),
		"scale-code",
		time.Now(),
	)
	taskLifecycle := domainPlan.NewTaskLifecycle()
	if err := taskLifecycle.Open(ctx, task, "token", "https://example.com/entry", time.Now().Add(time.Hour), ""); err != nil {
		t.Fatalf("failed to open task: %v", err)
	}
	task.ClearEvents()
	if err := taskLifecycle.Complete(ctx, task, assessment.NewID(9201)); err != nil {
		t.Fatalf("failed to complete task: %v", err)
	}
	task.ClearEvents()

	planRepo := &finalizePlanRepoStub{plan: p}
	planLifecycle := domainPlan.NewPlanLifecycle(
		&finalizeTaskRepoStub{tasks: []*domainPlan.AssessmentTask{task}},
		domainPlan.NewTaskGenerator(),
		domainPlan.NewTaskLifecycle(),
	)
	publisher := &recordingEventPublisher{}

	if err := finalizePlanIfDone(ctx, "test_finalize_plan", planRepo, planLifecycle, publisher, p.GetID()); err != nil {
		t.Fatalf("finalizePlanIfDone returned error: %v", err)
	}

	if !p.IsFinished() {
		t.Fatalf("expected plan to be marked finished")
	}
	if planRepo.saveCount != 1 {
		t.Fatalf("expected plan to be saved once, got %d", planRepo.saveCount)
	}
	if len(publisher.eventTypes) != 1 || publisher.eventTypes[0] != domainPlan.EventTypePlanFinished {
		t.Fatalf("expected plan.finished event, got %v", publisher.eventTypes)
	}
	if len(p.Events()) != 0 {
		t.Fatalf("expected pending plan events to be cleared after publish")
	}
}
