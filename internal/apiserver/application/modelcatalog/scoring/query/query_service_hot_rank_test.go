package query

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel/hotrank"
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

type hotScaleReaderStub struct {
	summaries        []scalereadmodel.ScaleSummaryRow
	findSummaryCalls int
}

func (r *hotScaleReaderStub) ListScales(context.Context, scalereadmodel.ScaleFilter, scalereadmodel.PageRequest) ([]scalereadmodel.ScaleSummaryRow, error) {
	r.findSummaryCalls++
	return append([]scalereadmodel.ScaleSummaryRow(nil), r.summaries...), nil
}

func (r *hotScaleReaderStub) CountScales(context.Context, scalereadmodel.ScaleFilter) (int64, error) {
	return int64(len(r.summaries)), nil
}

func TestListHotPublishedUsesHotRankReadModelOrdering(t *testing.T) {
	scaleA := mustHotScale(t, "S-A", "Q-A")
	scaleB := mustHotScale(t, "S-B", "Q-B")
	scaleC := mustHotScale(t, "S-C", "Q-C")
	reader := &hotScaleReaderStub{
		summaries: []scalereadmodel.ScaleSummaryRow{scaleA, scaleB, scaleC},
	}
	rank := &hotRankReadModelStub{
		entries: []hotrank.Entry{
			{QuestionnaireCode: "Q-B", Score: 7},
			{QuestionnaireCode: "Q-A", Score: 5},
		},
	}
	publishedReader, publishedRepo := hotPublishedSources(t, scaleA, scaleB, scaleC)
	svc := newHotQueryService(reader, publishedReader, publishedRepo, rank)

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
			reader := &hotScaleReaderStub{
				summaries: []scalereadmodel.ScaleSummaryRow{scaleA, scaleB},
			}
			publishedReader, publishedRepo := hotPublishedSources(t, scaleA, scaleB)
			svc := newHotQueryService(reader, publishedReader, publishedRepo, tc.rank)

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
			if reader.findSummaryCalls != 1 {
				t.Fatalf("find summary calls = %d, want 1", reader.findSummaryCalls)
			}
		})
	}
}

func TestResolveAssessmentScaleContextUsesPublishedReader(t *testing.T) {
	item := mustHotScale(t, "S-A", "Q-A")
	publishedReader, _ := hotPublishedSources(t, item)
	svc := newHotQueryService(&hotScaleReaderStub{}, publishedReader, nil, nil)

	result, err := svc.ResolveAssessmentScaleContext(context.Background(), "Q-A", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveAssessmentScaleContext() error = %v", err)
	}
	if result == nil || result.MedicalScaleCode == nil || *result.MedicalScaleCode != "S-A" {
		t.Fatalf("ResolveAssessmentScaleContext() = %+v, want scale code S-A", result)
	}
	if len(publishedReader.calls) != 1 || publishedReader.calls[0] != "Q-A:1.0.0" {
		t.Fatalf("published reader calls = %#v, want Q-A:1.0.0", publishedReader.calls)
	}
}

func TestResolveAssessmentScaleContextReturnsEmptyWhenScaleBindingNotFound(t *testing.T) {
	svc := newHotQueryService(&hotScaleReaderStub{}, &hotPublishedReaderStub{byQuestionnaire: map[string]*port.PublishedModel{}}, nil, nil)

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
	publishedReader := &hotPublishedReaderStub{
		byQuestionnaire: map[string]*port.PublishedModel{},
		err:             repoErr,
	}
	svc := newHotQueryService(&hotScaleReaderStub{}, publishedReader, nil, nil)

	result, err := svc.ResolveAssessmentScaleContext(context.Background(), "Q-A", "1.0.0")
	if !errors.Is(err, repoErr) {
		t.Fatalf("ResolveAssessmentScaleContext() error = %v, want %v", err, repoErr)
	}
	if result != nil {
		t.Fatalf("ResolveAssessmentScaleContext() result = %+v, want nil on repository error", result)
	}
}

func mustHotScale(t *testing.T, code, questionnaireCode string) scalereadmodel.ScaleSummaryRow {
	t.Helper()
	return scalereadmodel.ScaleSummaryRow{
		Code:              code,
		Title:             code + " title",
		Category:          "adhd",
		QuestionnaireCode: questionnaireCode,
		Status:            string(domain.ModelStatusPublished),
		CreatedBy:         meta.ID(101),
		UpdatedBy:         meta.ID(102),
	}
}
