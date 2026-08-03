package plan

import (
	"context"
	"testing"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type resumePlanRepositoryStub struct {
	domainPlan.AssessmentPlanRepository
	plan      *domainPlan.AssessmentPlan
	lockCalls int
	saveCalls int
}

func (r *resumePlanRepositoryStub) FindByIDForUpdate(context.Context, domainPlan.AssessmentPlanID) (*domainPlan.AssessmentPlan, error) {
	r.lockCalls++
	return r.plan, nil
}

func (r *resumePlanRepositoryStub) Save(context.Context, *domainPlan.AssessmentPlan) error {
	r.saveCalls++
	return nil
}

type resumeTaskRepositoryStub struct {
	domainPlan.AssessmentTaskRepository
	tasks             []*domainPlan.AssessmentTask
	lockCalls         int
	expectedRevisions []uint32
	saveErr           error
}

func (r *resumeTaskRepositoryStub) FindByPlanIDForUpdate(context.Context, domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	r.lockCalls++
	return r.tasks, nil
}

func (r *resumeTaskRepositoryStub) SaveRescheduled(_ context.Context, _ *domainPlan.AssessmentTask, expected uint32) error {
	r.expectedRevisions = append(r.expectedRevisions, expected)
	return r.saveErr
}

type recordingTransactionRunner struct {
	called, committed, rolledBack bool
}

func (r *recordingTransactionRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.called = true
	err := fn(ctx)
	if err != nil {
		r.rolledBack = true
		return err
	}
	r.committed = true
	return nil
}

var _ apptransaction.Runner = (*recordingTransactionRunner)(nil)

func newPausedPlanAndCanceledTask(t *testing.T) (*domainPlan.AssessmentPlan, *domainPlan.AssessmentTask, domainTestee.ID, time.Time) {
	t.Helper()
	plan, err := domainPlan.NewAssessmentPlan(7, "scale", domainPlan.PlanScheduleByDay, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	plan.RestoreFromRepository(plan.GetID(), domainPlan.PlanStatusPaused)
	testeeID := domainTestee.NewID(99)
	startDate := time.Date(2026, 8, 5, 0, 0, 0, 0, time.Local)
	plannedAt := time.Date(2026, 8, 5, 19, 0, 0, 0, time.Local)
	task := domainPlan.NewAssessmentTaskAt(plan.GetID(), 1, 7, testeeID, "scale", plannedAt, plannedAt.Add(-time.Hour))
	task.AssignEnrollment(domainPlan.PlanEnrollmentID(123))
	if err := domainPlan.NewTaskLifecycle().Cancel(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	return plan, task, testeeID, startDate
}

func TestResumePlanLocksAndCommitsPlanAndRevisionCASInOneTransaction(t *testing.T) {
	plan, task, testeeID, startDate := newPausedPlanAndCanceledTask(t)
	plans := &resumePlanRepositoryStub{plan: plan}
	tasks := &resumeTaskRepositoryStub{tasks: []*domainPlan.AssessmentTask{task}}
	tx := &recordingTransactionRunner{}
	service := NewLifecycleServiceWithEnrollment(plans, tasks, nil, nil, tx, nil)

	if _, err := service.ResumePlan(context.Background(), 7, plan.GetID().String(), map[string]string{testeeID.String(): startDate.Format("2006-01-02")}); err != nil {
		t.Fatal(err)
	}
	if !tx.called || !tx.committed || tx.rolledBack || plans.lockCalls != 1 || tasks.lockCalls != 1 || plans.saveCalls != 1 {
		t.Fatalf("tx=%+v plan_locks=%d task_locks=%d plan_saves=%d", tx, plans.lockCalls, tasks.lockCalls, plans.saveCalls)
	}
	if len(tasks.expectedRevisions) != 1 || tasks.expectedRevisions[0] != 1 || task.GetScheduleRevision() != 2 {
		t.Fatalf("expected_revisions=%v current=%d", tasks.expectedRevisions, task.GetScheduleRevision())
	}
}

func TestResumePlanRevisionConflictRollsBackWholeWorkflow(t *testing.T) {
	plan, task, testeeID, startDate := newPausedPlanAndCanceledTask(t)
	plans := &resumePlanRepositoryStub{plan: plan}
	tasks := &resumeTaskRepositoryStub{
		tasks:   []*domainPlan.AssessmentTask{task},
		saveErr: baseerrors.WithCode(code.ErrConflict, "task schedule revision conflict"),
	}
	tx := &recordingTransactionRunner{}
	service := NewLifecycleServiceWithEnrollment(plans, tasks, nil, nil, tx, nil)

	_, err := service.ResumePlan(context.Background(), 7, plan.GetID().String(), map[string]string{testeeID.String(): startDate.Format("2006-01-02")})
	if err == nil || !baseerrors.IsCode(err, code.ErrConflict) {
		t.Fatalf("err=%v want conflict", err)
	}
	if !tx.rolledBack || tx.committed {
		t.Fatalf("tx=%+v", tx)
	}
}

func TestResumePlanFailsClosedWithoutTransactionRunner(t *testing.T) {
	plan, task, testeeID, startDate := newPausedPlanAndCanceledTask(t)
	plans := &resumePlanRepositoryStub{plan: plan}
	tasks := &resumeTaskRepositoryStub{tasks: []*domainPlan.AssessmentTask{task}}
	service := NewLifecycleServiceWithEnrollment(plans, tasks, nil, nil, nil, nil)

	_, err := service.ResumePlan(context.Background(), 7, plan.GetID().String(), map[string]string{testeeID.String(): startDate.Format("2006-01-02")})
	if err == nil || !baseerrors.IsCode(err, code.ErrInternalServerError) {
		t.Fatalf("err=%v want internal transaction configuration error", err)
	}
	if !plan.IsPaused() || plans.lockCalls != 0 || plans.saveCalls != 0 || tasks.lockCalls != 0 {
		t.Fatalf("resume mutated state without transaction: status=%s plan_locks=%d plan_saves=%d task_locks=%d", plan.GetStatus(), plans.lockCalls, plans.saveCalls, tasks.lockCalls)
	}
}
