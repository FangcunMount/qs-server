package plan

import (
	"context"
	"errors"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/planreadmodel"
	"testing"
)

type scopedTaskReader struct {
	planreadmodel.TaskReader
	calls int
}

func (r *scopedTaskReader) GetTask(context.Context, int64, uint64) (*planreadmodel.TaskRow, error) {
	r.calls++
	return &planreadmodel.TaskRow{ID: 11, OrgID: 1, TesteeID: 3, ScaleCode: "S1"}, nil
}

type taskReadScope struct {
	denied bool
	calls  int
}

func (s *taskReadScope) ValidateTesteeStoreAccess(_ context.Context, org, user int64, testee uint64, resource, action string) error {
	s.calls++
	if org != 1 || user != 2 || testee != 3 || resource != appauthz.EvaluationPlanTaskResource || action != "read" {
		return errors.New("wrong read scope")
	}
	if s.denied {
		return errors.New("outside store")
	}
	return nil
}
func TestTaskDetailsCheckCurrentStoreBeforeEnrichment(t *testing.T) {
	for _, denied := range []bool{false, true} {
		reader := &scopedTaskReader{}
		access := &taskReadScope{denied: denied}
		service := NewQueryService(nil, reader, nil, access)
		ctx := appauthz.WithSnapshot(enrollmentContext(), &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: appauthz.EvaluationPlanTaskResource, Action: "read", Mode: appauthz.AuthorizationModeUnconditional}}})
		result, err := service.GetTask(ctx, 1, "11")
		if access.calls != 1 {
			t.Fatalf("scope check calls=%d", access.calls)
		}
		if denied {
			if err == nil || result != nil {
				t.Fatal("out of scope task returned")
			}
		} else if err != nil || result == nil {
			t.Fatalf("permitted result=%v err=%v", result, err)
		}
	}
}
func TestTaskDetailsRequireReadActionBeforeStoreLookup(t *testing.T) {
	reader := &scopedTaskReader{}
	service := NewQueryService(nil, reader, nil, &taskReadScope{})
	// A task-list permission cannot be borrowed for a detail read.
	result, err := service.GetTask(enrollmentContext(), 1, "11")
	if err == nil || result != nil || reader.calls != 0 {
		t.Fatal("read action was not checked before data lookup")
	}
}

type taskListScope struct{ taskReadScope }

func (*taskListScope) ListStoreScopedTesteeIDs(_ context.Context, org, user int64, resource, action string) ([]uint64, error) {
	if org != 1 || user != 2 || resource != appauthz.EvaluationPlanTaskResource || action != "list" {
		return nil, errors.New("wrong scope")
	}
	return []uint64{3, 4}, nil
}

type filteredTaskReader struct {
	planreadmodel.TaskReader
	filter planreadmodel.TaskFilter
}

func (r *filteredTaskReader) ListTasks(_ context.Context, filter planreadmodel.TaskFilter, page planreadmodel.PageRequest) (planreadmodel.TaskPage, error) {
	r.filter = filter
	return planreadmodel.TaskPage{Items: []planreadmodel.TaskRow{}, Page: page.Page, PageSize: page.PageSize}, nil
}
func TestTaskListAlwaysIntersectsTrustedStoreRange(t *testing.T) {
	for _, restrict := range []bool{false, true} {
		reader := &filteredTaskReader{}
		service := NewQueryService(nil, reader, nil, &taskListScope{})
		_, err := service.ListTasks(enrollmentContext(), ListTasksDTO{OrgID: 1, TesteeID: "3", RestrictToAccessScope: restrict, AccessibleTesteeIDs: []string{"4", "99"}})
		if err != nil {
			t.Fatal(err)
		}
		if !reader.filter.RestrictToAccessScope || reader.filter.TesteeID == nil || *reader.filter.TesteeID != 3 {
			t.Fatalf("lost scope/filter: %+v", reader.filter)
		}
		want := []uint64{3, 4}
		if restrict {
			want = []uint64{4}
		}
		if len(reader.filter.AccessibleTesteeIDs) != len(want) {
			t.Fatalf("range=%v", reader.filter.AccessibleTesteeIDs)
		}
		for i, id := range want {
			if reader.filter.AccessibleTesteeIDs[i] != id {
				t.Fatalf("range=%v", reader.filter.AccessibleTesteeIDs)
			}
		}
	}
}

type taskWindowScope struct {
	taskReadScope
	ids []uint64
}

func (s *taskWindowScope) ListStoreScopedTesteeIDs(_ context.Context, org, user int64, resource, action string) ([]uint64, error) {
	if org != 1 || user != 2 || resource != appauthz.EvaluationPlanTaskResource || action != "list" {
		return nil, errors.New("wrong scope")
	}
	return s.ids, nil
}
func TestTaskWindowNeverForwardsEmptyAuthorizationAsUnrestricted(t *testing.T) {
	for _, ids := range [][]uint64{nil, {4}} {
		// The embedded reader panics if called; both ranges must return empty
		// before the reader's empty-ID-as-unrestricted behavior can be reached.
		service := NewQueryService(nil, &filteredTaskReader{}, nil, &taskWindowScope{ids: ids})
		result, err := service.ListTaskWindow(enrollmentContext(), ListTaskWindowDTO{OrgID: 1, PlanID: "11", TesteeIDs: []string{"3"}, Page: 2, PageSize: 10})
		if err != nil || result == nil || len(result.Items) != 0 || result.HasMore {
			t.Fatalf("window=%+v err=%v", result, err)
		}
	}
}
func TestTaskWindowIntersectsRequestedIDsBeforePagination(t *testing.T) {
	reader := &taskReadModelStub{}
	service := NewQueryService(nil, reader, nil, &taskWindowScope{ids: []uint64{3, 4}})
	_, err := service.ListTaskWindow(enrollmentContext(), ListTaskWindowDTO{OrgID: 1, PlanID: "11", TesteeIDs: []string{"4", "99"}, Page: 2, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.lastWindowFilter.TesteeIDs) != 1 || reader.lastWindowFilter.TesteeIDs[0] != 4 || reader.lastWindowPage.Page != 2 {
		t.Fatalf("filter=%+v page=%+v", reader.lastWindowFilter, reader.lastWindowPage)
	}
}

type tasksByPlanScopeReader struct {
	planreadmodel.TaskReader
	ids   []uint64
	calls int
}

func (r *tasksByPlanScopeReader) ListTasksByPlanIDAndTesteeIDs(_ context.Context, planID uint64, ids []uint64) ([]planreadmodel.TaskRow, error) {
	r.calls++
	r.ids = append([]uint64(nil), ids...)
	if planID != 11 {
		return nil, errors.New("wrong plan")
	}
	return []planreadmodel.TaskRow{}, nil
}
func TestTasksByPlanUsesScopeForEveryEntry(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested []string
		narrow    bool
		want      []uint64
	}{
		{"default", nil, false, []uint64{3, 4}},
		{"intersection", []string{"4", "99"}, true, []uint64{4}},
		{"empty", nil, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &tasksByPlanScopeReader{}
			service := NewQueryService(&planReadModelStub{}, reader, nil, &taskWindowScope{ids: []uint64{3, 4}})
			var err error
			if tc.narrow {
				_, err = service.ListTasksByPlanInScope(enrollmentContext(), 1, "11", tc.requested)
			} else {
				_, err = service.ListTasksByPlan(enrollmentContext(), 1, "11")
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(reader.ids) != len(tc.want) {
				t.Fatalf("ids=%v want=%v", reader.ids, tc.want)
			}
			for i, id := range tc.want {
				if reader.ids[i] != id {
					t.Fatalf("ids=%v", reader.ids)
				}
			}
			if len(tc.want) == 0 && reader.calls != 0 {
				t.Fatal("empty scope queried tasks")
			}
		})
	}
}

type testeeParticipationScope struct {
	resource string
	denied   bool
	calls    int
}

func (s *testeeParticipationScope) ValidateTesteeStoreAccess(_ context.Context, org, user int64, testee uint64, resource, action string) error {
	s.calls++
	if org != 1 || user != 2 || testee != 3 || resource != s.resource || action != "list" {
		return errors.New("wrong participation scope")
	}
	if s.denied {
		return errors.New("outside store")
	}
	return nil
}
func TestTesteeParticipationChecksOwnActionAndStoreBeforeReading(t *testing.T) {
	cases := []struct {
		name, resource string
		run            func(PlanQueryService, context.Context) error
	}{
		{"tasks", appauthz.EvaluationPlanTaskResource, func(s PlanQueryService, ctx context.Context) error {
			_, err := s.ListTasksByTestee(ctx, "3")
			return err
		}},
		{"plans", appauthz.EvaluationPlanResource, func(s PlanQueryService, ctx context.Context) error {
			_, err := s.ListPlansByTestee(ctx, "3")
			return err
		}},
		{"tasks in plan", appauthz.EvaluationPlanTaskResource, func(s PlanQueryService, ctx context.Context) error {
			_, err := s.ListTasksByTesteeAndPlan(ctx, "3", "11")
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scope := &testeeParticipationScope{resource: tc.resource, denied: true}
			ctx := appauthz.WithSnapshot(enrollmentContext(), &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: tc.resource, Action: "list", Mode: appauthz.AuthorizationModeUnconditional}}})
			// Nil dependencies demonstrate rejection before private-data reads.
			service := NewQueryService(nil, nil, nil, scope)
			if err := tc.run(service, ctx); err == nil || scope.calls != 1 {
				t.Fatalf("missing store check: %v", err)
			}
			scope.denied = false
			service = NewQueryService(&planReadModelStub{}, &taskReadModelStub{}, nil, scope)
			if err := tc.run(service, ctx); err != nil {
				t.Fatalf("allowed: %v", err)
			}
			calls := scope.calls
			if err := tc.run(service, appauthz.WithSnapshot(ctx, &appauthz.Snapshot{})); err == nil || scope.calls != calls {
				t.Fatal("missing action reached data scope")
			}
		})
	}
}
