package plan

import (
	"context"
	"errors"
	"testing"
	"time"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type reminderStateTaskRepo struct {
	domainplan.AssessmentTaskRepository
	task  *domainplan.AssessmentTask
	err   error
	calls int
}

func (r *reminderStateTaskRepo) FindByID(context.Context, domainplan.AssessmentTaskID) (*domainplan.AssessmentTask, error) {
	r.calls++
	return r.task, r.err
}

func reminderStateTask(t *testing.T, status domainplan.TaskStatus) *domainplan.AssessmentTask {
	t.Helper()
	definedAt := time.Date(2026, 9, 20, 9, 0, 0, 0, time.Local)
	openAt := definedAt.Add(time.Hour)
	expireAt := openAt.Add(24 * time.Hour)
	task := domainplan.NewAssessmentTaskAt(domainplan.NewAssessmentPlanID(), 2, 19, testee.NewID(31), "scale", definedAt, definedAt)
	task.RestoreScheduleSemantics(3, &definedAt, definedAt)
	task.RestoreFromRepository(task.GetID(), 0, status, &openAt, &expireAt, nil, nil, nil, nil,
		"private-token", "https://collect.example.com/entry?token=private-token")
	return task
}

func TestTaskReminderStateReaderReturnsCurrentScopedFacts(t *testing.T) {
	for _, status := range []domainplan.TaskStatus{domainplan.TaskStatusOpened, domainplan.TaskStatusCompleted} {
		t.Run(status.String(), func(t *testing.T) {
			task := reminderStateTask(t, status)
			repo := &reminderStateTaskRepo{task: task}
			reader := NewTaskReminderStateReader(repo)
			got, err := reader.GetTaskReminderState(context.Background(), 19, task.GetID().String())
			if err != nil {
				t.Fatalf("GetTaskReminderState: %v", err)
			}
			if got == nil || got.TaskID != task.GetID().String() || got.PlanID != task.GetPlanID().String() ||
				got.OrgID != 19 || got.TesteeID != "31" || got.Status != status || got.ScheduleRevision != 3 ||
				got.OpenAt == nil || !got.OpenAt.Equal(*task.GetOpenAt()) || got.ExpireAt == nil || !got.ExpireAt.Equal(*task.GetExpireAt()) ||
				got.EntryURL != task.GetEntryURL() || repo.calls != 1 {
				t.Fatalf("unexpected reminder state: %#v; calls=%d", got, repo.calls)
			}
			*got.OpenAt = got.OpenAt.Add(time.Hour)
			if !task.GetOpenAt().Equal(time.Date(2026, 9, 20, 10, 0, 0, 0, time.Local)) {
				t.Fatalf("reader returned mutable Task time pointer")
			}
		})
	}
}

func TestTaskReminderStateReaderRejectsCrossOrganizationAndInvalidInput(t *testing.T) {
	task := reminderStateTask(t, domainplan.TaskStatusOpened)
	repo := &reminderStateTaskRepo{task: task}
	reader := NewTaskReminderStateReader(repo)
	got, err := reader.GetTaskReminderState(context.Background(), 20, task.GetID().String())
	if got != nil || !componenterrors.IsCode(err, code.ErrPageNotFound) {
		t.Fatalf("cross-organization lookup = (%#v, %v)", got, err)
	}
	for _, input := range []struct {
		orgID  int64
		taskID string
	}{{0, task.GetID().String()}, {19, ""}, {19, "invalid"}} {
		if _, err := reader.GetTaskReminderState(context.Background(), input.orgID, input.taskID); err == nil {
			t.Fatalf("invalid input %+v was accepted", input)
		}
	}
	if repo.calls != 1 {
		t.Fatalf("invalid input reached repository %d times", repo.calls-1)
	}
}

func TestTaskReminderStateReaderPreservesRepositoryFailure(t *testing.T) {
	want := errors.New("storage unavailable")
	repo := &reminderStateTaskRepo{err: want}
	reader := NewTaskReminderStateReader(repo)
	_, err := reader.GetTaskReminderState(context.Background(), 19, domainplan.NewAssessmentTaskID().String())
	if !errors.Is(err, want) {
		t.Fatalf("repository failure = %v, want %v", err, want)
	}
}
