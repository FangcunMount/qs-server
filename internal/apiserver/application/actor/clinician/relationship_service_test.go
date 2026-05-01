package clinician

import (
	"context"
	"testing"
	"time"

	cbErrors "github.com/FangcunMount/component-base/pkg/errors"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestAssignTesteeNormalizesAssignedToAttending(t *testing.T) {
	relationRepo := &relationshipServiceRelationRepo{}
	svc := &relationshipService{
		relationRepo:  relationRepo,
		clinicianRepo: &relationshipServiceClinicianRepo{item: makeActiveClinician(10)},
		testeeRepo:    &relationshipServiceTesteeRepo{item: makeTestee(20)},
		uow:           passthroughTxRunner{},
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
		uow:           passthroughTxRunner{},
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

func TestAssignPrimaryReplacesExistingAccessRelation(t *testing.T) {
	existingAccess := domainRelation.NewClinicianTesteeRelation(
		1,
		domainClinician.ID(11),
		domainTestee.ID(20),
		domainRelation.RelationTypeAttending,
		domainRelation.SourceTypeManual,
		nil,
		true,
		time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
		nil,
	)
	existingAccess.SetID(domainRelation.ID(9002))

	relationRepo := &relationshipServiceRelationRepo{
		activeAccessByTypes: existingAccess,
	}
	svc := &relationshipService{
		relationRepo:  relationRepo,
		clinicianRepo: &relationshipServiceClinicianRepo{item: makeActiveClinician(11)},
		testeeRepo:    &relationshipServiceTesteeRepo{item: makeTestee(20)},
		uow:           passthroughTxRunner{},
	}

	result, err := svc.AssignPrimary(context.Background(), AssignTesteeDTO{
		OrgID:       1,
		ClinicianID: 11,
		TesteeID:    20,
	})
	if err != nil {
		t.Fatalf("expected assign primary to succeed: %v", err)
	}
	if len(relationRepo.updatedItems) != 1 || relationRepo.updatedItems[0].IsActive() {
		t.Fatalf("expected existing access relation to be unbound")
	}
	if relationRepo.saved == nil || relationRepo.saved.RelationType() != domainRelation.RelationTypePrimary {
		t.Fatalf("expected new primary relation to be saved")
	}
	if result.RelationType != string(domainRelation.RelationTypePrimary) {
		t.Fatalf("expected primary result, got %s", result.RelationType)
	}
}

type passthroughTxRunner struct{}

func (passthroughTxRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestListAssignedTesteesUsesReadModel(t *testing.T) {
	relationReader := &relationshipServiceRelationReader{
		assignedRows: []actorreadmodel.TesteeRow{
			{ID: 21, OrgID: 1, Name: "testee-21"},
			{ID: 20, OrgID: 1, Name: "testee-20"},
		},
		assignedTotal: 2,
	}
	svc := &relationshipService{
		relationReader: relationReader,
	}

	result, err := svc.ListAssignedTestees(context.Background(), ListAssignedTesteeDTO{
		OrgID:       1,
		ClinicianID: 10,
		Offset:      0,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("expected list assigned testees to succeed: %v", err)
	}
	if relationReader.listAssignedCalls != 1 {
		t.Fatalf("expected read model to be called once, got %d", relationReader.listAssignedCalls)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 assigned testees, got %d", len(result.Items))
	}
	if result.Items[0].ID != 21 || result.Items[1].ID != 20 {
		t.Fatalf("expected relation order to be preserved, got %+v", result.Items)
	}
}

func TestListClinicianRelationsUsesReadModel(t *testing.T) {
	relationReader := &relationshipServiceRelationReader{
		clinicianRelations: []actorreadmodel.ClinicianRelationRow{
			{
				Relation: actorreadmodel.RelationRow{ID: 30, OrgID: 1, ClinicianID: 10, TesteeID: 30, RelationType: string(domainRelation.RelationTypeAttending), IsActive: true},
				Testee:   actorreadmodel.TesteeRow{ID: 30, OrgID: 1, Name: "testee-30"},
			},
			{
				Relation: actorreadmodel.RelationRow{ID: 31, OrgID: 1, ClinicianID: 10, TesteeID: 31, RelationType: string(domainRelation.RelationTypeAttending), IsActive: true},
				Testee:   actorreadmodel.TesteeRow{ID: 31, OrgID: 1, Name: "testee-31"},
			},
		},
		clinicianRelationsTotal: 2,
	}
	svc := &relationshipService{
		relationReader: relationReader,
	}

	result, err := svc.ListClinicianRelations(context.Background(), ListClinicianRelationDTO{
		OrgID:       1,
		ClinicianID: 10,
		ActiveOnly:  true,
		Offset:      0,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("expected list clinician relations to succeed: %v", err)
	}
	if relationReader.listClinicianRelationsCalls != 1 {
		t.Fatalf("expected read model to be called once, got %d", relationReader.listClinicianRelationsCalls)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 clinician relations, got %d", len(result.Items))
	}
	if result.Items[0].Testee.ID != 30 || result.Items[1].Testee.ID != 31 {
		t.Fatalf("expected relation order to be preserved, got %+v", result.Items)
	}
}

func makeActiveClinician(id uint64) *domainClinician.Clinician {
	item := domainClinician.NewClinician(1, nil, "clinician", "", "", domainClinician.TypeCounselor, "", true)
	item.SetID(domainClinician.ID(id))
	return item
}

func makeActiveRelation(clinicianID, testeeID uint64) *domainRelation.ClinicianTesteeRelation {
	item := domainRelation.NewClinicianTesteeRelation(
		1,
		domainClinician.ID(clinicianID),
		domainTestee.ID(testeeID),
		domainRelation.RelationTypeAttending,
		domainRelation.SourceTypeManual,
		nil,
		true,
		time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC),
		nil,
	)
	item.SetID(domainRelation.ID(testeeID))
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
	updatedItems          []*domainRelation.ClinicianTesteeRelation
	activePrimaryByTestee *domainRelation.ClinicianTesteeRelation
	activeAccessByTypes   *domainRelation.ClinicianTesteeRelation
	activeByClinician     []*domainRelation.ClinicianTesteeRelation
	historyByClinician    []*domainRelation.ClinicianTesteeRelation
}

func (s *relationshipServiceRelationRepo) Save(_ context.Context, item *domainRelation.ClinicianTesteeRelation) error {
	s.saved = item
	return nil
}

func (s *relationshipServiceRelationRepo) Update(_ context.Context, item *domainRelation.ClinicianTesteeRelation) error {
	s.updated = item
	s.updatedItems = append(s.updatedItems, item)
	return nil
}

func (s *relationshipServiceRelationRepo) FindByID(context.Context, domainRelation.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "relation not found")
}

func (s *relationshipServiceRelationRepo) FindActive(context.Context, int64, domainClinician.ID, domainTestee.ID, domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "relation not found")
}

func (s *relationshipServiceRelationRepo) FindActivePrimaryByTestee(context.Context, int64, domainTestee.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	if s.activePrimaryByTestee == nil {
		return nil, cbErrors.WithCode(code.ErrUserNotFound, "primary relation not found")
	}
	return s.activePrimaryByTestee, nil
}

func (s *relationshipServiceRelationRepo) FindActiveByTypes(context.Context, int64, domainClinician.ID, domainTestee.ID, []domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	if s.activeAccessByTypes != nil {
		return s.activeAccessByTypes, nil
	}
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "relation not found")
}

func (s *relationshipServiceRelationRepo) ListActiveByClinician(context.Context, int64, domainClinician.ID, []domainRelation.RelationType, int, int) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return s.activeByClinician, nil
}

func (s *relationshipServiceRelationRepo) ListHistoryByClinician(context.Context, int64, domainClinician.ID) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return s.historyByClinician, nil
}

func (s *relationshipServiceRelationRepo) CountActiveByClinician(context.Context, int64, domainClinician.ID, []domainRelation.RelationType) (int64, error) {
	return int64(len(s.activeByClinician)), nil
}

func (s *relationshipServiceRelationRepo) ListActiveByTestee(context.Context, int64, domainTestee.ID, []domainRelation.RelationType) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return nil, nil
}

func (s *relationshipServiceRelationRepo) ListHistoryByTestee(context.Context, int64, domainTestee.ID) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return nil, nil
}

func (s *relationshipServiceRelationRepo) HasActiveRelationForTestee(context.Context, int64, domainClinician.ID, domainTestee.ID, []domainRelation.RelationType) (bool, error) {
	return false, nil
}

func (s *relationshipServiceRelationRepo) ListActiveTesteeIDsByClinician(context.Context, int64, domainClinician.ID, []domainRelation.RelationType) ([]domainTestee.ID, error) {
	return nil, nil
}

type relationshipServiceRelationReader struct {
	assignedRows                 []actorreadmodel.TesteeRow
	assignedTotal                int64
	clinicianRelations           []actorreadmodel.ClinicianRelationRow
	clinicianRelationsTotal      int64
	testeeRelations              []actorreadmodel.TesteeRelationRow
	activeTesteeIDs              []uint64
	listAssignedCalls            int
	listClinicianRelationsCalls  int
	listTesteeRelationsCalls     int
	listActiveTesteeIDsCalls     int
	hasActiveRelationForTesteeFn func(context.Context, int64, uint64, uint64, []string) (bool, error)
}

func (s *relationshipServiceRelationReader) ListAssignedTestees(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.TesteeRow, int64, error) {
	s.listAssignedCalls++
	return s.assignedRows, s.assignedTotal, nil
}

func (s *relationshipServiceRelationReader) ListActiveTesteeIDsByClinician(context.Context, int64, uint64, []string) ([]uint64, error) {
	s.listActiveTesteeIDsCalls++
	return s.activeTesteeIDs, nil
}

func (s *relationshipServiceRelationReader) ListTesteeRelations(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.TesteeRelationRow, error) {
	s.listTesteeRelationsCalls++
	return s.testeeRelations, nil
}

func (s *relationshipServiceRelationReader) ListClinicianRelations(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.ClinicianRelationRow, int64, error) {
	s.listClinicianRelationsCalls++
	return s.clinicianRelations, s.clinicianRelationsTotal, nil
}

func (s *relationshipServiceRelationReader) HasActiveRelationForTestee(ctx context.Context, orgID int64, clinicianID, testeeID uint64, relationTypes []string) (bool, error) {
	if s.hasActiveRelationForTesteeFn != nil {
		return s.hasActiveRelationForTesteeFn(ctx, orgID, clinicianID, testeeID, relationTypes)
	}
	return false, nil
}

type relationshipServiceClinicianRepo struct {
	item *domainClinician.Clinician
}

func (s *relationshipServiceClinicianRepo) Save(context.Context, *domainClinician.Clinician) error {
	return nil
}

func (s *relationshipServiceClinicianRepo) Update(context.Context, *domainClinician.Clinician) error {
	return nil
}

func (s *relationshipServiceClinicianRepo) FindByID(context.Context, domainClinician.ID) (*domainClinician.Clinician, error) {
	return s.item, nil
}

func (s *relationshipServiceClinicianRepo) FindByOperator(context.Context, int64, uint64) (*domainClinician.Clinician, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "clinician not found")
}

func (s *relationshipServiceClinicianRepo) ListByOrg(context.Context, int64, int, int) ([]*domainClinician.Clinician, error) {
	return nil, nil
}

func (s *relationshipServiceClinicianRepo) Count(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *relationshipServiceClinicianRepo) Delete(context.Context, domainClinician.ID) error {
	return nil
}

type relationshipServiceTesteeRepo struct {
	item          *domainTestee.Testee
	findByIDCalls int
}

func (s *relationshipServiceTesteeRepo) Save(context.Context, *domainTestee.Testee) error {
	return nil
}

func (s *relationshipServiceTesteeRepo) Update(context.Context, *domainTestee.Testee) error {
	return nil
}

func (s *relationshipServiceTesteeRepo) FindByID(_ context.Context, id domainTestee.ID) (*domainTestee.Testee, error) {
	s.findByIDCalls++
	return s.item, nil
}

func (s *relationshipServiceTesteeRepo) FindByProfile(context.Context, int64, uint64) (*domainTestee.Testee, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "testee not found")
}

func (s *relationshipServiceTesteeRepo) Delete(context.Context, domainTestee.ID) error {
	return nil
}
