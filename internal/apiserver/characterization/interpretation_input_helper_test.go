package characterization_test

import (
	"context"
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/input"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

type draftReportBuilder interface {
	Build(context.Context, interpinput.InterpretationInput) (*report.Draft, error)
}

func buildLegacyReport(t *testing.T, builder draftReportBuilder, outcome evaloutcome.Outcome) *domainreport.InterpretReport {
	t.Helper()
	input, err := interpretationinput.FromLegacyOutcome(outcome)
	if err != nil {
		t.Fatalf("adapt interpretation input: %v", err)
	}
	draft, err := builder.Build(context.Background(), input)
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}
	return interpretationreporting.LegacyReportFromDraft(input, draft)
}
