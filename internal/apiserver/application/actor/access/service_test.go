package access

import (
	"context"
	"testing"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainOperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

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

func (s *stubOperatorRepository) Save(ctx context.Context, staff *domainOperator.Operator) error {
	panic("unexpected call")
}
func (s *stubOperatorRepository) Update(ctx context.Context, staff *domainOperator.Operator) error {
	panic("unexpected call")
}
func (s *stubOperatorRepository) FindByID(ctx context.Context, id domainOperator.ID) (*domainOperator.Operator, error) {
	panic("unexpected call")
}
func (s *stubOperatorRepository) FindByUser(ctx context.Context, orgID int64, userID int64) (*domainOperator.Operator, error) {
	return s.item, nil
}
func (s *stubOperatorRepository) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*domainOperator.Operator, error) {
	panic("unexpected call")
}
func (s *stubOperatorRepository) ListByRole(ctx context.Context, orgID int64, role domainOperator.Role, offset, limit int) ([]*domainOperator.Operator, error) {
	panic("unexpected call")
}
func (s *stubOperatorRepository) Delete(ctx context.Context, id domainOperator.ID) error {
	panic("unexpected call")
}
func (s *stubOperatorRepository) Count(ctx context.Context, orgID int64) (int64, error) {
	panic("unexpected call")
}

type stubClinicianRepository struct {
	item *domainClinician.Clinician
}

func (s *stubClinicianRepository) Save(ctx context.Context, item *domainClinician.Clinician) error {
	panic("unexpected call")
}
func (s *stubClinicianRepository) Update(ctx context.Context, item *domainClinician.Clinician) error {
	panic("unexpected call")
}
func (s *stubClinicianRepository) FindByID(ctx context.Context, id domainClinician.ID) (*domainClinician.Clinician, error) {
	panic("unexpected call")
}
func (s *stubClinicianRepository) FindByOperator(ctx context.Context, orgID int64, operatorID uint64) (*domainClinician.Clinician, error) {
	return s.item, nil
}
func (s *stubClinicianRepository) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*domainClinician.Clinician, error) {
	panic("unexpected call")
}
func (s *stubClinicianRepository) Count(ctx context.Context, orgID int64) (int64, error) {
	panic("unexpected call")
}
func (s *stubClinicianRepository) Delete(ctx context.Context, id domainClinician.ID) error {
	panic("unexpected call")
}

type stubRelationRepository struct {
	lastRelationTypes []domainRelation.RelationType
	activeAllowed     bool
}

func (s *stubRelationRepository) Save(ctx context.Context, item *domainRelation.ClinicianTesteeRelation) error {
	panic("unexpected call")
}
func (s *stubRelationRepository) Update(ctx context.Context, item *domainRelation.ClinicianTesteeRelation) error {
	panic("unexpected call")
}
func (s *stubRelationRepository) FindByID(ctx context.Context, id domainRelation.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) FindActive(ctx context.Context, orgID int64, clinicianID domainClinician.ID, testeeID domainTestee.ID, relationType domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) FindActivePrimaryByTestee(ctx context.Context, orgID int64, testeeID domainTestee.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) FindActiveByTypes(ctx context.Context, orgID int64, clinicianID domainClinician.ID, testeeID domainTestee.ID, relationTypes []domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) ListActiveByClinician(ctx context.Context, orgID int64, clinicianID domainClinician.ID, relationTypes []domainRelation.RelationType, offset, limit int) ([]*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) ListHistoryByClinician(ctx context.Context, orgID int64, clinicianID domainClinician.ID) ([]*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) CountActiveByClinician(ctx context.Context, orgID int64, clinicianID domainClinician.ID, relationTypes []domainRelation.RelationType) (int64, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) ListActiveByTestee(ctx context.Context, orgID int64, testeeID domainTestee.ID, relationTypes []domainRelation.RelationType) ([]*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) ListHistoryByTestee(ctx context.Context, orgID int64, testeeID domainTestee.ID) ([]*domainRelation.ClinicianTesteeRelation, error) {
	panic("unexpected call")
}
func (s *stubRelationRepository) HasActiveRelationForTestee(ctx context.Context, orgID int64, clinicianID domainClinician.ID, testeeID domainTestee.ID, relationTypes []domainRelation.RelationType) (bool, error) {
	s.lastRelationTypes = append([]domainRelation.RelationType(nil), relationTypes...)
	return s.activeAllowed, nil
}
func (s *stubRelationRepository) ListActiveTesteeIDsByClinician(ctx context.Context, orgID int64, clinicianID domainClinician.ID, relationTypes []domainRelation.RelationType) ([]domainTestee.ID, error) {
	s.lastRelationTypes = append([]domainRelation.RelationType(nil), relationTypes...)
	return []domainTestee.ID{domainTestee.ID(401)}, nil
}

type stubTesteeRepository struct {
	item *domainTestee.Testee
}

func (s *stubTesteeRepository) Save(ctx context.Context, testee *domainTestee.Testee) error {
	panic("unexpected call")
}
func (s *stubTesteeRepository) Update(ctx context.Context, testee *domainTestee.Testee) error {
	panic("unexpected call")
}
func (s *stubTesteeRepository) FindByID(ctx context.Context, id domainTestee.ID) (*domainTestee.Testee, error) {
	return s.item, nil
}
func (s *stubTesteeRepository) FindByIDs(ctx context.Context, ids []domainTestee.ID) ([]*domainTestee.Testee, error) {
	if s.item == nil {
		return []*domainTestee.Testee{}, nil
	}
	return []*domainTestee.Testee{s.item}, nil
}
func (s *stubTesteeRepository) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) FindByOrgAndName(ctx context.Context, orgID int64, name string) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) ListByOrg(ctx context.Context, orgID int64, filter domainTestee.ListFilter, offset, limit int) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) ListByOrgAndIDs(ctx context.Context, orgID int64, ids []domainTestee.ID, filter domainTestee.ListFilter, offset, limit int) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) ListByTags(ctx context.Context, orgID int64, tags []string, offset, limit int) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) ListByProfileIDs(ctx context.Context, profileIDs []uint64, offset, limit int) ([]*domainTestee.Testee, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) Delete(ctx context.Context, id domainTestee.ID) error {
	panic("unexpected call")
}
func (s *stubTesteeRepository) Count(ctx context.Context, orgID int64, filter domainTestee.ListFilter) (int64, error) {
	panic("unexpected call")
}
func (s *stubTesteeRepository) CountByOrgAndIDs(ctx context.Context, orgID int64, ids []domainTestee.ID, filter domainTestee.ListFilter) (int64, error) {
	panic("unexpected call")
}
