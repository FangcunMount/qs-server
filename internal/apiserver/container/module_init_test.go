package container

import (
	"context"
	"errors"
	"testing"

	actoraccess "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	actortestee "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	interpretationadmin "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/administration"
	interpretationparticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	interpretationpolicy "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
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

func TestAdministrationInterpretationAccessMapsAdminAudience(t *testing.T) {
	access := administrationInterpretationAccess{
		access: &operatorQueryStub{},
		actors: &actorAccessStub{scope: &actoraccess.TesteeAccessScope{IsAdmin: true}},
	}
	decision, err := access.AuthorizeAssessment(context.Background(), interpretationadmin.Actor{OrgID: 1, OperatorUserID: 2}, 42)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Audience != interpretationpolicy.AudienceAdmin || !decision.IsAdmin || decision.Restricted {
		t.Fatalf("decision=%#v", decision)
	}
	if decision.DecisionSource != administrationDecisionSource {
		t.Fatalf("source=%q", decision.DecisionSource)
	}
}

func TestAdministrationInterpretationAccessMapsRestrictedClinicianAudience(t *testing.T) {
	access := administrationInterpretationAccess{
		access: &operatorQueryStub{listScope: evaluationoperator.TesteeListScope{TesteeID: 7}},
		actors: &actorAccessStub{scope: &actoraccess.TesteeAccessScope{IsAdmin: false}},
	}
	decision, err := access.AuthorizeAssessment(context.Background(), interpretationadmin.Actor{OrgID: 1, OperatorUserID: 2}, 42)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Audience != interpretationpolicy.AudienceClinician || decision.IsAdmin || !decision.Restricted {
		t.Fatalf("decision=%#v", decision)
	}

	scope, err := access.ScopeReports(context.Background(), interpretationadmin.Actor{OrgID: 1, OperatorUserID: 2}, 7)
	if err != nil {
		t.Fatal(err)
	}
	// Specific TesteeID lists are not "Restricted" for filtering, but Audience must still be clinician.
	if scope.Restricted || scope.TesteeID != 7 {
		t.Fatalf("list scope=%#v", scope)
	}
	if scope.Audience != interpretationpolicy.AudienceClinician || scope.IsAdmin {
		t.Fatalf("audience scope=%#v", scope)
	}
}

type operatorQueryStub struct {
	evaluationoperator.QueryService
	listScope evaluationoperator.TesteeListScope
	err       error
}

func (s *operatorQueryStub) GetAssessment(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.Assessment, error) {
	return &evaluationoperator.Assessment{}, s.err
}

func (s *operatorQueryStub) ScopeTesteeList(context.Context, evaluationoperator.Actor, uint64) (evaluationoperator.TesteeListScope, error) {
	return s.listScope, s.err
}

type actorAccessStub struct {
	actoraccess.TesteeAccessService
	scope *actoraccess.TesteeAccessScope
	err   error
}

func (s *actorAccessStub) ResolveAccessScope(context.Context, int64, int64) (*actoraccess.TesteeAccessScope, error) {
	return s.scope, s.err
}
