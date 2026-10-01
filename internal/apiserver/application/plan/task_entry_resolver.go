package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// TaskEntryResolver checks current task state; legacy tokens are ignored. Collection
// must also prove the IAM User -> Testee relationship before returning it.
type TaskEntryResolver interface {
	ResolveTaskEntry(context.Context, string, string) (*ResolvedTaskEntry, error)
}

type ResolvedTaskEntry struct {
	TaskID               string
	TesteeID             string
	ScaleCode            string
	PlanID               string
	Title                string
	Status               string
	QuestionnaireCode    string
	QuestionnaireVersion string
	ModelVersion         string
	ExpireAt             time.Time
	OpenAt               time.Time
	DueAt                time.Time
	CanStart             bool
}

type taskEntryFinder interface {
	FindByID(context.Context, domainplan.AssessmentTaskID) (*domainplan.AssessmentTask, error)
}

type taskEntryResolver struct {
	tasks       taskEntryFinder
	scales      ScaleCatalog
	now         func() time.Time
	enrollments domainplan.EnrollmentRepository
}

func NewTaskEntryResolver(tasks taskEntryFinder, scales ScaleCatalog, enrollments ...domainplan.EnrollmentRepository) TaskEntryResolver {
	if tasks == nil || scales == nil {
		return nil
	}
	r := &taskEntryResolver{tasks: tasks, scales: scales, now: time.Now}
	if len(enrollments) > 0 {
		r.enrollments = enrollments[0]
	}
	return r
}

func (r *taskEntryResolver) ResolveTaskEntry(ctx context.Context, taskIDRaw, token string) (*ResolvedTaskEntry, error) {
	notFound := func() error { return errors.WithCode(code.ErrPageNotFound, "task entry not found") }
	taskID, err := domainplan.ParseAssessmentTaskID(taskIDRaw)
	if err != nil {
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
		task.GetOpenAt() == nil || r.now().Before(*task.GetOpenAt()) ||
		task.GetAssessmentID() != nil ||
		task.GetScaleCode() == "" {
		return nil, notFound()
	}
	return r.entry(ctx, task)
}

func (r *taskEntryResolver) entry(ctx context.Context, task *domainplan.AssessmentTask) (*ResolvedTaskEntry, error) {
	if r.enrollments != nil {
		enrollment, err := r.enrollments.FindByID(ctx, task.GetEnrollmentID())
		if err != nil {
			return nil, err
		}
		if enrollment == nil || !enrollment.IsActive() || enrollment.OrgID() != task.GetOrgID() || enrollment.TesteeID() != task.GetTesteeID() || enrollment.PlanID() != task.GetPlanID() {
			return nil, errors.WithCode(code.ErrPageNotFound, "task enrollment unavailable")
		}
	}
	published, err := r.scales.ExistsByCode(ctx, task.GetScaleCode())
	if err != nil {
		return nil, err
	}
	if !published {
		return nil, errors.WithCode(code.ErrPageNotFound, "task content unavailable")
	}
	result := &ResolvedTaskEntry{TaskID: task.GetID().String(), TesteeID: task.GetTesteeID().String(), ScaleCode: task.GetScaleCode(), PlanID: task.GetPlanID().String(), Title: r.scales.ResolveTitle(ctx, task.GetScaleCode()), Status: task.GetStatus().String(), DueAt: task.GetDueAt(), CanStart: task.IsOpened()}
	if task.GetOpenAt() != nil {
		result.OpenAt = *task.GetOpenAt()
	} else {
		result.OpenAt = task.GetPlannedAt()
	}
	if task.GetExpireAt() != nil {
		result.ExpireAt = *task.GetExpireAt()
	}
	if content, ok := r.scales.(interface {
		ResolveTaskContent(context.Context, string) (string, string, string, error)
	}); ok {
		result.QuestionnaireCode, result.QuestionnaireVersion, result.ModelVersion, err = content.ResolveTaskContent(ctx, task.GetScaleCode())
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ListParticipantTasks is internal only. Collection authorizes the selected
// profile before calling it; no operator scope or client-supplied org is used.
func (r *taskEntryResolver) ListParticipantTasks(ctx context.Context, raw string) ([]*ResolvedTaskEntry, error) {
	id, err := domainplan.ParseAssessmentTaskID(raw)
	if err != nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "invalid testee ID")
	}
	reader, ok := r.tasks.(interface {
		FindByTesteeID(context.Context, testee.ID) ([]*domainplan.AssessmentTask, error)
	})
	if !ok {
		return nil, fmt.Errorf("participant task reader unavailable")
	}
	tasks, err := reader.FindByTesteeID(ctx, testee.NewID(id.Uint64()))
	if err != nil {
		return nil, err
	}
	result := make([]*ResolvedTaskEntry, 0)
	for _, task := range tasks {
		if task == nil || task.GetTesteeID().Uint64() != id.Uint64() || task.GetStatus().IsTerminal() || task.GetAssessmentID() != nil {
			continue
		}
		if task.IsOpened() && (task.GetExpireAt() == nil || !r.now().Before(*task.GetExpireAt())) {
			continue
		}
		if task.IsPending() && !r.now().Before(domainplan.TaskOpenWindowEndsAt(task.GetPlannedAt())) {
			continue
		}
		entry, err := r.entry(ctx, task)
		if errors.IsCode(err, code.ErrPageNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		entry.CanStart = task.IsOpened() && task.GetOpenAt() != nil && !r.now().Before(*task.GetOpenAt())
		result = append(result, entry)
	}
	return result, nil
}
