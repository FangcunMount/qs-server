package access

import (
	"context"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestResolveAccessScopeLoadsSnapshotThroughReaderWhenContextMissing(t *testing.T) {
	operatorItem := actorreadmodel.OperatorRow{ID: 201, OrgID: 1, UserID: 101, Name: "operator", IsActive: true}
	reader := &stubAuthzSnapshotReader{snapshot: stubAuthzSnapshot{admin: true}}
	svc := NewTesteeAccessService(
		&stubOperatorReader{item: operatorItem},
		nil,
		nil,
		nil,
		reader,
	)

	scope, err := svc.ResolveAccessScope(context.Background(), 1, 101)
	if err != nil {
		t.Fatalf("expected access scope to resolve: %v", err)
	}
	if scope == nil || !scope.IsAdmin {
		t.Fatalf("expected admin access scope, got %#v", scope)
	}
	if reader.calls != 1 {
		t.Fatalf("expected snapshot reader to be called once, got %d", reader.calls)
	}
	if reader.orgID != 1 || reader.userID != 101 {
		t.Fatalf("expected snapshot reader args org=1 user=101, got org=%d user=%d", reader.orgID, reader.userID)
	}
}

func TestResolveAccessScopeUsesContextSnapshotBeforeReader(t *testing.T) {
	operatorItem := actorreadmodel.OperatorRow{ID: 201, OrgID: 1, UserID: 101, Name: "operator", IsActive: true}
	reader := &stubAuthzSnapshotReader{snapshot: stubAuthzSnapshot{admin: false}}
	svc := NewTesteeAccessService(
		&stubOperatorReader{item: operatorItem},
		nil,
		nil,
		nil,
		reader,
	)

	ctx := authzapp.WithSnapshot(context.Background(), &authzapp.Snapshot{Roles: []string{"qs:admin"}})
	scope, err := svc.ResolveAccessScope(ctx, 1, 101)
	if err != nil {
		t.Fatalf("expected access scope to resolve: %v", err)
	}
	if scope == nil || !scope.IsAdmin {
		t.Fatalf("expected admin access scope, got %#v", scope)
	}
	if reader.calls != 0 {
		t.Fatalf("expected context snapshot to avoid reader call, got %d calls", reader.calls)
	}
}

func TestResolveAccessScopeRejectsWhenSnapshotReaderMissing(t *testing.T) {
	operatorItem := actorreadmodel.OperatorRow{ID: 201, OrgID: 1, UserID: 101, Name: "operator", IsActive: true}
	svc := NewTesteeAccessService(
		&stubOperatorReader{item: operatorItem},
		nil,
		nil,
		nil,
		nil,
	)

	_, err := svc.ResolveAccessScope(context.Background(), 1, 101)
	if err == nil {
		t.Fatal("expected missing snapshot reader to reject access")
	}
	if !cberrors.IsCode(err, code.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestValidateTesteeAccessUsesAccessGrantRelations(t *testing.T) {
	operatorItem := actorreadmodel.OperatorRow{ID: 201, OrgID: 1, UserID: 101, Name: "operator", IsActive: true}
	clinicianItem := actorreadmodel.ClinicianRow{ID: 301, OrgID: 1, Name: "clinician", IsActive: true}
	testeeItem := actorreadmodel.TesteeRow{ID: 401, OrgID: 1, Name: "child"}

	relationRepo := &stubRelationReader{activeAllowed: true}
	svc := NewTesteeAccessService(
		&stubOperatorReader{item: operatorItem},
		&stubClinicianReader{item: clinicianItem},
		relationRepo,
		&stubTesteeReader{item: testeeItem},
		nil,
	)

	ctx := authzapp.WithSnapshot(context.Background(), &authzapp.Snapshot{})
	if err := svc.ValidateTesteeAccess(ctx, 1, 101, 401); err != nil {
		t.Fatalf("expected access validation to pass: %v", err)
	}

	expected := accessRelationTypesToStrings(domainRelation.AccessGrantRelationTypes())
	if len(relationRepo.lastRelationTypes) != len(expected) {
		t.Fatalf("expected access validation to check %v, got %v", expected, relationRepo.lastRelationTypes)
	}
	for index := range expected {
		if relationRepo.lastRelationTypes[index] != expected[index] {
			t.Fatalf("expected access validation to check %v, got %v", expected, relationRepo.lastRelationTypes)
		}
	}
}

type stubOperatorReader struct {
	item actorreadmodel.OperatorRow
}

func (s *stubOperatorReader) GetOperator(context.Context, uint64) (*actorreadmodel.OperatorRow, error) {
	panic("unexpected call")
}
func (s *stubOperatorReader) FindOperatorByUser(context.Context, int64, int64) (*actorreadmodel.OperatorRow, error) {
	return &s.item, nil
}
func (s *stubOperatorReader) ListOperators(context.Context, actorreadmodel.OperatorFilter) ([]actorreadmodel.OperatorRow, error) {
	panic("unexpected call")
}
func (s *stubOperatorReader) CountOperators(context.Context, int64) (int64, error) {
	panic("unexpected call")
}

type stubClinicianReader struct {
	item actorreadmodel.ClinicianRow
}

func (s *stubClinicianReader) GetClinician(context.Context, uint64) (*actorreadmodel.ClinicianRow, error) {
	panic("unexpected call")
}
func (s *stubClinicianReader) FindClinicianByOperator(context.Context, int64, uint64) (*actorreadmodel.ClinicianRow, error) {
	return &s.item, nil
}
func (s *stubClinicianReader) ListClinicians(context.Context, actorreadmodel.ClinicianFilter) ([]actorreadmodel.ClinicianRow, error) {
	panic("unexpected call")
}
func (s *stubClinicianReader) CountClinicians(context.Context, int64) (int64, error) {
	panic("unexpected call")
}

type stubRelationReader struct {
	lastRelationTypes []string
	activeAllowed     bool
}

func (s *stubRelationReader) ListAssignedTestees(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.TesteeRow, int64, error) {
	panic("unexpected call")
}
func (s *stubRelationReader) ListActiveTesteeIDsByClinician(_ context.Context, _ int64, _ uint64, relationTypes []string) ([]uint64, error) {
	s.lastRelationTypes = append([]string(nil), relationTypes...)
	return []uint64{401}, nil
}
func (s *stubRelationReader) ListActiveTesteeRelationsByTesteeIDs(context.Context, int64, []uint64, []string) ([]actorreadmodel.TesteeRelationRow, error) {
	panic("unexpected call")
}
func (s *stubRelationReader) ListTesteeRelations(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.TesteeRelationRow, error) {
	panic("unexpected call")
}
func (s *stubRelationReader) ListClinicianRelations(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.ClinicianRelationRow, int64, error) {
	panic("unexpected call")
}
func (s *stubRelationReader) HasActiveRelationForTestee(_ context.Context, _ int64, _, _ uint64, relationTypes []string) (bool, error) {
	s.lastRelationTypes = append([]string(nil), relationTypes...)
	return s.activeAllowed, nil
}

type stubTesteeReader struct {
	item actorreadmodel.TesteeRow
}

func (s *stubTesteeReader) GetTestee(context.Context, uint64) (*actorreadmodel.TesteeRow, error) {
	return &s.item, nil
}
func (s *stubTesteeReader) FindTesteeByProfile(context.Context, int64, uint64) (*actorreadmodel.TesteeRow, error) {
	panic("unexpected call")
}
func (s *stubTesteeReader) ListTestees(context.Context, actorreadmodel.TesteeFilter) ([]actorreadmodel.TesteeRow, error) {
	panic("unexpected call")
}
func (s *stubTesteeReader) CountTestees(context.Context, actorreadmodel.TesteeFilter) (int64, error) {
	panic("unexpected call")
}
func (s *stubTesteeReader) ListTesteesByProfileIDs(context.Context, []uint64, int, int) ([]actorreadmodel.TesteeRow, error) {
	panic("unexpected call")
}
func (s *stubTesteeReader) CountTesteesByProfileIDs(context.Context, []uint64) (int64, error) {
	panic("unexpected call")
}

type stubAuthzSnapshot struct {
	admin bool
}

func (s stubAuthzSnapshot) IsQSAdmin() bool {
	return s.admin
}

type stubAuthzSnapshotReader struct {
	snapshot iambridge.AuthzSnapshot
	err      error
	calls    int
	orgID    int64
	userID   int64
}

func (s *stubAuthzSnapshotReader) LoadAuthzSnapshot(_ context.Context, orgID, userID int64) (iambridge.AuthzSnapshot, error) {
	s.calls++
	s.orgID = orgID
	s.userID = userID
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}
