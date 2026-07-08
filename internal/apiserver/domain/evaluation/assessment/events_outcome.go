package assessment

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	evaldomainevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const EventTypeInterpretedOutcome = evaldomainevent.TypeInterpretedOutcome

type AssessmentInterpretedOutcomeData = eventoutcome.AssessmentInterpretedPayload
type AssessmentInterpretedOutcomeEvent = event.Event[AssessmentInterpretedOutcomeData]

// NewAssessmentInterpretedOutcomeEvent 创建结果-enriched interpreted event。
func NewAssessmentInterpretedOutcomeEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	model EventModelIdentity,
	primary *EventScoreValue,
	level *EventResultLevel,
	interpretedAt time.Time,
) AssessmentInterpretedOutcomeEvent {
	return evaldomainevent.NewInterpretedOutcomeEvent(
		orgID,
		int64(assessmentID),
		testeeID.Uint64(),
		model,
		primary,
		level,
		interpretedAt,
	)
}
