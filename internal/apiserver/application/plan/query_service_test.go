package plan

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
)

type taskWindowRepoStub struct {
	windowTasks         []*domainPlan.AssessmentTask
	windowHasMore       bool
	lastWindowOrgID     int64
	lastWindowPlanID    domainPlan.AssessmentPlanID
	lastWindowTesteeIDs []testee.ID
	lastWindowStatus    *domainPlan.TaskStatus
	lastWindowBefore    *time.Time
	lastWindowPage      int
	lastWindowPageSize  int
}

func (r *taskWindowRepoStub) FindByID(context.Context, domainPlan.AssessmentTaskID) (*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *taskWindowRepoStub) FindByPlanID(context.Context, domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *taskWindowRepoStub) FindByPlanIDAndTesteeIDs(context.Context, domainPlan.AssessmentPlanID, []testee.ID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *taskWindowRepoStub) FindByTesteeID(context.Context, testee.ID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *taskWindowRepoStub) FindByTesteeIDAndPlanID(context.Context, testee.ID, domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *taskWindowRepoStub) FindPendingTasks(context.Context, int64, time.Time) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *taskWindowRepoStub) FindExpiredTasks(context.Context) ([]*domainPlan.AssessmentTask, error) {
	return nil, nil
}

func (r *taskWindowRepoStub) FindList(context.Context, int64, *domainPlan.AssessmentPlanID, *testee.ID, *domainPlan.TaskStatus, int, int) ([]*domainPlan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (r *taskWindowRepoStub) FindListByTesteeIDs(context.Context, int64, *domainPlan.AssessmentPlanID, []testee.ID, *domainPlan.TaskStatus, int, int) ([]*domainPlan.AssessmentTask, int64, error) {
	return nil, 0, nil
}

func (r *taskWindowRepoStub) FindWindow(ctx context.Context, orgID int64, planID domainPlan.AssessmentPlanID, testeeIDs []testee.ID, status *domainPlan.TaskStatus, plannedBefore *time.Time, page, pageSize int) ([]*domainPlan.AssessmentTask, bool, error) {
	r.lastWindowOrgID = orgID
	r.lastWindowPlanID = planID
	r.lastWindowTesteeIDs = append([]testee.ID(nil), testeeIDs...)
	r.lastWindowStatus = status
	if plannedBefore != nil {
		clone := *plannedBefore
		r.lastWindowBefore = &clone
	} else {
		r.lastWindowBefore = nil
	}
	r.lastWindowPage = page
	r.lastWindowPageSize = pageSize
	return r.windowTasks, r.windowHasMore, nil
}

func (r *taskWindowRepoStub) Save(context.Context, *domainPlan.AssessmentTask) error {
	return nil
}

func (r *taskWindowRepoStub) SaveBatch(context.Context, []*domainPlan.AssessmentTask) error {
	return nil
}

func TestQueryServiceListTaskWindowForwardsWindowFilters(t *testing.T) {
	planAggregate, err := domainPlan.NewAssessmentPlan(1, "scale-code", domainPlan.PlanScheduleByWeek, 1, 1)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}
	task := domainPlan.NewAssessmentTask(
		planAggregate.GetID(),
		1,
		1,
		testee.NewID(3001),
		"scale-code",
		time.Date(2026, 4, 11, 10, 0, 0, 0, time.Local),
	)

	repo := &taskWindowRepoStub{
		windowTasks:   []*domainPlan.AssessmentTask{task},
		windowHasMore: true,
	}
	service := NewQueryService(&schedulerPlanRepoByIDStub{plan: planAggregate}, repo)

	result, err := service.ListTaskWindow(context.Background(), ListTaskWindowDTO{
		OrgID:         1,
		PlanID:        planAggregate.GetID().String(),
		TesteeIDs:     []string{"3001", "3002"},
		Status:        "pending",
		PlannedBefore: "2026-04-11 12:00:00",
		Page:          2,
		PageSize:      50,
	})
	if err != nil {
		t.Fatalf("ListTaskWindow returned error: %v", err)
	}

	if repo.lastWindowOrgID != 1 {
		t.Fatalf("expected org_id=1, got %d", repo.lastWindowOrgID)
	}
	if repo.lastWindowPlanID != planAggregate.GetID() {
		t.Fatalf("expected plan id %s, got %s", planAggregate.GetID().String(), repo.lastWindowPlanID.String())
	}
	if len(repo.lastWindowTesteeIDs) != 2 || repo.lastWindowTesteeIDs[0] != testee.NewID(3001) || repo.lastWindowTesteeIDs[1] != testee.NewID(3002) {
		t.Fatalf("unexpected window testee ids: %+v", repo.lastWindowTesteeIDs)
	}
	if repo.lastWindowStatus == nil || *repo.lastWindowStatus != domainPlan.TaskStatusPending {
		t.Fatalf("unexpected window status: %+v", repo.lastWindowStatus)
	}
	if repo.lastWindowBefore == nil || repo.lastWindowBefore.Format("2006-01-02 15:04:05") != "2026-04-11 12:00:00" {
		t.Fatalf("unexpected planned_before: %+v", repo.lastWindowBefore)
	}
	if repo.lastWindowPage != 2 || repo.lastWindowPageSize != 50 {
		t.Fatalf("unexpected page args: page=%d page_size=%d", repo.lastWindowPage, repo.lastWindowPageSize)
	}
	if result == nil || !result.HasMore || len(result.Items) != 1 {
		t.Fatalf("unexpected window result: %#v", result)
	}
}

func TestQueryServiceListTaskWindowRejectsInvalidStatus(t *testing.T) {
	service := NewQueryService(&schedulerPlanRepoByIDStub{}, &taskWindowRepoStub{})

	_, err := service.ListTaskWindow(context.Background(), ListTaskWindowDTO{
		OrgID:  1,
		PlanID: "614333603412718126",
		Status: "unknown",
	})
	if err == nil {
		t.Fatal("expected invalid status error")
	}
}
