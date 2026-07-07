// Package report 负责解释报告 aggregate 和 section structure。
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

// Section 是logical report section 独立于 测评编码。
type Section struct {
	Title   string
	Content string
	Blocks  []Block
}

// Block 是renderable report block 在 section。
type Block struct {
	Kind    string
	Payload any
}
