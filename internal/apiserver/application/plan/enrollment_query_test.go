package plan

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"testing"
)

type enrollmentQueryStoreStub struct{ items []EnrollmentItem }

func (s enrollmentQueryStoreStub) ListEnrollments(context.Context, EnrollmentQuery) ([]EnrollmentItem, int64, error) {
	return append([]EnrollmentItem(nil), s.items...), int64(len(s.items)), nil
}

type enrollmentScaleCatalogStub struct{}

func (enrollmentScaleCatalogStub) ExistsByCode(context.Context, string) (bool, error) {
	return true, nil
}
func (enrollmentScaleCatalogStub) ResolveTitle(_ context.Context, code string) string {
	return "title:" + code
}
func (enrollmentScaleCatalogStub) ResolveTitles(_ context.Context, codes []string) map[string]string {
	result := make(map[string]string, len(codes))
	for _, code := range codes {
		result[code] = "title:" + code
	}
	return result
}

func TestEnrollmentQueryProjectsRoundSummary(t *testing.T) {
	service := NewEnrollmentQueryService(enrollmentQueryStoreStub{items: []EnrollmentItem{{
		ID: 1,
		Tasks: []EnrollmentTaskItem{
			{ID: 11, ScaleCode: "S-1", Status: "completed"},
			{ID: 12, ScaleCode: "S-1", Status: "opened"},
		},
	}}}, enrollmentScaleCatalogStub{}, enrollmentScopeStub{})

	page, err := service.ListEnrollments(enrollmentContext(), EnrollmentQuery{OrgID: 1, TesteeID: 3, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	item := page.Items[0]
	if item.ScaleCode != "S-1" || item.ScaleTitle != "title:S-1" {
		t.Fatalf("scale projection = (%q,%q)", item.ScaleCode, item.ScaleTitle)
	}
	if item.TaskCount != 2 || item.CompletedTaskCount != 1 || item.CompletionRate != 0.5 {
		t.Fatalf("task summary = %+v", item)
	}
}

type enrollmentScopeStub struct{ err error }

func (s enrollmentScopeStub) ValidateTesteeStoreAccess(_ context.Context, org, user int64, testee uint64, resource, action string) error {
	if org != 1 || user != 2 || testee != 3 || resource != appauthz.EvaluationPlanTaskResource || action != "list" {
		return errors.New("wrong scope action")
	}
	return s.err
}
func enrollmentContext() context.Context {
	ctx := actorctx.WithGrantingUserID(context.Background(), 2)
	ctx = actorctx.WithOperatorOrgID(ctx, 1)
	return appauthz.WithSnapshot(ctx, &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: appauthz.EvaluationPlanTaskResource, Action: "list", Mode: appauthz.AuthorizationModeUnconditional}}})
}
func TestEnrollmentQueryRejectsBeforeReadingRecordsOrCount(t *testing.T) {
	cases := []struct {
		name   string
		ctx    context.Context
		access EnrollmentScopeChecker
		org    int64
	}{
		{"missing actor", context.Background(), enrollmentScopeStub{}, 1},
		{"missing checker", enrollmentContext(), nil, 1},
		{"foreign company", enrollmentContext(), enrollmentScopeStub{}, 9},
		{"outside store", enrollmentContext(), enrollmentScopeStub{err: errors.New("outside store")}, 1},
		{"missing action", appauthz.WithSnapshot(enrollmentContext(), &appauthz.Snapshot{}), enrollmentScopeStub{}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Nil store/catalog panic if an unauthorized request reaches either read.
			service := NewEnrollmentQueryService(nil, nil, tc.access)
			result, err := service.ListEnrollments(tc.ctx, EnrollmentQuery{OrgID: tc.org, TesteeID: 3})
			if err == nil || result != nil {
				t.Fatalf("unauthorized result=%+v err=%v", result, err)
			}
		})
	}
}
