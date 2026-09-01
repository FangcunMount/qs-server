package evaluation

import (
	"context"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type OnlineStepV2Status string

const (
	OnlineStepV2Progressed       OnlineStepV2Status = "progressed"
	OnlineStepV2AlreadyCompleted OnlineStepV2Status = "already_completed"
	OnlineStepV2AwaitingReview   OnlineStepV2Status = "awaiting_review"
	OnlineStepV2Blocked          OnlineStepV2Status = "blocked"
	OnlineStepV2Canceled         OnlineStepV2Status = "canceled"
)

// OnlineStepV2Command carries the exact durable address selected by the
// aggregate. The Runner must verify it against NextAction before any Provider
// call; callers cannot select a different Slot or execution ordinal.
type OnlineStepV2Command struct {
	RunID            meta.ID
	ExecutionKind    domainevaluation.EvidenceExecutionKind
	CaseID           string
	SlotOrdinal      int
	CandidateID      string
	ExecutionOrdinal int
	Owner            string
	RequestedOrgID   int64
	RequestedBy      string
}

func (c OnlineStepV2Command) Action() domainevaluation.EvidenceNextAction {
	kind := domainevaluation.EvidenceNextActionGeneration
	if c.ExecutionKind == domainevaluation.EvidenceExecutionSemantic {
		kind = domainevaluation.EvidenceNextActionSemantic
	}
	return domainevaluation.EvidenceNextAction{
		Kind: kind, CaseID: c.CaseID, SlotOrdinal: c.SlotOrdinal,
		CandidateID: c.CandidateID, ExecutionOrdinal: c.ExecutionOrdinal,
	}
}

type OnlineStepV2Result struct {
	Status   OnlineStepV2Status
	Evidence *domainevaluation.PromptEvaluationEvidenceV2
}

type OnlineStepV2Runner interface {
	RunStepV2(context.Context, OnlineStepV2Command) (*OnlineStepV2Result, error)
}
