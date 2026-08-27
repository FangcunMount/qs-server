package source

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestResolveCurrentUsesCatalogSourceAndOutcomeRuntime(t *testing.T) {
	report := testReport(t, 101, domainreport.BuilderIdentityFactorScoring, domainreport.ContentSchemaVersionV1)
	outcome := testOutcome(report, modelcatalog.DecisionKindScoreRange)
	catalog := &catalogStub{metadata: foundMetadata(report)}
	reports := &reportRepositoryStub{byID: map[meta.ID]*domainreport.InterpretReport{report.ID(): report}}
	outcomes := &outcomeRepositoryStub{byID: map[meta.ID]*evaluationfact.Record{outcome.ID(): outcome}}
	resolver, err := NewResolver(catalog, reports, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	current, err := resolver.ResolveCurrent(context.Background(), report.Association().AssessmentID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Report.ID() != report.ID() || current.Outcome.Runtime().DecisionKind != modelcatalog.DecisionKindScoreRange {
		t.Fatalf("current = %#v", current)
	}
	if reports.requestedID != report.ID() || outcomes.requestedID != report.OutcomeID() {
		t.Fatalf("selected report/outcome = %s/%s", reports.requestedID, outcomes.requestedID)
	}
}

func TestResolveCurrentDoesNotFallBackWhenCatalogIsMissingOrBroken(t *testing.T) {
	report := testReport(t, 101, domainreport.BuilderIdentityFactorScoring, domainreport.ContentSchemaVersionV1)
	tests := []struct {
		name string
		meta interpretationreadmodel.CurrentReportMetadata
		want error
	}{
		{name: "missing", meta: interpretationreadmodel.CurrentReportMetadata{AssessmentID: 7, Status: interpretationreadmodel.CurrentReportMetadataMissing}, want: ErrNotReady},
		{name: "dangling", meta: interpretationreadmodel.CurrentReportMetadata{AssessmentID: 7, Status: interpretationreadmodel.CurrentReportMetadataDangling, SourceKind: "artifact", SourceID: 101}, want: ErrInconsistent},
		{name: "mismatch", meta: interpretationreadmodel.CurrentReportMetadata{AssessmentID: 7, Status: interpretationreadmodel.CurrentReportMetadataMismatch, SourceKind: "artifact", SourceID: 101}, want: ErrInconsistent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := &catalogStub{metadata: tt.meta}
			resolver, err := NewResolver(catalog, &reportRepositoryStub{byID: map[meta.ID]*domainreport.InterpretReport{report.ID(): report}}, &outcomeRepositoryStub{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = resolver.ResolveCurrent(context.Background(), meta.FromUint64(7))
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if catalog.calls != 1 {
				t.Fatalf("catalog calls = %d", catalog.calls)
			}
		})
	}
}

func TestResolveCurrentRejectsLegacyAndAssociationDrift(t *testing.T) {
	legacy := testReport(t, 101, domainreport.UnknownBuilderIdentity, domainreport.LegacyContentSchemaVersion)
	valid := testReport(t, 102, domainreport.BuilderIdentityFactorScoring, domainreport.ContentSchemaVersionV1)
	driftedOutcome := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: valid.OutcomeID(), OrgID: 99, AssessmentID: valid.Association().AssessmentID, TesteeID: valid.Association().TesteeID,
		Model: modelFromReport(valid), Runtime: evaluationfact.RuntimeIdentity{DecisionKind: modelcatalog.DecisionKindScoreRange}, EvaluatedAt: time.Now(),
	})
	tests := []struct {
		name    string
		report  *domainreport.InterpretReport
		outcome *evaluationfact.Record
		want    error
	}{
		{name: "legacy", report: legacy, outcome: testOutcome(legacy, modelcatalog.DecisionKindScoreRange), want: ErrNotApplicable},
		{name: "association drift", report: valid, outcome: driftedOutcome, want: ErrInconsistent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, err := NewResolver(
				&catalogStub{metadata: foundMetadata(tt.report)},
				&reportRepositoryStub{byID: map[meta.ID]*domainreport.InterpretReport{tt.report.ID(): tt.report}},
				&outcomeRepositoryStub{byID: map[meta.ID]*evaluationfact.Record{tt.outcome.ID(): tt.outcome}},
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = resolver.ResolveCurrent(context.Background(), tt.report.Association().AssessmentID)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

type catalogStub struct {
	metadata interpretationreadmodel.CurrentReportMetadata
	err      error
	calls    int
}

func (s *catalogStub) GetCurrentReportMetadataByAssessmentIDs(context.Context, []uint64) (map[uint64]interpretationreadmodel.CurrentReportMetadata, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return map[uint64]interpretationreadmodel.CurrentReportMetadata{s.metadata.AssessmentID: s.metadata}, nil
}

type reportRepositoryStub struct {
	byID        map[meta.ID]*domainreport.InterpretReport
	requestedID meta.ID
}

func (*reportRepositoryStub) Insert(context.Context, *domainreport.InterpretReport) error { return nil }
func (s *reportRepositoryStub) FindByID(_ context.Context, id meta.ID) (*domainreport.InterpretReport, error) {
	s.requestedID = id
	if item := s.byID[id]; item != nil {
		return item, nil
	}
	return nil, domainreport.ErrInterpretReportNotFound
}
func (*reportRepositoryStub) FindByGenerationID(context.Context, meta.ID) (*domainreport.InterpretReport, error) {
	return nil, domainreport.ErrInterpretReportNotFound
}
func (*reportRepositoryStub) ListByAssessmentID(context.Context, meta.ID) ([]*domainreport.InterpretReport, error) {
	return nil, errors.New("ListByAssessmentID must not be used")
}

type outcomeRepositoryStub struct {
	byID        map[meta.ID]*evaluationfact.Record
	requestedID meta.ID
}

func (s *outcomeRepositoryStub) FindByID(_ context.Context, id meta.ID) (*evaluationfact.Record, error) {
	s.requestedID = id
	if item := s.byID[id]; item != nil {
		return item, nil
	}
	return nil, evaluationfact.ErrNotFound
}
func (*outcomeRepositoryStub) FindByAssessmentID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	return nil, errors.New("FindByAssessmentID must not be used")
}

func foundMetadata(report *domainreport.InterpretReport) interpretationreadmodel.CurrentReportMetadata {
	return interpretationreadmodel.CurrentReportMetadata{
		AssessmentID: report.Association().AssessmentID.Uint64(), Status: interpretationreadmodel.CurrentReportMetadataFound,
		SourceKind: "artifact", SourceID: report.ID().Uint64(), CreatedAt: report.GeneratedAt(),
	}
}

func testReport(t *testing.T, id uint64, builder, schema string) *domainreport.InterpretReport {
	t.Helper()
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	report, err := domainreport.RestoreInterpretReport(domainreport.InterpretReportInput{
		ID: meta.FromUint64(id), GenerationID: meta.FromUint64(id + 100), OutcomeID: meta.FromUint64(id + 200), InterpretationRunID: meta.FromUint64(id + 300),
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.FromUint64(7), TesteeID: 9},
		ReportType:  policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionCurrent,
		BuilderIdentity: builder, ContentSchemaVersion: schema, GeneratedAt: now,
		Content: domainreport.Content{Model: domainreport.ModelIdentity{Kind: string(modelcatalog.KindScale), Algorithm: string(modelcatalog.AlgorithmScaleDefault), Code: "scale-a", Version: "v1", Title: "Scale A"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func testOutcome(report *domainreport.InterpretReport, decision modelcatalog.DecisionKind) *evaluationfact.Record {
	association := report.Association()
	return evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: report.OutcomeID(), OrgID: association.OrgID, AssessmentID: association.AssessmentID, TesteeID: association.TesteeID,
		Model: modelFromReport(report), Runtime: evaluationfact.RuntimeIdentity{DecisionKind: decision}, EvaluatedAt: report.GeneratedAt().Add(-time.Second),
	})
}

func modelFromReport(report *domainreport.InterpretReport) evaluationfact.ModelIdentity {
	model := report.Content().Model
	return evaluationfact.ModelIdentity{Kind: modelcatalog.Kind(model.Kind), Algorithm: modelcatalog.Algorithm(model.Algorithm), Code: model.Code, Version: model.Version, Title: model.Title}
}
