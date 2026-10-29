package taskopen

import (
	"encoding/json"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	"testing"
	"time"

	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
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
	o, err := readRecoveryOpening(plan, boundary.Artifact)
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
			_, err = readRecoveryOpening(plan, boundary.Artifact)
			require.ErrorIs(t, err, ErrUnsafeRecoveryOpening)
		})
	}
}

// Match the same manifest-aware verifier used by the production owner. Legacy
// evidence is readable only when that exact physical route is registered.
func TestRecoveryOpeningUsesOwnerRoutesWithoutRewritingOriginalPlan(t *testing.T) {
	const topic, owner, revision = "qs.plan.task", "qs.mysql", "m3-1"
	physical, err := transport.PhysicalTopicV3(topic, transport.TransactionMessageType, owner, revision)
	require.NoError(t, err)
	active := transport.RouteBinding{Topic: topic, PhysicalTopic: physical, MessageType: transport.TransactionMessageType, Owner: owner, RouteRevision: revision, NamingScheme: transport.ReadableNamingV1}
	old := active
	old.NamingScheme = ""
	old.PhysicalTopic, err = transport.PhysicalTopicV2(topic, transport.TransactionMessageType, owner, revision)
	require.NoError(t, err)
	routes, err := transport.NewRouteResolver([]transport.RouteBinding{active}, []transport.RouteBinding{old})
	require.NoError(t, err)
	q, err := freeze(openedTask(t), 1)
	require.NoError(t, err)
	current, err := shared.FreezeWithRoutes(q, recoveryRoutes{}, owner, revision, routes)
	require.NoError(t, err)
	historical, err := shared.Freeze(q, recoveryRoutes{}, owner, revision)
	require.NoError(t, err)
	verify := func(p transaction.FrozenPlan) ([]byte, error) {
		return shared.ArtifactWithRoutes(p, owner, revision, routes)
	}
	for _, plan := range []transaction.FrozenPlan{current, historical} {
		before, err := transaction.EncodePlan(plan)
		require.NoError(t, err)
		opening, err := readRecoveryOpening(plan, verify)
		require.NoError(t, err)
		require.True(t, opening.MatchesCurrent(&opening.Task))
		after, err := transaction.EncodePlan(plan)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
	_, err = readRecoveryOpening(current, boundary.Artifact)
	require.ErrorIs(t, err, ErrUnsafeRecoveryOpening, "legacy-only verifier must not accept current routes")
	_, err = readRecoveryOpening(current, nil)
	require.ErrorIs(t, err, ErrUnsafeRecoveryOpening)
	activeOnly, err := transport.NewRouteResolver([]transport.RouteBinding{active}, nil)
	require.NoError(t, err)
	_, err = readRecoveryOpening(historical, func(p transaction.FrozenPlan) ([]byte, error) {
		return shared.ArtifactWithRoutes(p, owner, revision, activeOnly)
	})
	require.ErrorIs(t, err, ErrUnsafeRecoveryOpening, "unregistered history must not authorize recovery")
	changed := current.Input()
	changed.Branches[0].PhysicalTopic = "other"
	wrong, err := transaction.NewPlan(changed)
	require.NoError(t, err)
	_, err = readRecoveryOpening(wrong, verify)
	require.ErrorIs(t, err, ErrUnsafeRecoveryOpening)
}
