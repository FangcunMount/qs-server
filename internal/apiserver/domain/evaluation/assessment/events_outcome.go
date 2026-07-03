package assessment

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const EventTypeInterpretedOutcome = EventTypeInterpreted

// AssessmentInterpretedOutcomeData is the outcome-enriched interpreted event payload.
type AssessmentInterpretedOutcomeData = eventoutcome.AssessmentInterpretedPayload

type AssessmentInterpretedOutcomeEvent = event.Event[AssessmentInterpretedOutcomeData]

// NewAssessmentInterpretedOutcomeEvent creates an outcome-enriched interpreted event.
func NewAssessmentInterpretedOutcomeEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	model EventModelIdentity,
	primary *EventScoreValue,
	level *EventResultLevel,
	interpretedAt time.Time,
) AssessmentInterpretedOutcomeEvent {
	return event.New(EventTypeInterpretedOutcome, AggregateType, strconv.FormatInt(int64(assessmentID), 10),
		AssessmentInterpretedOutcomeData{
			OrgID:         orgID,
			AssessmentID:  int64(assessmentID),
			TesteeID:      testeeID.Uint64(),
			Model:         model,
			PrimaryScore:  primary,
			Level:         level,
			InterpretedAt: interpretedAt,
		},
	)
}
