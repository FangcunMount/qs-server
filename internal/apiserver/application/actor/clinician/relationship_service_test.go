package clinician

import (
	"context"
	"testing"
	"time"

	cbErrors "github.com/FangcunMount/component-base/pkg/errors"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestAssignTesteeNormalizesAssignedToAttending(t *testing.T) {
	relationRepo := &relationshipServiceRelationRepo{}
	svc := &relationshipService{
		relationRepo:  relationRepo,
		clinicianRepo: &relationshipServiceClinicianRepo{item: makeActiveClinician(10)},
		testeeRepo:    &relationshipServiceTesteeRepo{item: makeTestee(20)},
	}

	result, err := svc.AssignTestee(context.Background(), AssignTesteeDTO{
		OrgID:        1,
		ClinicianID:  10,
		TesteeID:     20,
		RelationType: string(domainRelation.RelationTypeAssigned),
	})
	if err != nil {
		t.Fatalf("expected assign testee to succeed: %v", err)
	}
	if result.RelationType != string(domainRelation.RelationTypeAttending) {
		t.Fatalf("expected assigned to normalize to attending, got %s", result.RelationType)
	}
	if relationRepo.saved == nil || relationRepo.saved.RelationType() != domainRelation.RelationTypeAttending {
		t.Fatalf("expected saved relation type to be attending")
	}
}

func TestTransferPrimaryUnbindsExistingPrimary(t *testing.T) {
	existingPrimary := domainRelation.NewClinicianTesteeRelation(
		1,
		domainClinician.ID(10),
		domainTestee.ID(20),
		domainRelation.RelationTypePrimary,
		domainRelation.SourceTypeManual,
		nil,
		true,
		time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
		nil,
	)
	existingPrimary.SetID(domainRelation.ID(9001))

	relationRepo := &relationshipServiceRelationRepo{
		activePrimaryByTestee: existingPrimary,
	}
	svc := &relationshipService{
		relationRepo:  relationRepo,
		clinicianRepo: &relationshipServiceClinicianRepo{item: makeActiveClinician(11)},
		testeeRepo:    &relationshipServiceTesteeRepo{item: makeTestee(20)},
	}

	result, err := svc.TransferPrimary(context.Background(), TransferPrimaryDTO{
		OrgID:         1,
		ToClinicianID: 11,
		TesteeID:      20,
	})
	if err != nil {
		t.Fatalf("expected transfer primary to succeed: %v", err)
	}
	if relationRepo.updated == nil || relationRepo.updated.IsActive() {
		t.Fatalf("expected existing primary relation to be unbound")
	}
	if relationRepo.saved == nil || relationRepo.saved.RelationType() != domainRelation.RelationTypePrimary {
		t.Fatalf("expected new primary relation to be saved")
	}
	if relationRepo.saved.SourceType() != domainRelation.SourceTypeTransfer {
		t.Fatalf("expected default transfer source type, got %s", relationRepo.saved.SourceType())
	}
	if result.ClinicianID != 11 {
		t.Fatalf("expected transferred primary to target clinician 11, got %d", result.ClinicianID)
	}
}

func makeActiveClinician(id uint64) *domainClinician.Clinician {
	item := domainClinician.NewClinician(1, nil, "clinician", "", "", domainClinician.TypeCounselor, "", true)
	item.SetID(domainClinician.ID(id))
	return item
}

func makeTestee(id uint64) *domainTestee.Testee {
	item := domainTestee.NewTestee(1, "testee", domainTestee.GenderMale, nil)
	item.SetID(domainTestee.ID(id))
	return item
}

type relationshipServiceRelationRepo struct {
	saved                 *domainRelation.ClinicianTesteeRelation
	updated               *domainRelation.ClinicianTesteeRelation
	activePrimaryByTestee *domainRelation.ClinicianTesteeRelation
}

func (s *relationshipServiceRelationRepo) Save(ctx context.Context, item *domainRelation.ClinicianTesteeRelation) error {
	s.saved = item
	return nil
}

func (s *relationshipServiceRelationRepo) Update(ctx context.Context, item *domainRelation.ClinicianTesteeRelation) error {
	s.updated = item
	return nil
}

func (s *relationshipServiceRelationRepo) FindByID(ctx context.Context, id domainRelation.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "relation not found")
}

func (s *relationshipServiceRelationRepo) FindActive(ctx context.Context, orgID int64, clinicianID domainClinician.ID, testeeID domainTestee.ID, relationType domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "relation not found")
}

func (s *relationshipServiceRelationRepo) FindActivePrimaryByTestee(ctx context.Context, orgID int64, testeeID domainTestee.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	if s.activePrimaryByTestee == nil {
		return nil, cbErrors.WithCode(code.ErrUserNotFound, "primary relation not found")
	}
	return s.activePrimaryByTestee, nil
}

func (s *relationshipServiceRelationRepo) FindActiveByTypes(ctx context.Context, orgID int64, clinicianID domainClinician.ID, testeeID domainTestee.ID, relationTypes []domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "relation not found")
}

func (s *relationshipServiceRelationRepo) ListActiveByClinician(ctx context.Context, orgID int64, clinicianID domainClinician.ID, relationTypes []domainRelation.RelationType, offset, limit int) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return nil, nil
}

func (s *relationshipServiceRelationRepo) ListHistoryByClinician(ctx context.Context, orgID int64, clinicianID domainClinician.ID) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return nil, nil
}

func (s *relationshipServiceRelationRepo) CountActiveByClinician(ctx context.Context, orgID int64, clinicianID domainClinician.ID, relationTypes []domainRelation.RelationType) (int64, error) {
	return 0, nil
}

func (s *relationshipServiceRelationRepo) ListActiveByTestee(ctx context.Context, orgID int64, testeeID domainTestee.ID, relationTypes []domainRelation.RelationType) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return nil, nil
}

func (s *relationshipServiceRelationRepo) ListHistoryByTestee(ctx context.Context, orgID int64, testeeID domainTestee.ID) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return nil, nil
}

func (s *relationshipServiceRelationRepo) HasActiveRelationForTestee(ctx context.Context, orgID int64, clinicianID domainClinician.ID, testeeID domainTestee.ID, relationTypes []domainRelation.RelationType) (bool, error) {
	return false, nil
}

func (s *relationshipServiceRelationRepo) ListActiveTesteeIDsByClinician(ctx context.Context, orgID int64, clinicianID domainClinician.ID, relationTypes []domainRelation.RelationType) ([]domainTestee.ID, error) {
	return nil, nil
}

type relationshipServiceClinicianRepo struct {
	item *domainClinician.Clinician
}

func (s *relationshipServiceClinicianRepo) Save(ctx context.Context, item *domainClinician.Clinician) error {
	return nil
}

func (s *relationshipServiceClinicianRepo) Update(ctx context.Context, item *domainClinician.Clinician) error {
	return nil
}

func (s *relationshipServiceClinicianRepo) FindByID(ctx context.Context, id domainClinician.ID) (*domainClinician.Clinician, error) {
	return s.item, nil
}

func (s *relationshipServiceClinicianRepo) FindByOperator(ctx context.Context, orgID int64, operatorID uint64) (*domainClinician.Clinician, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "clinician not found")
}

func (s *relationshipServiceClinicianRepo) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*domainClinician.Clinician, error) {
	return nil, nil
}

func (s *relationshipServiceClinicianRepo) Count(ctx context.Context, orgID int64) (int64, error) {
	return 0, nil
}

func (s *relationshipServiceClinicianRepo) Delete(ctx context.Context, id domainClinician.ID) error {
	return nil
}

type relationshipServiceTesteeRepo struct {
	item *domainTestee.Testee
}

func (s *relationshipServiceTesteeRepo) Save(ctx context.Context, testee *domainTestee.Testee) error {
	return nil
}

func (s *relationshipServiceTesteeRepo) Update(ctx context.Context, testee *domainTestee.Testee) error {
	return nil
}

func (s *relationshipServiceTesteeRepo) FindByID(ctx context.Context, id domainTestee.ID) (*domainTestee.Testee, error) {
	return s.item, nil
}

func (s *relationshipServiceTesteeRepo) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*domainTestee.Testee, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "testee not found")
}

func (s *relationshipServiceTesteeRepo) FindByOrgAndName(ctx context.Context, orgID int64, name string) ([]*domainTestee.Testee, error) {
	return nil, nil
}

func (s *relationshipServiceTesteeRepo) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*domainTestee.Testee, error) {
	return nil, nil
}

func (s *relationshipServiceTesteeRepo) ListByOrgAndIDs(ctx context.Context, orgID int64, ids []domainTestee.ID, filter domainTestee.ListFilter, offset, limit int) ([]*domainTestee.Testee, error) {
	return nil, nil
}

func (s *relationshipServiceTesteeRepo) ListByTags(ctx context.Context, orgID int64, tags []string, offset, limit int) ([]*domainTestee.Testee, error) {
	return nil, nil
}

func (s *relationshipServiceTesteeRepo) ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) ([]*domainTestee.Testee, error) {
	return nil, nil
}

func (s *relationshipServiceTesteeRepo) ListByProfileIDs(ctx context.Context, profileIDs []uint64, offset, limit int) ([]*domainTestee.Testee, error) {
	return nil, nil
}

func (s *relationshipServiceTesteeRepo) Delete(ctx context.Context, id domainTestee.ID) error {
	return nil
}

func (s *relationshipServiceTesteeRepo) Count(ctx context.Context, orgID int64) (int64, error) {
	return 0, nil
}

func (s *relationshipServiceTesteeRepo) CountByOrgAndIDs(ctx context.Context, orgID int64, ids []domainTestee.ID, filter domainTestee.ListFilter) (int64, error) {
	return 0, nil
}
