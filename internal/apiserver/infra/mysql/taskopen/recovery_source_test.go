package taskopen

import (
	"encoding/json"
	"testing"
	"time"

	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type recoveryRoutes struct{}

func (recoveryRoutes) GetTopicForEvent(string) (string, bool) { return "qs.plan.task", true }

func recoveryPlanFixture(t *testing.T) transaction.FrozenPlan {
	t.Helper()
	q, err := freeze(openedTask(t), 1)
	require.NoError(t, err)
	plan, err := shared.Freeze(q, recoveryRoutes{}, "qs.mysql", "m3-1")
	require.NoError(t, err)
	return plan
}

func TestRecoveryOpeningUsesOriginalPrivateArtifactAndWire(t *testing.T) {
	plan := recoveryPlanFixture(t)
	o, err := readRecoveryOpening(plan)
	require.NoError(t, err)
	require.Equal(t, plan.Input().Branches[0].Message.Input().ID, o.Request.OpeningEventID)
	require.Equal(t, o.Task.ID.String(), o.Request.Intent.TaskID)
	require.True(t, o.MatchesCurrent(&o.Task))
	encoded, err := json.Marshal(o)
	require.NoError(t, err)
	require.JSONEq(t, "{}", string(encoded), "private authority and entry URL must not be serializable")
	changed := o.Task
	changed.EntryURL = "another-entry"
	require.False(t, o.MatchesCurrent(&changed))
	changed = o.Task
	changed.ScheduleRevision++
	require.False(t, o.MatchesCurrent(&changed))
	for _, field := range []string{"planned", "scale", "enrollment", "sequence", "due", "defined"} {
		t.Run(field, func(t *testing.T) {
			changed := o.Task
			clock := changed.PlannedAt.Add(time.Second)
			switch field {
			case "planned":
				changed.PlannedAt = clock
			case "scale":
				changed.ScaleCode = "other-scale"
			case "enrollment":
				changed.EnrollmentID++
			case "sequence":
				changed.Seq++
			case "due":
				changed.DueAt = &clock
			case "defined":
				changed.ScheduleDefinedAt = &clock
			}
			require.False(t, o.MatchesCurrent(&changed))
		})
	}
}

func TestRecoveryOpeningRejectsChangedRequestOrWire(t *testing.T) {
	for _, name := range []string{"input_hash", "request_key", "scope", "wire"} {
		t.Run(name, func(t *testing.T) {
			in := recoveryPlanFixture(t).Input()
			switch name {
			case "input_hash":
				in.InputHash[0] ^= 1
			case "request_key":
				in.IdempotencyKey = "other-opening"
			case "scope":
				in.Scope = "org:99"
			case "wire":
				in.Branches[0].Wire = []byte(`{}`)
			}
			plan, err := transaction.NewPlan(in)
			if err != nil {
				return
			}
			_, err = readRecoveryOpening(plan)
			require.ErrorIs(t, err, ErrUnsafeRecoveryOpening)
		})
	}
}
