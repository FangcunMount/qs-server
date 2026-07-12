package operations

import (
	"context"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"testing"
)

func TestOperationsRequiresAuditPermissionBeforeRepositoryRead(t *testing.T) {
	g := &genRepo{}
	s := NewService(outcome{}, g, runRepo{}, reportRepo{})
	_, err := s.FindGenerationsByOutcomeID(context.Background(), Actor{OperatorUserID: 1}, meta.ID(2))
	if err == nil {
		t.Fatal("expected permission error")
	}
	if g.calls != 0 {
		t.Fatal("repository read before authorization")
	}
}

type outcome struct{}

func (outcome) FindOutcomeIDByAssessmentID(context.Context, meta.ID) (meta.ID, error) {
	return meta.ID(1), nil
}

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
func (runRepo) ListByGenerationID(context.Context, meta.ID) ([]*domainrun.InterpretationRun, error) {
	return nil, nil
}
func (runRepo) Save(context.Context, *domainrun.InterpretationRun) error { return nil }

type reportRepo struct{}

func (reportRepo) Insert(context.Context, *domainreport.InterpretReport) error { return nil }
func (reportRepo) FindByID(context.Context, meta.ID) (*domainreport.InterpretReport, error) {
	return nil, nil
}
func (reportRepo) FindByGenerationID(context.Context, meta.ID) (*domainreport.InterpretReport, error) {
	return nil, nil
}
func (reportRepo) FindLatestByAssessmentID(context.Context, meta.ID) (*domainreport.InterpretReport, error) {
	return nil, nil
}
func (reportRepo) ListByAssessmentID(context.Context, meta.ID) ([]*domainreport.InterpretReport, error) {
	return nil, nil
}
