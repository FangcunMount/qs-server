// Package report owns the interpretation report aggregate and section structure.
package report

import domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"

type (
	ID                 = domainreport.ID
	AssessmentID       = domainreport.AssessmentID
	InterpretReport    = domainreport.InterpretReport
	DimensionInterpret = domainreport.DimensionInterpret
	Suggestion         = domainreport.Suggestion
	ReportType         = domainreport.ReportType
	RiskLevel          = domainreport.RiskLevel
	ModelIdentity      = domainreport.ModelIdentity
	ScoreValue         = domainreport.ScoreValue
	ResultLevel        = domainreport.ResultLevel
	ModelExtra         = domainreport.ModelExtra
)

const ReportTypeStandard = domainreport.ReportTypeStandard

var (
	NewInterpretReport         = domainreport.NewInterpretReport
	ReconstructInterpretReport = domainreport.ReconstructInterpretReport
	FinalizeInterpretReport    = domainreport.FinalizeInterpretReport
	NewID                      = domainreport.NewID
	ParseID                    = domainreport.ParseID
	IsHighRisk                 = domainreport.IsHighRisk
)

// Section is a logical report section independent of assessment code.
type Section struct {
	Title   string
	Content string
	Blocks  []Block
}

// Block is a renderable report block within a section.
type Block struct {
	Kind    string
	Payload any
}
