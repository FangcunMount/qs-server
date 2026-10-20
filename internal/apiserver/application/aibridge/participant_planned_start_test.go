package aibridge

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type plannedParticipantStore struct {
	Store
	plan                      *Start
	planReads, operationReads int
	staged                    []Start
	stageError                error
}

func (*plannedParticipantStore) Original(context.Context, string) (*Start, error) {
	return nil, ErrNotFound
}
func (s *plannedParticipantStore) ReadPlannedStart(context.Context, Actor, string) (*Start, bool, error) {
	s.planReads++
	return s.plan, s.plan != nil, nil
}
func (s *plannedParticipantStore) ReadRequestOperation(_ context.Context, _ OperationScope, _, id string) (MessagingOperation, error) {
	s.operationReads++
	return MessagingOperation{OperationID: id, CommandID: id, Status: "pending", TransportStatus: "transaction_pending"}, nil
}
func (s *plannedParticipantStore) StageStart(_ context.Context, r Start) error {
	s.staged = append(s.staged, r)
	return s.stageError
}

func plannedParticipantFixture() (*Participant, *plannedParticipantStore, Actor, string) {
	actor, id := Actor{OrgID: "1", SubjectID: "parent"}, uuid.NewString()
	s := &plannedParticipantStore{plan: &Start{RequestID: id, Actor: actor, TesteeID: "7", AssessmentIDs: []string{"42"}, Goal: "first frozen goal", Evidence: []EvidenceItem{{AssessmentID: "42", TesteeID: "7", ReportID: "99", SourceVersion: "first", Facts: []Fact{{Ref: "report", Value: "first frozen snapshot"}}}}}}
	p := &Participant{IntakeClosed: true, Access: accessStub{}, Bridge: &Service{Store: s}}
	return p, s, actor, id
}

func TestParticipantPendingStartRemainsAnOperationNotAcceptedBusiness(t *testing.T) {
	p, store, actor, id := plannedParticipantFixture()
	operation, err := p.ReadOperation(t.Context(), actor, 7, 42, id, id)
	require.NoError(t, err)
	require.Equal(t, "pending", operation.Status)
	require.Equal(t, "transaction_pending", operation.TransportStatus)
	require.Equal(t, 1, store.planReads)
	require.Equal(t, 1, store.operationReads)
	_, err = p.Read(t.Context(), actor, 7, 42, id)
	require.ErrorIs(t, err, ErrNotFound)
	require.Equal(t, 1, store.planReads, "business reads must not expose a Prepared plan as a request")
}

func TestParticipantRetriesFrozenStartDuringClosedIntakeWithoutSourceReload(t *testing.T) {
	p, store, actor, id := plannedParticipantFixture()
	unknown := errors.New("original transaction is pending")
	store.stageError = unknown
	// Sources is intentionally absent. A same-operation retry must read its
	// frozen input and retain the original result, including while intake closes.
	err := p.Request(t.Context(), actor, 7, 42, 99, id)
	require.ErrorIs(t, err, unknown)
	require.Len(t, store.staged, 1)
	require.Equal(t, *store.plan, store.staged[0])
}

func TestParticipantPlannedOperationKeepsCurrentAccessAndOriginalResourceScope(t *testing.T) {
	for _, name := range []string{"revoked", "revoked-not-found", "organization", "subject", "testee", "assessment", "request"} {
		t.Run(name, func(t *testing.T) {
			p, store, actor, id := plannedParticipantFixture()
			switch name {
			case "revoked":
				p.Access = accessStub{errors.New("revoked")}
			case "revoked-not-found":
				p.Access = accessStub{ErrNotFound}
			case "organization":
				store.plan.Actor.OrgID = "2"
			case "subject":
				store.plan.Actor.SubjectID = "other"
			case "testee":
				store.plan.TesteeID = "8"
			case "assessment":
				store.plan.AssessmentIDs = []string{"43"}
			case "request":
				store.plan.RequestID = uuid.NewString()
			}
			_, err := p.ReadOperation(t.Context(), actor, 7, 42, id, id)
			require.Error(t, err)
			require.Zero(t, store.operationReads)
			if name == "revoked" || name == "revoked-not-found" {
				require.Zero(t, store.planReads)
			}
		})
	}
}

func TestParticipantFrozenRetryRejectsChangedReportAndResourceWithoutStaging(t *testing.T) {
	for _, name := range []string{"report", "testee", "assessment", "actor"} {
		t.Run(name, func(t *testing.T) {
			p, store, actor, id := plannedParticipantFixture()
			testee, assessment, report := uint64(7), uint64(42), uint64(99)
			switch name {
			case "report":
				report++
			case "testee":
				testee++
			case "assessment":
				assessment++
			case "actor":
				actor.SubjectID = "another"
			}
			require.ErrorIs(t, p.Request(t.Context(), actor, testee, assessment, report, id), ErrConflict)
			require.Empty(t, store.staged)
		})
	}
}
