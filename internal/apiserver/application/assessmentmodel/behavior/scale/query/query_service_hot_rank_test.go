package query

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/behavior/scale/shared"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition/hotrank"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type hotRankReadModelStub struct {
	entries []hotrank.Entry
	err     error
	queries []hotrank.Query
}

func (s *hotRankReadModelStub) Top(_ context.Context, query hotrank.Query) ([]hotrank.Entry, error) {
	s.queries = append(s.queries, query)
	if s.err != nil {
		return nil, s.err
	}
	return s.entries, nil
}

type hotScaleRepoStub struct {
	byQuestionnaire           map[string]*scaledefinition.MedicalScale
	byCode                    map[string]*scaledefinition.MedicalScale
	summaries                 []*scaledefinition.MedicalScale
	findByQuestionnaireErr    error
	findByQuestionnaireRefErr error
	findByQuestionnaireCalls  []string
	findByQuestionnaireRefs   []string
	findSummaryCalls          int
}

func (r *hotScaleRepoStub) Create(context.Context, *scaledefinition.MedicalScale) error { return nil }
func (r *hotScaleRepoStub) CreatePublishedSnapshot(context.Context, *scaledefinition.MedicalScale, bool) error {
	return nil
}
func (r *hotScaleRepoStub) FindByCode(_ context.Context, code string) (*scaledefinition.MedicalScale, error) {
	if item, ok := r.findByCode(code); ok {
		return item, nil
	}
	return nil, errors.New("not found")
}
func (r *hotScaleRepoStub) FindPublishedByCode(ctx context.Context, code string) (*scaledefinition.MedicalScale, error) {
	return r.FindByCode(ctx, code)
}
func (r *hotScaleRepoStub) FindByCodeVersion(ctx context.Context, code, _ string) (*scaledefinition.MedicalScale, error) {
	return r.FindByCode(ctx, code)
}
func (r *hotScaleRepoStub) FindByQuestionnaireCode(_ context.Context, questionnaireCode string) (*scaledefinition.MedicalScale, error) {
	r.findByQuestionnaireCalls = append(r.findByQuestionnaireCalls, questionnaireCode)
	if r.findByQuestionnaireErr != nil {
		return nil, r.findByQuestionnaireErr
	}
	return r.byQuestionnaire[questionnaireCode], nil
}
func (r *hotScaleRepoStub) FindPublishedByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*scaledefinition.MedicalScale, error) {
	return r.FindByQuestionnaireCode(ctx, questionnaireCode)
}
func (r *hotScaleRepoStub) FindByQuestionnaireRef(_ context.Context, questionnaireCode, questionnaireVersion string) (*scaledefinition.MedicalScale, error) {
	r.findByQuestionnaireRefs = append(r.findByQuestionnaireRefs, questionnaireCode+":"+questionnaireVersion)
	if r.findByQuestionnaireRefErr != nil {
		return nil, r.findByQuestionnaireRefErr
	}
	if item, ok := r.byQuestionnaire[questionnaireCode]; ok && item.GetQuestionnaireVersion() == questionnaireVersion {
		return item, nil
	}
	return nil, scaledefinition.ErrNotFound
}
func (r *hotScaleRepoStub) ListScales(context.Context, scalereadmodel.ScaleFilter, scalereadmodel.PageRequest) ([]scalereadmodel.ScaleSummaryRow, error) {
	r.findSummaryCalls++
	return hotScaleRows(r.summaries), nil
}
func (r *hotScaleRepoStub) CountScales(context.Context, scalereadmodel.ScaleFilter) (int64, error) {
	return int64(len(r.summaries)), nil
}
func (r *hotScaleRepoStub) Update(context.Context, *scaledefinition.MedicalScale) error { return nil }
func (r *hotScaleRepoStub) Remove(context.Context, string) error                        { return nil }
func (r *hotScaleRepoStub) ExistsByCode(context.Context, string) (bool, error)          { return false, nil }
func (r *hotScaleRepoStub) SetActivePublishedVersion(context.Context, string, string) error {
	return nil
}
func (r *hotScaleRepoStub) ClearActivePublishedVersion(context.Context, string) error {
	return nil
}

func TestListHotPublishedUsesHotRankReadModelOrdering(t *testing.T) {
	scaleA := mustHotScale(t, "S-A", "Q-A")
	scaleB := mustHotScale(t, "S-B", "Q-B")
	scaleC := mustHotScale(t, "S-C", "Q-C")
	repo := &hotScaleRepoStub{
		byQuestionnaire: map[string]*scaledefinition.MedicalScale{
			"Q-A": scaleA,
			"Q-B": scaleB,
		},
		summaries: []*scaledefinition.MedicalScale{scaleA, scaleB, scaleC},
	}
	rank := &hotRankReadModelStub{
		entries: []hotrank.Entry{
			{QuestionnaireCode: "Q-B", Score: 7},
			{QuestionnaireCode: "Q-A", Score: 5},
		},
	}
	repo.byCode = hotScaleByCode(scaleA, scaleB, scaleC)
	svc := NewQueryService(repo, repo, nil, nil, nil, rank)

	result, err := svc.ListHotPublished(context.Background(), shared.ListHotScalesDTO{Limit: 3, WindowDays: 14})
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
				byQuestionnaire: map[string]*scaledefinition.MedicalScale{},
				summaries:       []*scaledefinition.MedicalScale{scaleA, scaleB},
			}
			repo.byCode = hotScaleByCode(scaleA, scaleB)
			svc := NewQueryService(repo, repo, nil, nil, nil, tc.rank)

			result, err := svc.ListHotPublished(context.Background(), shared.ListHotScalesDTO{Limit: 3})
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
		byQuestionnaire: map[string]*scaledefinition.MedicalScale{
			"Q-A": item,
		},
	}
	svc := NewQueryService(repo, repo, nil, nil, nil)

	result, err := svc.ResolveAssessmentScaleContext(context.Background(), "Q-A", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveAssessmentScaleContext() error = %v", err)
	}
	if result == nil || result.MedicalScaleCode == nil || *result.MedicalScaleCode != "S-A" {
		t.Fatalf("ResolveAssessmentScaleContext() = %+v, want scale code S-A", result)
	}
	if result.MedicalScaleID == nil || *result.MedicalScaleID == 0 {
		t.Fatalf("MedicalScaleID = %+v, want non-zero id", result.MedicalScaleID)
	}
	if len(repo.findByQuestionnaireRefs) != 1 || repo.findByQuestionnaireRefs[0] != "Q-A:1.0.0" {
		t.Fatalf("FindByQuestionnaireRef calls = %#v, want Q-A:1.0.0", repo.findByQuestionnaireRefs)
	}
}

func TestResolveAssessmentScaleContextReturnsEmptyWhenScaleBindingNotFound(t *testing.T) {
	repo := &hotScaleRepoStub{
		byQuestionnaire: map[string]*scaledefinition.MedicalScale{},
	}
	svc := NewQueryService(repo, repo, nil, nil, nil)

	result, err := svc.ResolveAssessmentScaleContext(context.Background(), "Q-A", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveAssessmentScaleContext() error = %v", err)
	}
	if result == nil ||
		result.MedicalScaleCode != nil ||
		result.MedicalScaleID != nil ||
		result.ScaleVersion != nil {
		t.Fatalf("ResolveAssessmentScaleContext() = %+v, want empty scale context", result)
	}
}

func TestResolveAssessmentScaleContextReturnsRepositoryError(t *testing.T) {
	repoErr := errors.New("mongo unavailable")
	repo := &hotScaleRepoStub{
		byQuestionnaire:           map[string]*scaledefinition.MedicalScale{},
		findByQuestionnaireRefErr: repoErr,
	}
	svc := NewQueryService(repo, repo, nil, nil, nil)

	result, err := svc.ResolveAssessmentScaleContext(context.Background(), "Q-A", "1.0.0")
	if !errors.Is(err, repoErr) {
		t.Fatalf("ResolveAssessmentScaleContext() error = %v, want %v", err, repoErr)
	}
	if result != nil {
		t.Fatalf("ResolveAssessmentScaleContext() result = %+v, want nil on repository error", result)
	}
}

func (r *hotScaleRepoStub) findByCode(code string) (*scaledefinition.MedicalScale, bool) {
	if r.byCode == nil {
		return nil, false
	}
	item, ok := r.byCode[code]
	return item, ok
}

func hotScaleByCode(items ...*scaledefinition.MedicalScale) map[string]*scaledefinition.MedicalScale {
	result := make(map[string]*scaledefinition.MedicalScale, len(items))
	for _, item := range items {
		if item != nil {
			result[item.GetCode().String()] = item
		}
	}
	return result
}

func hotScaleRows(items []*scaledefinition.MedicalScale) []scalereadmodel.ScaleSummaryRow {
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

func mustHotScale(t *testing.T, code, questionnaireCode string) *scaledefinition.MedicalScale {
	t.Helper()
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode(code),
		code+" title",
		scaledefinition.WithID(meta.ID(901)),
		scaledefinition.WithQuestionnaire(meta.NewCode(questionnaireCode), "1.0.0"),
		scaledefinition.WithStatus(scaledefinition.StatusPublished),
		scaledefinition.WithCategory(scaledefinition.CategoryADHD),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return scale
}
