package assessment

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const EventTypeInterpretedV2 = eventcatalog.AssessmentInterpretedV2

// AssessmentInterpretedV2Data is the v2 interpreted event payload.
type AssessmentInterpretedV2Data struct {
	OrgID         int64              `json:"org_id"`
	AssessmentID  int64              `json:"assessment_id"`
	TesteeID      uint64             `json:"testee_id"`
	Model         EventModelIdentity `json:"model"`
	PrimaryScore  *EventScoreValue   `json:"primary_score,omitempty"`
	Level         *EventResultLevel  `json:"level,omitempty"`
	InterpretedAt time.Time          `json:"interpreted_at"`
}

// IsHighRisk reports whether the outcome should trigger high-risk workflows.
func (d AssessmentInterpretedV2Data) IsHighRisk() bool {
	if d.Level != nil && isHighEventSeverity(d.Level.Severity) {
		return true
	}
	if d.Level != nil && isRiskLevelEventCode(d.Level.Code) {
		return IsHighRisk(RiskLevel(d.Level.Code))
	}
	return false
}

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
