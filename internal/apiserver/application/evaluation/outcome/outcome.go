package outcome

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// Outcome is the canonical evaluation write model passed from scoring to interpretation.
type Outcome struct {
	Assessment           *assessment.Assessment
	Input                *evaluationinput.InputSnapshot
	Execution            *assessment.AssessmentOutcome
	RuntimeDescriptorKey evalpipeline.RuntimeDescriptorKey
}
