package testee

import (
	"context"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domaintestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestParseDate(t *testing.T) {
	t.Parallel()
	got, err := parseDate("2026-04-22", true)
	if err != nil || got == nil || got.Format("2006-01-02") != "2026-04-23" {
		t.Fatalf("unexpected end-exclusive date: %v, %v", got, err)
	}
	got, err = parseDate(time.Date(2026, 4, 22, 8, 0, 0, 0, time.UTC).Format(time.RFC3339), false)
	if err != nil || got == nil {
		t.Fatalf("expected RFC3339 date to parse, got %v, %v", got, err)
	}
	if _, err := parseDate("bad-date", false); err == nil {
		t.Fatal("expected invalid date error")
	}
}

type assessmentRepoStub struct {
	domainassessment.Repository
	value *domainassessment.Assessment
	calls *int
}

func (s assessmentRepoStub) FindByID(context.Context, domainassessment.ID) (*domainassessment.Assessment, error) {
	if s.calls != nil {
		*s.calls++
	}
	return s.value, nil
}

type scoreStub struct{ called bool }

func (s *scoreStub) Get(context.Context, uint64) (*evaloutcome.ScoreFact, error) {
	s.called = true
	return &evaloutcome.ScoreFact{AssessmentID: 1}, nil
}

type testAssessmentCaches struct {
	owner       uint64
	ownerFound  bool
	detail      *Assessment
	detailFound bool
	setDetails  int
}

func (c *testAssessmentCaches) ReadOwner(ctx context.Context, _ uint64, load func(context.Context) (uint64, error)) (uint64, error) {
	if c.ownerFound {
		return c.owner, nil
	}
	owner, err := load(ctx)
	if err == nil {
		c.owner, c.ownerFound = owner, true
	}
	return owner, err
}
func (c *testAssessmentCaches) ReadDetail(ctx context.Context, _ uint64, load func(context.Context) (*Assessment, error)) (*Assessment, error) {
	if c.detailFound {
		return cloneAssessment(c.detail), nil
	}
	value, err := load(ctx)
	if err == nil && value != nil && value.Status == "evaluated" {
		c.detail, c.detailFound = cloneAssessment(value), true
		c.setDetails++
	}
	return value, err
}

type detailReaderStub struct {
	assessmentReaderStub
	row   evaluationreadmodel.AssessmentRow
	calls int
}

func (r *detailReaderStub) GetAssessment(context.Context, uint64) (*evaluationreadmodel.AssessmentRow, error) {
	r.calls++
	row := r.row
	return &row, nil
}

func TestAssessmentAccessCacheStoresOnlyOwnerProjection(t *testing.T) {
	var repoCalls int
	value, err := domainassessment.NewAssessment(9, domaintestee.NewID(7), domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), domainassessment.NewAnswerSheetRef(meta.FromUint64(2)), domainassessment.NewAdhocOrigin(), domainassessment.WithID(meta.FromUint64(1)))
	if err != nil {
		t.Fatal(err)
	}
	caches := &testAssessmentCaches{}
	svc := NewServiceWithCaches(assessmentRepoStub{value: value, calls: &repoCalls}, nil, nil, caches, nil)
	for range 2 {
		if err := svc.AuthorizeAssessment(context.Background(), Actor{TesteeID: 7}, 1); err != nil {
			t.Fatal(err)
		}
	}
	if repoCalls != 1 || !caches.ownerFound || caches.owner != 7 {
		t.Fatalf("repo calls=%d owner=%d found=%v", repoCalls, caches.owner, caches.ownerFound)
	}
	if err := svc.AuthorizeAssessment(context.Background(), Actor{TesteeID: 8}, 1); err == nil {
		t.Fatal("cached owner allowed a foreign testee")
	}
}

func TestAssessmentDetailCacheAdmitsOnlyEvaluatedOutcome(t *testing.T) {
	value, err := domainassessment.NewAssessment(9, domaintestee.NewID(7), domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), domainassessment.NewAnswerSheetRef(meta.FromUint64(2)), domainassessment.NewAdhocOrigin(), domainassessment.WithID(meta.FromUint64(1)))
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"pending", "submitted", "failed", "evaluated"} {
		t.Run(status, func(t *testing.T) {
			caches := &testAssessmentCaches{owner: 7, ownerFound: true}
			reader := &detailReaderStub{row: evaluationreadmodel.AssessmentRow{ID: 1, OrgID: 9, TesteeID: 7, Status: status}}
			svc := NewServiceWithCaches(assessmentRepoStub{value: value}, reader, nil, caches, caches)
			for range 2 {
				if _, err := svc.GetAssessment(context.Background(), Actor{TesteeID: 7}, 1); err != nil {
					t.Fatal(err)
				}
			}
			wantReads, wantSets := 2, 0
			if status == "evaluated" {
				wantReads, wantSets = 1, 1
			}
			if reader.calls != wantReads || caches.setDetails != wantSets {
				t.Fatalf("reader calls=%d sets=%d, want %d/%d", reader.calls, caches.setDetails, wantReads, wantSets)
			}
		})
	}
}
func (*scoreStub) Trend(context.Context, uint64, string, int) (*evaloutcome.FactorTrendFact, error) {
	return nil, nil
}

func TestGetScoreRejectsNonOwnerBeforeReadingScore(t *testing.T) {
	t.Parallel()
	scores := &scoreStub{}
	a, err := domainassessment.NewAssessment(9, domaintestee.NewID(7), domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), domainassessment.NewAnswerSheetRef(meta.FromUint64(2)), domainassessment.NewAdhocOrigin(), domainassessment.WithID(meta.FromUint64(1)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(assessmentRepoStub{value: a}, nil, scores)
	if _, err := svc.GetScore(context.Background(), Actor{TesteeID: 8}, 1); err == nil {
		t.Fatal("expected ownership error")
	}
	if scores.called {
		t.Fatal("score reader called before ownership was established")
	}
}

func TestNormalizeStatusesDoesNotReintroduceInterpreted(t *testing.T) {
	t.Parallel()
	got := normalizeStatuses("done")
	if len(got) != 1 || got[0] != "evaluated" {
		t.Fatalf("done statuses = %v, want evaluated only", got)
	}
}

type assessmentReaderStub struct {
	filter evaluationreadmodel.AssessmentFilter
	page   evaluationreadmodel.PageRequest
}

func (*assessmentReaderStub) GetAssessment(context.Context, uint64) (*evaluationreadmodel.AssessmentRow, error) {
	return nil, nil
}

func (*assessmentReaderStub) GetAssessmentByAnswerSheetID(context.Context, uint64) (*evaluationreadmodel.AssessmentRow, error) {
	return nil, nil
}

func (s *assessmentReaderStub) ListAssessments(_ context.Context, filter evaluationreadmodel.AssessmentFilter, page evaluationreadmodel.PageRequest) ([]evaluationreadmodel.AssessmentRow, int64, error) {
	s.filter = filter
	s.page = page
	return nil, 0, nil
}

func TestListAssessmentsKeepsReadModelPaginationAndEmptyListContract(t *testing.T) {
	t.Parallel()

	reader := &assessmentReaderStub{}
	svc := NewService(nil, reader, nil)
	result, err := svc.ListAssessments(context.Background(), Actor{TesteeID: 42}, ListQuery{
		Page: 0, PageSize: 101, Status: "done", ScaleCode: "scale-a", RiskLevel: "high",
		ModelKind: "scale", ModelKinds: []string{"scale"}, ModelCode: "scale-a",
		DateFrom: "2026-08-01", DateTo: "2026-08-02",
	})
	if err != nil {
		t.Fatalf("ListAssessments() error = %v", err)
	}
	if result == nil || result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("ListAssessments() items = %#v, want non-nil empty slice", result)
	}
	if result.Page != 1 || result.PageSize != 100 || result.Total != 0 || result.TotalPages != 0 {
		t.Fatalf("ListAssessments() pagination = %#v", result)
	}
	if reader.page.Page != 1 || reader.page.PageSize != 100 {
		t.Fatalf("read-model page = %#v", reader.page)
	}
	if reader.filter.TesteeID == nil || *reader.filter.TesteeID != 42 ||
		len(reader.filter.Statuses) != 1 || reader.filter.Statuses[0] != "evaluated" ||
		reader.filter.ScaleCode != "scale-a" || reader.filter.RiskLevel != "high" ||
		reader.filter.ModelKind != "scale" || len(reader.filter.ModelKinds) != 1 ||
		reader.filter.ModelCode != "scale-a" || reader.filter.DateFrom == nil || reader.filter.DateTo == nil {
		t.Fatalf("read-model filter = %#v", reader.filter)
	}
}
