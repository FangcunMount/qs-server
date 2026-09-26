package plan

import (
	"context"
	"crypto/subtle"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/google/uuid"
)

// TaskEntryResolver checks current task state and its entry token. Collection
// must also prove the IAM User -> Testee relationship before returning it.
type TaskEntryResolver interface {
	ResolveTaskEntry(context.Context, string, string) (*ResolvedTaskEntry, error)
}

type ResolvedTaskEntry struct {
	TaskID    string
	TesteeID  string
	ScaleCode string
	ExpireAt  time.Time
}

type taskEntryFinder interface {
	FindByID(context.Context, domainplan.AssessmentTaskID) (*domainplan.AssessmentTask, error)
}

type taskEntryResolver struct {
	tasks  taskEntryFinder
	scales ScaleCatalog
	now    func() time.Time
}

func NewTaskEntryResolver(tasks taskEntryFinder, scales ScaleCatalog) TaskEntryResolver {
	if tasks == nil || scales == nil {
		return nil
	}
	return &taskEntryResolver{tasks: tasks, scales: scales, now: time.Now}
}

func (r *taskEntryResolver) ResolveTaskEntry(ctx context.Context, taskIDRaw, token string) (*ResolvedTaskEntry, error) {
	notFound := func() error { return errors.WithCode(code.ErrPageNotFound, "task entry not found") }
	taskID, err := domainplan.ParseAssessmentTaskID(taskIDRaw)
	if err != nil || len(token) != 36 {
		return nil, notFound()
	}
	parsedToken, err := uuid.Parse(token)
	if err != nil || parsedToken.String() != token {
		return nil, notFound()
	}
	task, err := r.tasks.FindByID(ctx, taskID)
	if err != nil {
		if errors.IsCode(err, code.ErrPageNotFound) {
			return nil, notFound()
		}
		return nil, err
	}
	if task == nil || !task.IsOpened() || task.GetExpireAt() == nil ||
		!r.now().Before(*task.GetExpireAt()) ||
		subtle.ConstantTimeCompare([]byte(task.GetEntryToken()), []byte(token)) != 1 ||
		task.GetScaleCode() == "" {
		return nil, notFound()
	}
	published, err := r.scales.ExistsByCode(ctx, task.GetScaleCode())
	if err != nil {
		return nil, err
	}
	if !published {
		return nil, notFound()
	}
	return &ResolvedTaskEntry{
		TaskID:    task.GetID().String(),
		TesteeID:  task.GetTesteeID().String(),
		ScaleCode: task.GetScaleCode(),
		ExpireAt:  *task.GetExpireAt(),
	}, nil
}
