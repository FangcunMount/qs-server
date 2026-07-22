package plan

import (
	"context"
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
	}}}, enrollmentScaleCatalogStub{})

	page, err := service.ListEnrollments(context.Background(), EnrollmentQuery{Page: 1, PageSize: 20})
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
