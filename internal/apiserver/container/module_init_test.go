package container

import (
	"context"
	"errors"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	rm "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
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
		actors: &actorAccessStub{},
	}
	decision, err := access.AuthorizeAssessment(reportScopeContext(true), interpretationadmin.Actor{OrgID: 1, OperatorUserID: 2}, 42)
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

func TestAdministrationInterpretationAccessMapsRestrictedOperatorAudience(t *testing.T) {
	access := administrationInterpretationAccess{
		access: &operatorQueryStub{},
		actors: &actorAccessStub{},
	}
	decision, err := access.AuthorizeAssessment(reportScopeContext(false), interpretationadmin.Actor{OrgID: 1, OperatorUserID: 2}, 42)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Audience != interpretationpolicy.AudienceOperator || decision.IsAdmin || !decision.Restricted {
		t.Fatalf("decision=%#v", decision)
	}

	scope, err := access.ScopeReports(reportScopeContext(false), interpretationadmin.Actor{OrgID: 1, OperatorUserID: 2}, 7)
	if err != nil {
		t.Fatal(err)
	}
	// Audience and explicit store filtering remain independent.
	if !scope.RestrictToStoreScope || scope.TesteeID != 7 || len(scope.StoreScopedTesteeIDs) != 1 {
		t.Fatalf("list scope=%#v", scope)
	}
	if scope.Audience != interpretationpolicy.AudienceOperator || scope.IsAdmin {
		t.Fatalf("audience scope=%#v", scope)
	}
}

type operatorQueryStub struct {
	resource, action string
	evaluationoperator.QueryService
	err error
}

func (s *operatorQueryStub) GetAssessment(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.Assessment, error) {
	return &evaluationoperator.Assessment{}, s.err
}

type actorAccessStub struct {
	resource, action string
	actoraccess.TesteeAccessService
	err error
}

type participantOwnershipReader struct{ rm.TesteeReader }

func (*participantOwnershipReader) GetTestee(context.Context, uint64) (*rm.TesteeRow, error) {
	return &rm.TesteeRow{ID: 7, OrgID: 1}, nil
}

func TestParticipantReportAccessNeedsNoOperatorOrStoreButRetainsOwnership(t *testing.T) {
	denied := errors.New("assessment belongs to another participant")
	assessments := &participantAssessmentAccessStub{err: denied}
	access := participantInterpretationAccess{
		testees:     actortestee.NewSelfServiceQueryServiceWithAssessmentSummary(&participantOwnershipReader{}, nil),
		assessments: assessments,
	}
	if err := access.AuthorizeParticipant(context.Background(), interpretationparticipant.Actor{TesteeID: 7}); err != nil {
		t.Fatal(err)
	}
	if err := access.AuthorizeOwnAssessment(context.Background(), 7, 42); !errors.Is(err, denied) {
		t.Fatalf("ownership check lost: %v", err)
	}
	if assessments.calls != 1 {
		t.Fatal("ownership not checked")
	}
}

func reportScopeContext(admin bool) context.Context {
	snapshot := &appauthz.Snapshot{AuthzVersion: 1, ScopeContractVersion: 1}
	if admin {
		snapshot.Permissions = []appauthz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: appauthz.AuthorizationModeUnconditional}}
	}
	return appauthz.WithSnapshot(context.Background(), snapshot)
}
func (s *operatorQueryStub) AuthorizeAssessmentResource(_ context.Context, _ evaluationoperator.Actor, _ uint64, resource, action string) error {
	s.resource = resource
	s.action = action
	return s.err
}
func (s *actorAccessStub) ValidateTesteeStoreAccess(_ context.Context, _, _ int64, _ uint64, resource, action string) error {
	s.resource = resource
	s.action = action
	return s.err
}
func (s *actorAccessStub) ListStoreScopedTesteeIDs(_ context.Context, _, _ int64, resource, action string) ([]uint64, error) {
	s.resource = resource
	s.action = action
	return []uint64{7}, s.err
}
func TestReportsAuthorizeTheirOwnResource(t *testing.T) {
	query := &operatorQueryStub{}
	actors := &actorAccessStub{}
	access := administrationInterpretationAccess{access: query, actors: actors}
	ctx := reportScopeContext(false)
	if _, err := access.AuthorizeAssessment(ctx, interpretationadmin.Actor{OrgID: 1, OperatorUserID: 2}, 42); err != nil {
		t.Fatal(err)
	}
	if query.resource != "qs:evaluation:collection:reports" || query.action != "read" {
		t.Fatal("borrowed assessment permission")
	}
	scope, err := access.ScopeReports(ctx, interpretationadmin.Actor{OrgID: 1, OperatorUserID: 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if actors.resource != "qs:evaluation:collection:reports" || actors.action != "list" || !scope.RestrictToStoreScope || len(scope.StoreScopedTesteeIDs) != 1 {
		t.Fatalf("incorrect report range: %+v", scope)
	}
}
