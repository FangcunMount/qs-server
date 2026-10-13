package plan

import (
	"context"
	"testing"
	"time"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	appeventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
)

type openingStagerSpy struct{ calls int }

func (s *openingStagerSpy) Stage(context.Context, ...event.DomainEvent) error { s.calls++; return nil }

func TestReminderProducerDisabledDoesNotStageAvailableOutbox(t *testing.T) {
	now := time.Now()
	task := domainplan.NewAssessmentTaskAt(domainplan.NewAssessmentPlanID(), 1, 1, testee.NewID(2), "scale", now.Add(-time.Minute), now)
	if err := domainplan.NewTaskLifecycle().OpenAt(t.Context(), task, "", "https://example.test?task_id="+task.GetID().String(), now); err != nil {
		t.Fatal(err)
	}
	tasks := &schedulerTaskRepoStub{}
	stager := &openingStagerSpy{}
	persistence := taskPersistence{tasks: tasks, enrollments: &enrollmentRepoStub{}, tx: directPlanTxRunner{},
		openingOptions: TaskOpeningOptions{ReminderEnabled: false, ReminderOutbox: appeventing.ProfileBinding{Stager: stager}},
	}
	staged, err := persistence.saveOpened(t.Context(), task)
	if err != nil || staged || stager.calls != 0 || len(tasks.savedTasks) != 1 || len(task.Events()) != 1 {
		t.Fatalf("disabled producer must retain direct opening event without reminder: staged=%v calls=%d saved=%d events=%d err=%v", staged, stager.calls, len(tasks.savedTasks), len(task.Events()), err)
	}
}

func TestReminderProducerEnabledRequiresOutboxAtConstruction(t *testing.T) {
	tasks := &schedulerTaskRepoStub{}
	plans := &schedulerPlanRepoByIDStub{}
	enrollments := &enrollmentRepoStub{}
	for name, create := range map[string]func() error{
		"management": func() error {
			_, err := NewTaskManagementService(tasks, plans, enrollments, directPlanTxRunner{}, &entryGeneratorStub{}, event.NewNopEventPublisher(), TaskOpeningOptions{ReminderEnabled: true})
			return err
		},
		"scheduler": func() error {
			_, err := NewTaskSchedulerService(tasks, plans, enrollments, directPlanTxRunner{}, &entryGeneratorStub{}, event.NewNopEventPublisher(), TaskOpeningOptions{ReminderEnabled: true})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := create(); !componenterrors.IsCode(err, code.ErrModuleInitializationFailed) {
				t.Fatalf("enabled reminder lacks outbox: %v", err)
			}
		})
	}
	if _, err := NewTaskManagementService(tasks, plans, enrollments, directPlanTxRunner{}, &entryGeneratorStub{}, event.NewNopEventPublisher(), TaskOpeningOptions{}); err != nil {
		t.Fatalf("disabled reminder must remain valid: %v", err)
	}
}
