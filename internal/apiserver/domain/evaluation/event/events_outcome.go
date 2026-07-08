package event

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const TypeInterpretedOutcome = TypeInterpreted

// InterpretedOutcomeData 是结果-enriched interpreted 事件载荷。
type InterpretedOutcomeData = eventoutcome.AssessmentInterpretedPayload

// InterpretedOutcomeEvent 测评已解读（含结果投影）事件
type InterpretedOutcomeEvent = event.Event[InterpretedOutcomeData]

// NewInterpretedOutcomeEvent 创建结果-enriched interpreted event。
func NewInterpretedOutcomeEvent(
	orgID int64,
	assessmentID int64,
	testeeID uint64,
	model ModelIdentity,
	primary *ScoreValue,
	level *ResultLevel,
	interpretedAt time.Time,
) InterpretedOutcomeEvent {
	return event.New(TypeInterpretedOutcome, AggregateType, strconv.FormatInt(assessmentID, 10),
		InterpretedOutcomeData{
			OrgID:         orgID,
			AssessmentID:  assessmentID,
			TesteeID:      testeeID,
			Model:         model,
			PrimaryScore:  primary,
			Level:         level,
			InterpretedAt: interpretedAt,
		},
	)
}
