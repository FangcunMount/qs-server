// Package modelpreview is the outbound port that the model-catalog uses to
// preview an interpretation report for a draft model. The model-catalog knows
// the model and the answers, but running an evaluation + building a report is an
// evaluation/interpretation concern. This neutral port lets the model-catalog
// depend on an abstraction instead of importing the evaluation module directly.
package modelpreview

import (
	"context"

	interpretationreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// Request carries the model facts and the execution input needed to preview a report.
type Request struct {
	SubKind              modelcatalog.SubKind
	Algorithm            modelcatalog.Algorithm
	Code                 string
	Version              string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Input                *evaluationinput.InputSnapshot
}

// Result is the projected preview outcome plus the built interpretation report.
type Result struct {
	OutcomeCode  string
	OutcomeTitle string
	Scores       map[string]float64
	Report       *interpretationreport.Draft
}

// ReportPreviewer runs an evaluation and builds an interpretation report for preview.
type ReportPreviewer interface {
	PreviewReport(ctx context.Context, req Request) (*Result, error)
}
