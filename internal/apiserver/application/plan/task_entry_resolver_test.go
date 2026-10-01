package plan

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/google/uuid"
)

type taskEntryRepoStub struct {
	task  *domainplan.AssessmentTask
	err   error
	reads int
}

func (s *taskEntryRepoStub) FindByID(context.Context, domainplan.AssessmentTaskID) (*domainplan.AssessmentTask, error) {
	s.reads++
	return s.task, s.err
}

type taskEntryScaleStub struct {
	published bool
	err       error
	reads     int
}

func (s *taskEntryScaleStub) ExistsByCode(context.Context, string) (bool, error) {
	s.reads++
	return s.published, s.err
}
func (*taskEntryScaleStub) ResolveTitle(_ context.Context, code string) string { return code }
func (*taskEntryScaleStub) ResolveTitles(context.Context, []string) map[string]string {
	return nil
}

func taskForEntryTest(at time.Time, status domainplan.TaskStatus, token string) *domainplan.AssessmentTask {
	plannedAt := at.Add(-time.Hour)
	openAt := at.Add(-30 * time.Minute)
	expireAt := at.Add(time.Hour)
	task := domainplan.NewAssessmentTaskAt(domainplan.NewAssessmentPlanID(), 1, 19, testee.NewID(31), "scale-1", plannedAt, plannedAt)
	task.RestoreFromRepository(task.GetID(), 0, status, &openAt, &expireAt, nil, nil, nil, nil, token, "https://collect.example.com/entry")
	return task
}

func TestTaskEntryResolverRequiresCurrentTaskWithLegacyTokenIgnored(t *testing.T) {
	at := time.Date(2026, 9, 26, 9, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	token := uuid.NewString()
	task := taskForEntryTest(at, domainplan.TaskStatusOpened, token)
	repo := &taskEntryRepoStub{task: task}
	scales := &taskEntryScaleStub{published: true}
	resolver := NewTaskEntryResolver(repo, scales).(*taskEntryResolver)
	resolver.now = func() time.Time { return at }

	got, err := resolver.ResolveTaskEntry(context.Background(), task.GetID().String(), "")
	if err != nil || got == nil || got.TaskID != task.GetID().String() || got.TesteeID != "31" ||
		got.ScaleCode != "scale-1" || !got.ExpireAt.Equal(*task.GetExpireAt()) {
		t.Fatalf("resolved current task = (%#v, %v)", got, err)
	}
	for _, candidate := range []struct {
		name   string
		taskID string
		token  string
		status domainplan.TaskStatus
		now    time.Time
	}{
		{"pending", task.GetID().String(), token, domainplan.TaskStatusPending, at},
		{"future open", task.GetID().String(), token, domainplan.TaskStatusOpened, at.Add(-time.Hour)},
		{"bad task ID", "not-an-id", token, domainplan.TaskStatusOpened, at},
		{"canceled", task.GetID().String(), token, domainplan.TaskStatusCanceled, at},
		{"completed", task.GetID().String(), token, domainplan.TaskStatusCompleted, at},
		{"expired time", task.GetID().String(), token, domainplan.TaskStatusOpened, *task.GetExpireAt()},
	} {
		t.Run(candidate.name, func(t *testing.T) {
			repo.task = taskForEntryTest(at, candidate.status, token)
			resolver.now = func() time.Time { return candidate.now }
			if _, err := resolver.ResolveTaskEntry(context.Background(), candidate.taskID, candidate.token); !componenterrors.IsCode(err, code.ErrPageNotFound) {
				t.Fatalf("entry should be hidden, got %v", err)
			}
		})
	}
	if scales.reads != 1 {
		t.Fatalf("invalid or terminal entries reached published catalog %d times", scales.reads-1)
	}
}

func TestTaskEntryResolverDistinguishesUnpublishedFromUnavailableCatalog(t *testing.T) {
	at := time.Now()
	token := uuid.NewString()
	task := taskForEntryTest(at, domainplan.TaskStatusOpened, token)
	repo := &taskEntryRepoStub{task: task}
	scales := &taskEntryScaleStub{}
	resolver := NewTaskEntryResolver(repo, scales).(*taskEntryResolver)
	resolver.now = func() time.Time { return at }

	if _, err := resolver.ResolveTaskEntry(context.Background(), task.GetID().String(), token); !componenterrors.IsCode(err, code.ErrPageNotFound) {
		t.Fatalf("unpublished scale should hide entry: %v", err)
	}
	want := stderrors.New("catalog unavailable")
	scales.err = want
	if _, err := resolver.ResolveTaskEntry(context.Background(), task.GetID().String(), token); !stderrors.Is(err, want) {
		t.Fatalf("catalog failure was converted into expiration: %v", err)
	}
	repo.err = want
	if _, err := resolver.ResolveTaskEntry(context.Background(), task.GetID().String(), token); !stderrors.Is(err, want) {
		t.Fatalf("task storage failure was converted into expiration: %v", err)
	}
}

func TestTaskEntryIgnoresLegacyToken(t *testing.T) {
	at := time.Now()
	task := taskForEntryTest(at, domainplan.TaskStatusOpened, "old-secret")
	resolver := NewTaskEntryResolver(&taskEntryRepoStub{task: task}, &taskEntryScaleStub{published: true})
	for _, token := range []string{"", "old-secret", "untrusted", "ae_doctor"} {
		got, err := resolver.ResolveTaskEntry(t.Context(), task.GetID().String(), token)
		if err != nil || got.TaskID != task.GetID().String() {
			t.Fatalf("legacy locator %q: %v", token, err)
		}
	}
}

type participantTasksStub struct {
	taskEntryRepoStub
	tasks []*domainplan.AssessmentTask
}

func (r *participantTasksStub) FindByTesteeID(context.Context, testee.ID) ([]*domainplan.AssessmentTask, error) {
	return r.tasks, r.err
}
func TestParticipantTaskListFiltersTerminalAndTimeBoundaries(t *testing.T) {
	at := time.Now()
	opened := taskForEntryTest(at, domainplan.TaskStatusOpened, "")
	pending := taskForEntryTest(at, domainplan.TaskStatusPending, "")
	expired := taskForEntryTest(at.Add(-2*time.Hour), domainplan.TaskStatusOpened, "")
	completed := taskForEntryTest(at, domainplan.TaskStatusCompleted, "")
	canceled := taskForEntryTest(at, domainplan.TaskStatusCanceled, "")
	resolver := NewTaskEntryResolver(&participantTasksStub{tasks: []*domainplan.AssessmentTask{opened, pending, expired, completed, canceled}}, &taskEntryScaleStub{published: true}).(*taskEntryResolver)
	resolver.now = func() time.Time { return at }
	got, err := resolver.ListParticipantTasks(t.Context(), "31")
	if err != nil || len(got) != 2 || !got[0].CanStart || got[1].CanStart || got[0].PlanID == "" || got[0].DueAt.IsZero() {
		t.Fatalf("task list %#v, %v", got, err)
	}
}

func TestTaskEntryRequiresItsActiveEnrollment(t *testing.T) {
	at := time.Now()
	task := taskForEntryTest(at, domainplan.TaskStatusOpened, "")
	good := domainplan.NewEnrollment(task.GetOrgID(), task.GetPlanID(), task.GetTesteeID(), 1, at, at)
	task.AssignEnrollment(good.ID())
	for name, enrollment := range map[string]*domainplan.Enrollment{
		"valid":           good,
		"missing":         nil,
		"foreign profile": domainplan.NewEnrollment(task.GetOrgID(), task.GetPlanID(), testee.NewID(99), 1, at, at),
		"foreign org":     domainplan.NewEnrollment(99, task.GetPlanID(), task.GetTesteeID(), 1, at, at),
		"foreign plan":    domainplan.NewEnrollment(task.GetOrgID(), domainplan.NewAssessmentPlanID(), task.GetTesteeID(), 1, at, at),
	} {
		t.Run(name, func(t *testing.T) {
			resolver := NewTaskEntryResolver(&taskEntryRepoStub{task: task}, &taskEntryScaleStub{published: true}, &enrollmentRepoStub{active: enrollment})
			_, err := resolver.ResolveTaskEntry(t.Context(), task.GetID().String(), "")
			if (err == nil) != (name == "valid") {
				t.Fatalf("enrollment scope: %v", err)
			}
		})
	}
}
