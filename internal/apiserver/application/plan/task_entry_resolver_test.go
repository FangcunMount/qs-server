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

func TestTaskEntryResolverRequiresCurrentTaskAndExactToken(t *testing.T) {
	at := time.Date(2026, 9, 26, 9, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	token := uuid.NewString()
	task := taskForEntryTest(at, domainplan.TaskStatusOpened, token)
	repo := &taskEntryRepoStub{task: task}
	scales := &taskEntryScaleStub{published: true}
	resolver := NewTaskEntryResolver(repo, scales).(*taskEntryResolver)
	resolver.now = func() time.Time { return at }

	got, err := resolver.ResolveTaskEntry(context.Background(), task.GetID().String(), token)
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
		{"wrong token", task.GetID().String(), uuid.NewString(), domainplan.TaskStatusOpened, at},
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
