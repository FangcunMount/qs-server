package container

import (
	"context"
	"errors"
	"testing"

	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
)

type reminderConsumerStub struct{}

func (*reminderConsumerStub) ProcessTaskOpenedReminder(context.Context, notificationApp.TaskOpenedReminderRequest) error {
	return nil
}

func TestTaskOpenedReminderConsumerSurvivesProducerRollback(t *testing.T) {
	consumer := &reminderConsumerStub{}
	got, err := initTaskOpenedReminderConsumer(false, func() (notificationApp.TaskOpenedReminderService, error) {
		return consumer, nil
	})
	if err != nil || got != consumer {
		t.Fatalf("disabled producer must retain available consumer: service=%v err=%v", got, err)
	}
}

func TestTaskOpenedReminderConsumerDependencyPolicy(t *testing.T) {
	missing := errors.New("recipient lookup unavailable")
	build := func() (notificationApp.TaskOpenedReminderService, error) { return nil, missing }
	if got, err := initTaskOpenedReminderConsumer(false, build); err != nil || got != nil {
		t.Fatalf("disabled producer with unavailable dependencies: service=%v err=%v", got, err)
	}
	if _, err := initTaskOpenedReminderConsumer(true, build); !errors.Is(err, missing) {
		t.Fatalf("enabled producer must fail startup: err=%v", err)
	}
}
