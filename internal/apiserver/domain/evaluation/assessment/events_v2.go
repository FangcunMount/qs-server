package assessment

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const EventTypeInterpretedV2 = eventcatalog.AssessmentInterpretedV2

// AssessmentInterpretedV2Data is the v2 interpreted event payload.
type AssessmentInterpretedV2Data = eventoutcome.AssessmentInterpretedPayload

type AssessmentInterpretedV2Event = event.Event[AssessmentInterpretedV2Data]

// NewAssessmentInterpretedV2Event creates a v2 interpreted event.
func NewAssessmentInterpretedV2Event(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	model EventModelIdentity,
	primary *EventScoreValue,
	level *EventResultLevel,
	interpretedAt time.Time,
) AssessmentInterpretedV2Event {
	return event.New(EventTypeInterpretedV2, AggregateType, strconv.FormatInt(int64(assessmentID), 10),
		AssessmentInterpretedV2Data{
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
