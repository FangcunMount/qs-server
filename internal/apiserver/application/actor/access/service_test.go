package access

import (
	"context"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainOperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestResolveAccessScopeLoadsSnapshotThroughReaderWhenContextMissing(t *testing.T) {
	operatorItem := domainOperator.NewOperator(1, 101, "operator")
	operatorItem.SetID(201)
	reader := &stubAuthzSnapshotReader{snapshot: stubAuthzSnapshot{admin: true}}
	svc := NewTesteeAccessService(
		&stubOperatorRepository{item: operatorItem},
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
	operatorItem := domainOperator.NewOperator(1, 101, "operator")
	operatorItem.SetID(201)
	reader := &stubAuthzSnapshotReader{snapshot: stubAuthzSnapshot{admin: false}}
	svc := NewTesteeAccessService(
		&stubOperatorRepository{item: operatorItem},
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
	operatorItem := domainOperator.NewOperator(1, 101, "operator")
	operatorItem.SetID(201)
	svc := NewTesteeAccessService(
		&stubOperatorRepository{item: operatorItem},
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
	operatorItem := domainOperator.NewOperator(1, 101, "operator")
	operatorItem.SetID(201)

	clinicianItem := domainClinician.NewClinician(1, nil, "clinician", "", "", domainClinician.TypeCounselor, "", true)
	clinicianItem.SetID(301)

	testeeItem := domainTestee.NewTestee(1, "child", domainTestee.GenderMale, nil)
	testeeItem.SetID(401)

	relationRepo := &stubRelationRepository{activeAllowed: true}
	svc := NewTesteeAccessService(
		&stubOperatorRepository{item: operatorItem},
		&stubClinicianRepository{item: clinicianItem},
		relationRepo,
		&stubTesteeRepository{item: testeeItem},
		nil,
	)

	ctx := authzapp.WithSnapshot(context.Background(), &authzapp.Snapshot{})
	if err := svc.ValidateTesteeAccess(ctx, 1, 101, 401); err != nil {
		t.Fatalf("expected access validation to pass: %v", err)
	}

	expected := domainRelation.AccessGrantRelationTypes()
	if len(relationRepo.lastRelationTypes) != len(expected) {
		t.Fatalf("expected access validation to check %v, got %v", expected, relationRepo.lastRelationTypes)
	}
	for index := range expected {
		if relationRepo.lastRelationTypes[index] != expected[index] {
			t.Fatalf("expected access validation to check %v, got %v", expected, relationRepo.lastRelationTypes)
		}
	}
}

type stubOperatorRepository struct {
	item *domainOperator.Operator
}

func (s *stubOperatorRepository) Save(context.Context, *domainOperator.Operator) error {
	panic("unexpected call")
}
func (s *stubOperatorRepository) Update(context.Context, *domainOperator.Operator) error {
	panic("unexpected call")
}
func (s *stubOperatorRepository) FindByID(context.Context, domainOperator.ID) (*domainOperator.Operator, error) {
	panic("unexpected call")
}
func (s *stubOperatorRepository) FindByUser(context.Context, int64, int64) (*domainOperator.Operator, error) {
	return s.item, nil
}
func (s *stubOperatorRepository) ListByOrg(context.Context, int64, int, int) ([]*domainOperator.Operator, error) {
	panic("unexpected call")
}
func (s *stubOperatorRepository) ListByRole(context.Context, int64, domainOperator.Role, int, int) ([]*domainOperator.Operator, error) {
	panic("unexpected call")
}
func (s *stubOperatorRepository) Delete(context.Context, domainOperator.ID) error {
	panic("unexpected call")
}
func (s *stubOperatorRepository) Count(context.Context, int64) (int64, error) {
	panic("unexpected call")
}

type stubClinicianRepository struct {
	item *domainClinician.Clinician
}

func (s *stubClinicianRepository) Save(context.Context, *domainClinician.Clinician) error {
	panic("unexpected call")
}
func (s *stubClinicianRepository) Update(context.Context, *domainClinician.Clinician) error {
	panic("unexpected call")
}
func (s *stubClinicianRepository) FindByID(context.Context, domainClinician.ID) (*domainClinician.Clinician, error) {
	panic("unexpected call")
}
func (s *stubClinicianRepository) FindByOperator(context.Context, int64, uint64) (*domainClinician.Clinician, error) {
	return s.item, nil
}
func (s *stubClinicianRepository) ListByOrg(context.Context, int64, int, int) ([]*domainClinician.Clinician, error) {
	panic("unexpected call")
}
func (s *stubClinicianRepository) Count(context.Context, int64) (int64, error) {
	panic("unexpected call")
}
func (s *stubClinicianRepository) Delete(context.Context, domainClinician.ID) error {
	panic("unexpected call")
}

type stubRelationRepository struct {
	lastRelationTypes []domainRelation.RelationType
	activeAllowed     bool
}

func (s *stubRelationRepository) Save(context.Context, *domainRelation.ClinicianTesteeRelation) error {
	panic("unexpected call")
}
func (s *stubRelationRepository) Update(context.Context, *domainRelation.ClinicianTesteeRelation) error {
	panic("unexpected call")
}
func (s *stubRelationRepository) FindByID(context.Context, domainRelation.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) FindActive(context.Context, int64, domainClinician.ID, domainTestee.ID, domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) FindActivePrimaryByTestee(context.Context, int64, domainTestee.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) FindActiveByTypes(context.Context, int64, domainClinician.ID, domainTestee.ID, []domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) ListActiveByClinician(context.Context, int64, domainClinician.ID, []domainRelation.RelationType, int, int) ([]*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) ListHistoryByClinician(context.Context, int64, domainClinician.ID) ([]*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) CountActiveByClinician(context.Context, int64, domainClinician.ID, []domainRelation.RelationType) (int64, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) ListActiveByTestee(context.Context, int64, domainTestee.ID, []domainRelation.RelationType) ([]*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) ListHistoryByTestee(context.Context, int64, domainTestee.ID) ([]*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) HasActiveRelationForTestee(_ context.Context, _ int64, _ domainClinician.ID, _ domainTestee.ID, relationTypes []domainRelation.RelationType) (bool, error) {
	s.lastRelationTypes = append([]domainRelation.RelationType(nil), relationTypes...)
	return s.activeAllowed, nil
}
func (s *stubRelationRepository) ListActiveTesteeIDsByClinician(_ context.Context, _ int64, _ domainClinician.ID, relationTypes []domainRelation.RelationType) ([]domainTestee.ID, error) {
	s.lastRelationTypes = append([]domainRelation.RelationType(nil), relationTypes...)
	return []domainTestee.ID{domainTestee.ID(401)}, nil
}

type stubTesteeRepository struct {
	item *domainTestee.Testee
}

func (s *stubTesteeRepository) Save(context.Context, *domainTestee.Testee) error {
	panic("unexpected call")
}
func (s *stubTesteeRepository) Update(context.Context, *domainTestee.Testee) error {
	panic("unexpected call")
}
func (s *stubTesteeRepository) FindByID(context.Context, domainTestee.ID) (*domainTestee.Testee, error) {
	return s.item, nil
}
func (s *stubTesteeRepository) FindByIDs(context.Context, []domainTestee.ID) ([]*domainTestee.Testee, error) {
	if s.item == nil {
		return []*domainTestee.Testee{}, nil
	}
	return []*domainTestee.Testee{s.item}, nil
}
func (s *stubTesteeRepository) FindByProfile(context.Context, int64, uint64) (*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) FindByOrgAndName(context.Context, int64, string) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) ListByOrg(context.Context, int64, domainTestee.ListFilter, int, int) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) ListByOrgAndIDs(context.Context, int64, []domainTestee.ID, domainTestee.ListFilter, int, int) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) ListByTags(context.Context, int64, []string, int, int) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) ListKeyFocus(context.Context, int64, int, int) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) ListByProfileIDs(context.Context, []uint64, int, int) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) Delete(context.Context, domainTestee.ID) error {
	panic("unexpected call")
}
func (s *stubTesteeRepository) Count(context.Context, int64, domainTestee.ListFilter) (int64, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) CountByOrgAndIDs(context.Context, int64, []domainTestee.ID, domainTestee.ListFilter) (int64, error) {
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
