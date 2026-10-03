package aibridge

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"testing"
)

type participantOperationStore struct {
	readingStore
	operationReads int
	aggregate      string
}

func (s *participantOperationStore) ReadRequestOperation(_ context.Context, scope OperationScope, aggregate, id string) (MessagingOperation, error) {
	s.operationReads++
	s.aggregate = aggregate
	return MessagingOperation{OperationID: id, CommandID: id, Status: "submitted"}, nil
}
func TestMQParticipantOperationRequiresCurrentAccessAndOriginalRequest(t *testing.T) {
	for _, scenario := range []string{"authorized", "revoked", "organization", "owner", "testee", "assessment", "invalid_command"} {
		t.Run(scenario, func(t *testing.T) {
			actor := Actor{"1", "parent"}
			requestID, commandID := uuid.NewString(), uuid.NewString()
			r := &Start{RequestID: requestID, Actor: actor, TesteeID: "7", AssessmentIDs: []string{"42"}}
			s := &participantOperationStore{readingStore: readingStore{request: r}}
			var deny error
			switch scenario {
			case "revoked":
				deny = errors.New("revoked")
			case "organization":
				r.Actor.OrgID = "2"
			case "owner":
				r.Actor.SubjectID = "other"
			case "testee":
				r.TesteeID = "8"
			case "assessment":
				r.AssessmentIDs = []string{"43"}
			case "invalid_command":
				commandID = "bad"
			}
			p := &Participant{Access: accessStub{deny}, Bridge: &Service{Store: s}}
			o, err := p.ReadOperation(t.Context(), actor, 7, 42, requestID, commandID)
			if scenario == "authorized" {
				if err != nil || o.CommandID != commandID || s.aggregate != requestID || s.operationReads != 1 {
					t.Fatalf("operation=%+v err=%v", o, err)
				}
			} else if err == nil || s.operationReads != 0 {
				t.Fatal("unauthorized operation lookup")
			}
			if scenario == "revoked" && s.reads != 0 {
				t.Fatal("read before current authorization")
			}
		})
	}
}

func TestMQParticipantClosedIntakeRetainsReadAndOriginalDuplicate(t *testing.T) {
	actor := Actor{"1", "parent"}
	id := uuid.NewString()
	r := &Start{RequestID: id, Actor: actor, TesteeID: "7", AssessmentIDs: []string{"42"}, Evidence: []EvidenceItem{{ReportID: "99"}}}
	s := &participantOperationStore{readingStore: readingStore{request: r}}
	p := &Participant{IntakeClosed: true, Access: accessStub{}, Bridge: &Service{Store: s}}
	if e := p.Request(t.Context(), actor, 7, 42, 99, id); e != nil {
		t.Fatal("original duplicate denied", e)
	}
	if _, e := p.ReadOperation(t.Context(), actor, 7, 42, id, id); e != nil {
		t.Fatal("read tied to new intake", e)
	}
	empty := &stagingStore{}
	p.Bridge.Store = empty
	if e := p.Request(t.Context(), actor, 7, 42, 99, uuid.NewString()); !errors.Is(e, ErrManagementUnavailable) || len(empty.requests) != 0 {
		t.Fatal("closed intake staged new work", e)
	}
}
