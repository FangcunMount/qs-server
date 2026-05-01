package scale

import (
	"context"
	"errors"
	"testing"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type hotRankReadModelStub struct {
	entries []domainScale.ScaleHotRankEntry
	err     error
	queries []domainScale.ScaleHotRankQuery
}

func (s *hotRankReadModelStub) Top(_ context.Context, query domainScale.ScaleHotRankQuery) ([]domainScale.ScaleHotRankEntry, error) {
	s.queries = append(s.queries, query)
	if s.err != nil {
		return nil, s.err
	}
	return s.entries, nil
}

type hotScaleRepoStub struct {
	byQuestionnaire          map[string]*domainScale.MedicalScale
	byCode                   map[string]*domainScale.MedicalScale
	summaries                []*domainScale.MedicalScale
	findByQuestionnaireCalls []string
	findSummaryCalls         int
}

func (r *hotScaleRepoStub) Create(context.Context, *domainScale.MedicalScale) error { return nil }
func (r *hotScaleRepoStub) FindByCode(_ context.Context, code string) (*domainScale.MedicalScale, error) {
	if item, ok := r.findByCode(code); ok {
		return item, nil
	}
	return nil, errors.New("not found")
}
func (r *hotScaleRepoStub) FindByQuestionnaireCode(_ context.Context, questionnaireCode string) (*domainScale.MedicalScale, error) {
	r.findByQuestionnaireCalls = append(r.findByQuestionnaireCalls, questionnaireCode)
	return r.byQuestionnaire[questionnaireCode], nil
}
func (r *hotScaleRepoStub) ListScales(context.Context, scalereadmodel.ScaleFilter, scalereadmodel.PageRequest) ([]scalereadmodel.ScaleSummaryRow, error) {
	r.findSummaryCalls++
	return hotScaleRows(r.summaries), nil
}
func (r *hotScaleRepoStub) CountScales(context.Context, scalereadmodel.ScaleFilter) (int64, error) {
	return int64(len(r.summaries)), nil
}
func (r *hotScaleRepoStub) Update(context.Context, *domainScale.MedicalScale) error { return nil }
func (r *hotScaleRepoStub) Remove(context.Context, string) error                    { return nil }
func (r *hotScaleRepoStub) ExistsByCode(context.Context, string) (bool, error)      { return false, nil }

func TestListHotPublishedUsesHotRankReadModelOrdering(t *testing.T) {
	scaleA := mustHotScale(t, "S-A", "Q-A")
	scaleB := mustHotScale(t, "S-B", "Q-B")
	scaleC := mustHotScale(t, "S-C", "Q-C")
	repo := &hotScaleRepoStub{
		byQuestionnaire: map[string]*domainScale.MedicalScale{
			"Q-A": scaleA,
			"Q-B": scaleB,
		},
		summaries: []*domainScale.MedicalScale{scaleA, scaleB, scaleC},
	}
	rank := &hotRankReadModelStub{
		entries: []domainScale.ScaleHotRankEntry{
			{QuestionnaireCode: "Q-B", Score: 7},
			{QuestionnaireCode: "Q-A", Score: 5},
		},
	}
	repo.byCode = hotScaleByCode(scaleA, scaleB, scaleC)
	svc := NewQueryService(repo, repo, nil, nil, nil, rank)

	result, err := svc.ListHotPublished(context.Background(), ListHotScalesDTO{Limit: 3, WindowDays: 14})
	if err != nil {
		t.Fatalf("ListHotPublished() error = %v", err)
	}
	if len(rank.queries) != 1 || rank.queries[0].WindowDays != 14 || rank.queries[0].Limit != 20 {
		t.Fatalf("rank queries = %+v, want window 14 candidate limit 20", rank.queries)
	}
	if len(result.Items) != 3 {
		t.Fatalf("items len = %d, want 3", len(result.Items))
	}
	if result.Items[0].Code != "S-B" || result.Items[0].SubmissionCount != 7 {
		t.Fatalf("first item = %+v, want S-B score 7", result.Items[0])
	}
	if result.Items[1].Code != "S-A" || result.Items[1].SubmissionCount != 5 {
		t.Fatalf("second item = %+v, want S-A score 5", result.Items[1])
	}
	if result.Items[2].Code != "S-C" || result.Items[2].SubmissionCount != 0 {
		t.Fatalf("fallback item = %+v, want S-C score 0", result.Items[2])
	}
}

func TestListHotPublishedFallsBackWhenHotRankEmptyOrUnavailable(t *testing.T) {
	scaleA := mustHotScale(t, "S-A", "Q-A")
	scaleB := mustHotScale(t, "S-B", "Q-B")

	for _, tc := range []struct {
		name string
		rank *hotRankReadModelStub
	}{
		{name: "empty", rank: &hotRankReadModelStub{}},
		{name: "error", rank: &hotRankReadModelStub{err: errors.New("redis down")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &hotScaleRepoStub{
				byQuestionnaire: map[string]*domainScale.MedicalScale{},
				summaries:       []*domainScale.MedicalScale{scaleA, scaleB},
			}
			repo.byCode = hotScaleByCode(scaleA, scaleB)
			svc := NewQueryService(repo, repo, nil, nil, nil, tc.rank)

			result, err := svc.ListHotPublished(context.Background(), ListHotScalesDTO{Limit: 3})
			if err != nil {
				t.Fatalf("ListHotPublished() error = %v", err)
			}
			if len(result.Items) != 2 {
				t.Fatalf("items len = %d, want 2", len(result.Items))
			}
			if result.Items[0].Code != "S-A" || result.Items[0].SubmissionCount != 0 {
				t.Fatalf("first fallback item = %+v, want S-A score 0", result.Items[0])
			}
			if repo.findSummaryCalls != 1 {
				t.Fatalf("find summary calls = %d, want 1", repo.findSummaryCalls)
			}
		})
	}
}

func TestResolveAssessmentScaleContextUsesScaleRepositoryBehindApplicationPort(t *testing.T) {
	item := mustHotScale(t, "S-A", "Q-A")
	repo := &hotScaleRepoStub{
		byQuestionnaire: map[string]*domainScale.MedicalScale{
			"Q-A": item,
		},
	}
	svc := NewQueryService(repo, repo, nil, nil, nil)

	result, err := svc.ResolveAssessmentScaleContext(context.Background(), "Q-A")
	if err != nil {
		t.Fatalf("ResolveAssessmentScaleContext() error = %v", err)
	}
	if result == nil || result.MedicalScaleCode == nil || *result.MedicalScaleCode != "S-A" {
		t.Fatalf("ResolveAssessmentScaleContext() = %+v, want scale code S-A", result)
	}
	if result.MedicalScaleID == nil || *result.MedicalScaleID == 0 {
		t.Fatalf("MedicalScaleID = %+v, want non-zero id", result.MedicalScaleID)
	}
}

func (r *hotScaleRepoStub) findByCode(code string) (*domainScale.MedicalScale, bool) {
	if r.byCode == nil {
		return nil, false
	}
	item, ok := r.byCode[code]
	return item, ok
}

func hotScaleByCode(items ...*domainScale.MedicalScale) map[string]*domainScale.MedicalScale {
	result := make(map[string]*domainScale.MedicalScale, len(items))
	for _, item := range items {
		if item != nil {
			result[item.GetCode().String()] = item
		}
	}
	return result
}

func hotScaleRows(items []*domainScale.MedicalScale) []scalereadmodel.ScaleSummaryRow {
	rows := make([]scalereadmodel.ScaleSummaryRow, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		rows = append(rows, scalereadmodel.ScaleSummaryRow{
			Code:              item.GetCode().String(),
			Title:             item.GetTitle(),
			Description:       item.GetDescription(),
			Category:          item.GetCategory().String(),
			QuestionnaireCode: item.GetQuestionnaireCode().String(),
			Status:            item.GetStatus().String(),
			CreatedBy:         item.GetCreatedBy(),
			CreatedAt:         item.GetCreatedAt(),
			UpdatedBy:         item.GetUpdatedBy(),
			UpdatedAt:         item.GetUpdatedAt(),
		})
	}
	return rows
}

func mustHotScale(t *testing.T, code, questionnaireCode string) *domainScale.MedicalScale {
	t.Helper()
	scale, err := domainScale.NewMedicalScale(
		meta.NewCode(code),
		code+" title",
		domainScale.WithID(meta.ID(901)),
		domainScale.WithQuestionnaire(meta.NewCode(questionnaireCode), "1.0.0"),
		domainScale.WithStatus(domainScale.StatusPublished),
		domainScale.WithCategory(domainScale.CategoryADHD),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return scale
}
