package plan

import (
	"context"
	"testing"
	"time"

	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/planreadmodel"
)

type planReadModelStub struct {
	planRows       []planreadmodel.PlanRow
	planTotal      int64
	planByID       map[uint64]planreadmodel.PlanRow
	testeePlanRows []planreadmodel.PlanRow
	lastListFilter planreadmodel.PlanFilter
	lastListPage   planreadmodel.PageRequest
	lastGetOrgID   int64
	lastGetPlanID  uint64
	lastTesteeID   uint64
}

type taskReadModelStub struct {
	windowRows       []planreadmodel.TaskRow
	windowHasMore    bool
	lastWindowFilter planreadmodel.TaskWindowFilter
	lastWindowPage   planreadmodel.PageRequest
}

type scaleCatalogStub struct {
	titles map[string]string
}

func (r *planReadModelStub) GetPlan(_ context.Context, orgID int64, planID uint64) (*planreadmodel.PlanRow, error) {
	r.lastGetOrgID = orgID
	r.lastGetPlanID = planID
	if r.planByID != nil {
		row := r.planByID[planID]
		return &row, nil
	}
	return &planreadmodel.PlanRow{ID: planID, OrgID: orgID, ScaleCode: "scale-code", Status: "active"}, nil
}

func (r *planReadModelStub) ListPlans(_ context.Context, filter planreadmodel.PlanFilter, page planreadmodel.PageRequest) (planreadmodel.PlanPage, error) {
	r.lastListFilter = filter
	r.lastListPage = page
	return planreadmodel.PlanPage{Items: r.planRows, Total: r.planTotal, Page: page.Page, PageSize: page.PageSize}, nil
}

func (r *planReadModelStub) ListPlansByTesteeID(_ context.Context, testeeID uint64) ([]planreadmodel.PlanRow, error) {
	r.lastTesteeID = testeeID
	return r.testeePlanRows, nil
}

func (r *taskReadModelStub) GetTask(context.Context, int64, uint64) (*planreadmodel.TaskRow, error) {
	return nil, nil
}

func (r *taskReadModelStub) ListTasks(context.Context, planreadmodel.TaskFilter, planreadmodel.PageRequest) (planreadmodel.TaskPage, error) {
	return planreadmodel.TaskPage{}, nil
}

func (r *taskReadModelStub) ListTaskWindow(_ context.Context, filter planreadmodel.TaskWindowFilter, page planreadmodel.PageRequest) (planreadmodel.TaskWindow, error) {
	r.lastWindowFilter = filter
	r.lastWindowPage = page
	return planreadmodel.TaskWindow{Items: r.windowRows, Page: page.Page, PageSize: page.PageSize, HasMore: r.windowHasMore}, nil
}

func (r *taskReadModelStub) ListTasksByPlanID(context.Context, uint64) ([]planreadmodel.TaskRow, error) {
	return nil, nil
}

func (r *taskReadModelStub) ListTasksByPlanIDAndTesteeIDs(context.Context, uint64, []uint64) ([]planreadmodel.TaskRow, error) {
	return nil, nil
}

func (r *taskReadModelStub) ListTasksByTesteeID(context.Context, uint64) ([]planreadmodel.TaskRow, error) {
	return nil, nil
}

func (r *taskReadModelStub) ListTasksByTesteeIDAndPlanID(context.Context, uint64, uint64) ([]planreadmodel.TaskRow, error) {
	return nil, nil
}

func (r *scaleCatalogStub) ExistsByCode(context.Context, string) (bool, error) {
	return true, nil
}

func (r *scaleCatalogStub) ResolveTitle(_ context.Context, code string) string {
	if r == nil {
		return ""
	}
	return r.titles[code]
}

func (r *scaleCatalogStub) ResolveTitles(_ context.Context, codes []string) map[string]string {
	results := make(map[string]string, len(codes))
	if r == nil {
		return results
	}
	for _, code := range codes {
		results[code] = r.titles[code]
	}
	return results
}

func TestQueryServiceListTaskWindowForwardsWindowFilters(t *testing.T) {
	planAggregate, err := domainPlan.NewAssessmentPlan(1, "scale-code", domainPlan.PlanScheduleByWeek, 1, 1)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}
	reader := &taskReadModelStub{
		windowRows: []planreadmodel.TaskRow{{
			ID:        1001,
			PlanID:    planAggregate.GetID().Uint64(),
			Seq:       1,
			OrgID:     1,
			TesteeID:  3001,
			ScaleCode: "scale-code",
			PlannedAt: time.Date(2026, 4, 11, 10, 0, 0, 0, time.Local),
			Status:    "pending",
		}},
		windowHasMore: true,
	}
	service := NewQueryService(&planReadModelStub{}, reader, nil)

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

	if reader.lastWindowFilter.OrgID != 1 {
		t.Fatalf("expected org_id=1, got %d", reader.lastWindowFilter.OrgID)
	}
	if reader.lastWindowFilter.PlanID != planAggregate.GetID().Uint64() {
		t.Fatalf("expected plan id %s, got %d", planAggregate.GetID().String(), reader.lastWindowFilter.PlanID)
	}
	if len(reader.lastWindowFilter.TesteeIDs) != 2 || reader.lastWindowFilter.TesteeIDs[0] != 3001 || reader.lastWindowFilter.TesteeIDs[1] != 3002 {
		t.Fatalf("unexpected window testee ids: %+v", reader.lastWindowFilter.TesteeIDs)
	}
	if reader.lastWindowFilter.Status == nil || *reader.lastWindowFilter.Status != "pending" {
		t.Fatalf("unexpected window status: %+v", reader.lastWindowFilter.Status)
	}
	if reader.lastWindowFilter.PlannedBefore == nil || reader.lastWindowFilter.PlannedBefore.Format("2006-01-02 15:04:05") != "2026-04-11 12:00:00" {
		t.Fatalf("unexpected planned_before: %+v", reader.lastWindowFilter.PlannedBefore)
	}
	if reader.lastWindowPage.Page != 2 || reader.lastWindowPage.PageSize != 50 {
		t.Fatalf("unexpected page args: page=%d page_size=%d", reader.lastWindowPage.Page, reader.lastWindowPage.PageSize)
	}
	if result == nil || !result.HasMore || len(result.Items) != 1 {
		t.Fatalf("unexpected window result: %#v", result)
	}
}

func TestQueryServiceListTaskWindowRejectsInvalidStatus(t *testing.T) {
	service := NewQueryService(&planReadModelStub{}, &taskReadModelStub{}, nil)

	_, err := service.ListTaskWindow(context.Background(), ListTaskWindowDTO{
		OrgID:  1,
		PlanID: "614333603412718126",
		Status: "unknown",
	})
	if err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestQueryServiceListPlansResolvesScaleTitle(t *testing.T) {
	planReader := &planReadModelStub{
		planRows: []planreadmodel.PlanRow{{
			ID:           1001,
			OrgID:        1,
			ScaleCode:    "scale-code",
			ScheduleType: "by_week",
			Interval:     1,
			TotalTimes:   1,
			Status:       "active",
		}},
		planTotal: 1,
	}
	service := NewQueryService(planReader, &taskReadModelStub{}, &scaleCatalogStub{
		titles: map[string]string{"scale-code": "抑郁自评量表"},
	})

	result, err := service.ListPlans(context.Background(), ListPlansDTO{
		OrgID:    1,
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListPlans returned error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].ScaleTitle != "抑郁自评量表" {
		t.Fatalf("expected scale title to be resolved, got %q", result.Items[0].ScaleTitle)
	}
}
