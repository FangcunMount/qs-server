package plan

import (
	"context"
	"testing"
	"time"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	planentry "github.com/FangcunMount/qs-server/internal/apiserver/port/planentry"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"gorm.io/gorm"
)

type fixtureEnrollmentLifecycle struct{}

func (fixtureEnrollmentLifecycle) TerminateActiveByPlan(context.Context, int64, domainplan.AssessmentPlanID, string, time.Time) (int64, error) {
	return 0, nil
}
func (fixtureEnrollmentLifecycle) CloseActiveByPlanIfAllTasksTerminal(context.Context, int64, domainplan.AssessmentPlanID, time.Time) (int64, error) {
	return 0, nil
}

type schedulerEnrollmentFixture struct {
	domainplan.EnrollmentRepository
	items map[domainplan.PlanEnrollmentID]*domainplan.Enrollment
}

func (r schedulerEnrollmentFixture) FindByID(_ context.Context, id domainplan.PlanEnrollmentID) (*domainplan.Enrollment, error) {
	return r.items[id], nil
}
func (schedulerEnrollmentFixture) CloseIfAllTasksTerminal(context.Context, domainplan.PlanEnrollmentID, time.Time) (bool, error) {
	return false, nil
}

func newTestTaskSchedulerService(t *testing.T, tasks *schedulerTaskRepoStub, plans domainplan.AssessmentPlanRepository, entry planentry.Generator, publisher event.EventPublisher) TaskSchedulerService {
	t.Helper()
	enrollments := schedulerEnrollmentFixture{items: map[domainplan.PlanEnrollmentID]*domainplan.Enrollment{}}
	for _, task := range append(append(append([]*domainplan.AssessmentTask{}, tasks.pendingTasks...), tasks.scopedTasks...), tasks.expiredTasks...) {
		if task == nil {
			continue
		}
		enrollment := domainplan.NewEnrollment(task.GetOrgID(), task.GetPlanID(), task.GetTesteeID(), 1, task.GetPlannedAt(), time.Now())
		task.AssignEnrollment(enrollment.ID())
		enrollments.items[enrollment.ID()] = enrollment
	}
	service, err := NewTaskSchedulerService(tasks, plans, enrollments, directPlanTxRunner{}, entry, publisher, TaskOpeningOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type entryEnrollmentFixture struct {
	domainplan.EnrollmentRepository
	tasks taskEntryFinder
}

func (r entryEnrollmentFixture) FindByID(_ context.Context, id domainplan.PlanEnrollmentID) (*domainplan.Enrollment, error) {
	var items []*domainplan.AssessmentTask
	switch tasks := r.tasks.(type) {
	case *taskEntryRepoStub:
		items = []*domainplan.AssessmentTask{tasks.task}
	case *participantTasksStub:
		items = tasks.tasks
	}
	for _, task := range items {
		if task != nil && task.GetEnrollmentID() == id {
			return domainplan.RestoreEnrollment(id, task.GetOrgID(), task.GetPlanID(), task.GetTesteeID(), 1, task.GetPlannedAt(), domainplan.EnrollmentStatusActive, time.Now(), nil, nil, "", domainplan.EnrollmentRecordOriginNative), nil
		}
	}
	return nil, nil
}
func newTestTaskEntryResolver(t *testing.T, tasks taskEntryFinder, scales ScaleCatalog, enrollment ...domainplan.EnrollmentRepository) TaskEntryResolver {
	t.Helper()
	var repo domainplan.EnrollmentRepository = entryEnrollmentFixture{tasks: tasks}
	if len(enrollment) > 0 {
		repo = enrollment[0]
	}
	resolver, err := NewTaskEntryResolver(tasks, scales, repo)
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func TestTaskConstructorsRejectMissingRequiredDependencies(t *testing.T) {
	tasks := &schedulerTaskRepoStub{}
	plans := &schedulerPlanRepoByIDStub{}
	enrollments := &enrollmentRepoStub{}
	entry := &entryGeneratorStub{}
	for name, create := range map[string]func() error{
		"scheduler task": func() error {
			_, e := NewTaskSchedulerService(nil, plans, enrollments, directPlanTxRunner{}, entry, event.NewNopEventPublisher(), TaskOpeningOptions{})
			return e
		},
		"scheduler plan": func() error {
			_, e := NewTaskSchedulerService(tasks, nil, enrollments, directPlanTxRunner{}, entry, event.NewNopEventPublisher(), TaskOpeningOptions{})
			return e
		},
		"scheduler enrollment": func() error {
			_, e := NewTaskSchedulerService(tasks, plans, nil, directPlanTxRunner{}, entry, event.NewNopEventPublisher(), TaskOpeningOptions{})
			return e
		},
		"scheduler transaction": func() error {
			_, e := NewTaskSchedulerService(tasks, plans, enrollments, nil, entry, event.NewNopEventPublisher(), TaskOpeningOptions{})
			return e
		},
		"management plan": func() error {
			_, e := NewTaskManagementService(tasks, nil, enrollments, directPlanTxRunner{}, entry, event.NewNopEventPublisher(), TaskOpeningOptions{})
			return e
		},
		"management enrollment": func() error {
			_, e := NewTaskManagementService(tasks, plans, nil, directPlanTxRunner{}, entry, event.NewNopEventPublisher(), TaskOpeningOptions{})
			return e
		},
		"management transaction": func() error {
			_, e := NewTaskManagementService(tasks, plans, enrollments, nil, entry, event.NewNopEventPublisher(), TaskOpeningOptions{})
			return e
		},
		"entry enrollment": func() error { _, e := NewTaskEntryResolver(tasks, enrollmentScaleCatalogStub{}, nil); return e },
		"lifecycle enrollment": func() error {
			_, e := NewLifecycleService(plans, tasks, enrollmentScaleCatalogStub{}, nil, directPlanTxRunner{}, event.NewNopEventPublisher())
			return e
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := create(); !componenterrors.IsCode(err, code.ErrModuleInitializationFailed) {
				t.Fatalf("missing dependency: %v", err)
			}
		})
	}
}

func TestTaskPersistenceUsesRunnerAndBorrowedTransaction(t *testing.T) {
	runner := &recordingTransactionRunner{}
	persistence := taskPersistence{tx: runner}
	called := false
	if err := persistence.withinTransaction(t.Context(), func(context.Context) error { called = true; return nil }); err != nil || !runner.called || !called {
		t.Fatalf("runner=%+v called=%v err=%v", runner, called, err)
	}
	runner.called = false
	borrowed := &gorm.DB{}
	ctx := mysql.WithTx(t.Context(), borrowed)
	if err := persistence.withinTransaction(ctx, func(inner context.Context) error {
		if tx, active := mysql.TxFromContext(inner); !active || tx != borrowed {
			t.Fatal("caller transaction was not preserved")
		}
		return nil
	}); err != nil || runner.called {
		t.Fatalf("borrowed transaction invoked own runner: called=%v err=%v", runner.called, err)
	}
}

var _ apptransaction.Runner = directPlanTxRunner{}
