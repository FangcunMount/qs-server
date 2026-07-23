package operations

import (
	"context"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestOperationsRejectsUnconfiguredServiceBeforeRepositoryAccess(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)
	_, err := service.FindReportByID(context.Background(), Actor{OrgID: 1, OperatorUserID: 1}, meta.ID(1))
	if !cberrors.IsCode(err, code.ErrModuleInitializationFailed) {
		t.Fatalf("error = %v, want module initialization failure", err)
	}
}

func TestOperationsRequiresAuditPermissionBeforeRepositoryRead(t *testing.T) {
	g := &genRepo{}
	s := NewService(outcome{}, g, runRepo{}, &reportRepo{}, access{err: context.Canceled})
	_, err := s.FindGenerationsByOutcomeID(context.Background(), Actor{OrgID: 1, OperatorUserID: 1}, meta.ID(2))
	if err == nil {
		t.Fatal("expected permission error")
	}
	if g.calls != 0 {
		t.Fatal("repository read before authorization")
	}
}

func TestFindReportAuthorizesMetadataBeforeReadingArtifact(t *testing.T) {
	reports := &reportRepo{metadata: &ArtifactMetadata{ID: meta.ID(9), OrgID: 8}}
	service := NewService(outcome{}, &genRepo{}, runRepo{}, reports, access{err: context.Canceled})

	_, err := service.FindReportByID(context.Background(), Actor{OrgID: 8, OperatorUserID: 1}, meta.ID(9))
	if err == nil {
		t.Fatal("expected authorization error")
	}
	if reports.metadataCalls != 1 || reports.fullCalls != 0 {
		t.Fatalf("metadata/full calls = %d/%d, want 1/0", reports.metadataCalls, reports.fullCalls)
	}
}

type outcome struct{}

func (outcome) FindOutcomeByAssessmentID(context.Context, meta.ID) (OutcomeRef, error) {
	return OutcomeRef{ID: meta.ID(1), OrgID: 1}, nil
}
func (outcome) FindOutcomeByID(context.Context, meta.ID) (OutcomeRef, error) {
	return OutcomeRef{ID: meta.ID(1), OrgID: 1}, nil
}

type access struct{ err error }

func (a access) AuthorizeAudit(context.Context, Actor, int64) error { return a.err }

type genRepo struct{ calls int }

func (g *genRepo) Create(context.Context, *domaingeneration.ReportGeneration) error { return nil }
func (g *genRepo) FindByID(context.Context, meta.ID) (*domaingeneration.ReportGeneration, error) {
	return nil, nil
}
func (g *genRepo) FindByKey(context.Context, domaingeneration.Key) (*domaingeneration.ReportGeneration, error) {
	return nil, nil
}
func (g *genRepo) ListByOutcomeID(context.Context, meta.ID) ([]*domaingeneration.ReportGeneration, error) {
	g.calls++
	return nil, nil
}
func (g *genRepo) Save(context.Context, *domaingeneration.ReportGeneration, uint64) error {
	return nil
}

type runRepo struct{}

func (runRepo) Create(context.Context, *domainrun.InterpretationRun) error { return nil }
func (runRepo) FindByID(context.Context, meta.ID) (*domainrun.InterpretationRun, error) {
	return nil, nil
}
func (runRepo) FindLatestByGenerationID(context.Context, meta.ID) (*domainrun.InterpretationRun, error) {
	return nil, nil
}
func (runRepo) Save(context.Context, *domainrun.InterpretationRun) error { return nil }

type reportRepo struct {
	metadata      *ArtifactMetadata
	metadataCalls int
	fullCalls     int
}

func (*reportRepo) Insert(context.Context, *domainreport.InterpretReport) error { return nil }
func (r *reportRepo) FindMetadataByID(context.Context, meta.ID) (*ArtifactMetadata, error) {
	r.metadataCalls++
	return r.metadata, nil
}
func (r *reportRepo) FindByID(context.Context, meta.ID) (*domainreport.InterpretReport, error) {
	r.fullCalls++
	return nil, nil
}
func (*reportRepo) FindByGenerationID(context.Context, meta.ID) (*domainreport.InterpretReport, error) {
	return nil, nil
}
func (*reportRepo) ListByAssessmentID(context.Context, meta.ID) ([]*domainreport.InterpretReport, error) {
	return nil, nil
}
