package result

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type Outcome struct {
	Assessment *assessment.Assessment
	Input      *evaluationinput.InputSnapshot
	Result     *assessment.EvaluationResult
}

type Writer interface {
	Write(ctx context.Context, outcome Outcome) error
}

type ScoreProjector interface {
	Kind() assessment.EvaluationModelKind
	Project(ctx context.Context, outcome Outcome) error
}

type ReportBuilder interface {
	Kind() assessment.EvaluationModelKind
	Build(ctx context.Context, outcome Outcome) (*domainReport.InterpretReport, error)
}
