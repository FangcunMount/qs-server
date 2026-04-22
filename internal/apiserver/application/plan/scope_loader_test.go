package plan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
)

type scopeLoaderPlanRepoStub struct {
	item *domainplan.AssessmentPlan
	err  error
}

func (s *scopeLoaderPlanRepoStub) FindByID(context.Context, domainplan.AssessmentPlanID) (*domainplan.AssessmentPlan, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.item, nil
}

func (s *scopeLoaderPlanRepoStub) FindByScaleCode(context.Context, string) ([]*domainplan.AssessmentPlan, error) {
	return nil, nil
}

func (s *scopeLoaderPlanRepoStub) FindActivePlans(context.Context) ([]*domainplan.AssessmentPlan, error) {
	return nil, nil
}

func (s *scopeLoaderPlanRepoStub) FindByTesteeID(context.Context, testee.ID) ([]*domainplan.AssessmentPlan, error) {
	return nil, nil
}

func (s *scopeLoaderPlanRepoStub) FindList(context.Context, int64, string, string, int, int) ([]*domainplan.AssessmentPlan, int64, error) {
	return nil, 0, nil
}

func (s *scopeLoaderPlanRepoStub) Save(context.Context, *domainplan.AssessmentPlan) error {
	return nil
}

type scopeLoaderTaskRepoStub struct {
	item *domainplan.AssessmentTask
	err  error
}

func (s *scopeLoaderTaskRepoStub) FindByID(context.Context, domainplan.AssessmentTaskID) (*domainplan.AssessmentTask, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.item, nil
}

func (s *scopeLoaderTaskRepoStub) FindByPlanID(context.Context, domainplan.AssessmentPlanID) ([]*domainplan.AssessmentTask, error) {
	return nil, nil
}

func (s *scopeLoaderTaskRepoStub) FindByPlanIDAndTesteeIDs(context.Context, domainplan.AssessmentPlanID, []testee.ID) ([]*domainplan.AssessmentTask, error) {
	return nil, nil
}

func (s *scopeLoaderTaskRepoStub) FindByTesteeID(context.Context, testee.ID) ([]*domainplan.AssessmentTask, error) {
	return nil, nil
}

func (s *scopeLoaderTaskRepoStub) FindByTesteeIDAndPlanID(context.Context, testee.ID, domainplan.AssessmentPlanID) ([]*domainplan.AssessmentTask, error) {
	return nil, nil
}

func (s *scopeLoaderTaskRepoStub) FindPendingTasks(context.Context, int64, time.Time) ([]*domainplan.AssessmentTask, error) {
	return nil, nil
}

func (s *scopeLoaderTaskRepoStub) FindExpiredTasks(context.Context) ([]*domainplan.AssessmentTask, error) {
	return nil, nil
}

func (s *scopeLoaderTaskRepoStub) FindList(context.Context, int64, *domainplan.AssessmentPlanID, *testee.ID, *domainplan.TaskStatus, int, int) ([]*domainplan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (s *scopeLoaderTaskRepoStub) FindListByTesteeIDs(context.Context, int64, *domainplan.AssessmentPlanID, []testee.ID, *domainplan.TaskStatus, int, int) ([]*domainplan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (s *scopeLoaderTaskRepoStub) FindWindow(context.Context, int64, domainplan.AssessmentPlanID, []testee.ID, *domainplan.TaskStatus, *time.Time, int, int) ([]*domainplan.AssessmentTask, bool, error) {
	return nil, false, nil
}

func (s *scopeLoaderTaskRepoStub) Save(context.Context, *domainplan.AssessmentTask) error {
	return nil
}

func (s *scopeLoaderTaskRepoStub) SaveBatch(context.Context, []*domainplan.AssessmentTask) error {
	return nil
}

func TestLoadPlanInOrgRejectsScopeMismatch(t *testing.T) {
	planAggregate, err := domainplan.NewAssessmentPlan(7, "scale-code", domainplan.PlanScheduleByWeek, 1, 2)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}

	_, err = loadPlanInOrg(context.Background(), &scopeLoaderPlanRepoStub{item: planAggregate}, 8, planAggregate.GetID().String(), "test")
	if err == nil {
		t.Fatalf("expected scope mismatch error")
	}
}

func TestLoadTaskInOrgReturnsTaskInScope(t *testing.T) {
	task := domainplan.NewAssessmentTask(domainplan.AssessmentPlanID(100), 1, 7, testee.NewID(3001), "scale-code", time.Now())

	result, err := loadTaskInOrg(context.Background(), &scopeLoaderTaskRepoStub{item: task}, 7, task.GetID().String(), "test")
	if err != nil {
		t.Fatalf("loadTaskInOrg returned error: %v", err)
	}
	if result != task {
		t.Fatalf("expected returned task to match stub item")
	}
}

func TestLoadTaskInOrgMapsRepositoryMiss(t *testing.T) {
	task := domainplan.NewAssessmentTask(domainplan.AssessmentPlanID(100), 1, 7, testee.NewID(3001), "scale-code", time.Now())
	repo := &scopeLoaderTaskRepoStub{
		item: task,
		err:  errors.New("not found"),
	}

	_, err := loadTaskInOrg(context.Background(), repo, 7, task.GetID().String(), "test")
	if err == nil {
		t.Fatalf("expected not found error")
	}
}
