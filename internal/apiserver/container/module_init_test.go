package container

import (
	"context"
	"errors"
	"testing"

	actortestee "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	interpretationparticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
)

type participantTesteeQueryStub struct {
	actortestee.TesteeQueryService
	result *actortestee.TesteeResult
	err    error
	calls  int
}

func (s *participantTesteeQueryStub) GetByID(context.Context, uint64) (*actortestee.TesteeResult, error) {
	s.calls++
	return s.result, s.err
}

type participantAssessmentAccessStub struct {
	evaluationtestee.Service
	err   error
	calls int
}

func (s *participantAssessmentAccessStub) AuthorizeAssessment(context.Context, evaluationtestee.Actor, uint64) error {
	s.calls++
	return s.err
}

func TestParticipantInterpretationAccessRejectsMissingTesteeBeforeAssessment(t *testing.T) {
	testees := &participantTesteeQueryStub{}
	assessments := &participantAssessmentAccessStub{}
	access := participantInterpretationAccess{testees: testees, assessments: assessments}
	if err := access.AuthorizeOwnAssessment(context.Background(), 7, 42); err == nil {
		t.Fatal("missing testee was authorized")
	}
	if assessments.calls != 0 {
		t.Fatal("assessment ownership checked before participant existence")
	}
}

func TestParticipantInterpretationAccessChecksParticipantThenAssessment(t *testing.T) {
	denied := errors.New("assessment denied")
	testees := &participantTesteeQueryStub{result: &actortestee.TesteeResult{ID: 7}}
	assessments := &participantAssessmentAccessStub{err: denied}
	access := participantInterpretationAccess{testees: testees, assessments: assessments}
	if err := access.AuthorizeParticipant(context.Background(), interpretationparticipant.Actor{TesteeID: 7}); err != nil {
		t.Fatal(err)
	}
	if err := access.AuthorizeOwnAssessment(context.Background(), 7, 42); !errors.Is(err, denied) {
		t.Fatalf("assessment authorization error = %v", err)
	}
	if testees.calls != 2 || assessments.calls != 1 {
		t.Fatalf("testee calls=%d assessment calls=%d", testees.calls, assessments.calls)
	}
}
