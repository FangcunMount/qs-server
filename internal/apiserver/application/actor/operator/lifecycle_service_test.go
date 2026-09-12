package operator

import (
	"context"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"testing"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestCreateAndSaveOperator_PersistsContactInfoOnCreate(t *testing.T) {
	repo := newFakeOperatorRepo()
	service := newTestLifecycleService(repo)

	dto := RegisterOperatorDTO{
		OrgID:    1,
		Name:     "章依文",
		Email:    "zhangyiwen001@fangcunmount.com",
		Phone:    "+8617700000001",
		Roles:    []string{"qs:assessment_operator"},
		IsActive: true,
	}

	operator, created, err := service.createAndSaveOperator(context.Background(), dto, 10001)
	if err != nil {
		t.Fatalf("createAndSaveOperator returned error: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true")
	}
	if operator.Email() != dto.Email {
		t.Fatalf("expected email %q, got %q", dto.Email, operator.Email())
	}
	if operator.Phone() != dto.Phone {
		t.Fatalf("expected phone %q, got %q", dto.Phone, operator.Phone())
	}
	if len(operator.Roles()) != 0 {
		t.Fatalf("local operator roles = %v, want IAM snapshot projection only", operator.Roles())
	}
}

func TestCreateAndSaveOperator_ReusesExistingOperatorByUserID(t *testing.T) {
	repo := newFakeOperatorRepo()
	service := newTestLifecycleService(repo)

	existing := domain.NewOperator(1, 10001, "旧名字")
	existing.SetID(20001)
	if err := repo.Save(context.Background(), existing); err != nil {
		t.Fatalf("save existing operator: %v", err)
	}

	dto := RegisterOperatorDTO{
		OrgID:    1,
		Name:     "章依文",
		Email:    "zhangyiwen001@fangcunmount.com",
		Phone:    "+8617700000001",
		Roles:    []string{"qs:result_reviewer"},
		IsActive: true,
	}

	operator, created, err := service.createAndSaveOperator(context.Background(), dto, 10001)
	if err != nil {
		t.Fatalf("createAndSaveOperator returned error: %v", err)
	}
	if created {
		t.Fatalf("expected created=false when operator already exists")
	}
	if operator.ID() != existing.ID() {
		t.Fatalf("expected existing operator to be reused")
	}
	if operator.Name() != dto.Name {
		t.Fatalf("expected name %q, got %q", dto.Name, operator.Name())
	}
	if operator.Email() != dto.Email {
		t.Fatalf("expected email %q, got %q", dto.Email, operator.Email())
	}
	if operator.Phone() != dto.Phone {
		t.Fatalf("expected phone %q, got %q", dto.Phone, operator.Phone())
	}
	if len(operator.Roles()) != 0 {
		t.Fatalf("local operator roles = %v, want IAM snapshot projection only", operator.Roles())
	}
}

func TestValidateRegisterDTORequiresPasswordForNewIAMAccount(t *testing.T) {
	service := newTestLifecycleService(newFakeOperatorRepo())
	err := service.validateRegisterDTO(RegisterOperatorDTO{
		OrgID: 1,
		Name:  "章依文",
		Phone: "+8617700000001",
	})
	if err == nil || !errors.IsCode(err, code.ErrValidation) {
		t.Fatalf("validateRegisterDTO() error = %v, want validation error", err)
	}
}

func newTestLifecycleService(repo domain.Repository) *lifecycleService {
	validator := domain.NewValidator()
	return &lifecycleService{
		repo:       repo,
		validator:  validator,
		editor:     domain.NewEditor(validator),
		lifecycler: domain.NewLifecycler(),
	}
}

type fakeOperatorRepo struct {
	byUser  map[int64]*domain.Operator
	nextID  uint64
	updates int
}

func newFakeOperatorRepo() *fakeOperatorRepo {
	return &fakeOperatorRepo{
		byUser: make(map[int64]*domain.Operator),
		nextID: 1,
	}
}

func (r *fakeOperatorRepo) Save(_ context.Context, operator *domain.Operator) error {
	if _, exists := r.byUser[operator.UserID()]; exists {
		return errors.WithCode(code.ErrUserAlreadyExists, "operator already exists in this organization")
	}
	if operator.ID() == 0 {
		operator.SetID(domain.ID(r.nextID))
		r.nextID++
	}
	r.byUser[operator.UserID()] = operator
	return nil
}

func (r *fakeOperatorRepo) Update(_ context.Context, operator *domain.Operator) error {
	r.byUser[operator.UserID()] = operator
	r.updates++
	return nil
}

func (r *fakeOperatorRepo) FindByID(_ context.Context, id domain.ID) (*domain.Operator, error) {
	for _, operator := range r.byUser {
		if operator.ID() == id {
			return operator, nil
		}
	}
	return nil, errors.WithCode(code.ErrUserNotFound, "operator not found")
}

func (r *fakeOperatorRepo) FindByUser(_ context.Context, _ int64, userID int64) (*domain.Operator, error) {
	if operator, exists := r.byUser[userID]; exists {
		return operator, nil
	}
	return nil, errors.WithCode(code.ErrUserNotFound, "operator not found")
}

func (r *fakeOperatorRepo) ListByOrg(_ context.Context, _ int64, _, _ int) ([]*domain.Operator, error) {
	return nil, nil
}

func (r *fakeOperatorRepo) ListByRole(_ context.Context, _ int64, _ domain.Role, _, _ int) ([]*domain.Operator, error) {
	return nil, nil
}

func (r *fakeOperatorRepo) Delete(_ context.Context, id domain.ID) error {
	for userID, operator := range r.byUser {
		if operator.ID() == id {
			delete(r.byUser, userID)
			return nil
		}
	}
	return nil
}

func (r *fakeOperatorRepo) Count(_ context.Context, _ int64) (int64, error) {
	return int64(len(r.byUser)), nil
}

func TestRegisterRejectsRoleOnlyAssignmentBeforeIdentityOrLocalWrites(t *testing.T) {
	repo := newFakeOperatorRepo()
	service := newTestLifecycleService(repo)
	service.authz = &operatorAuthzGatewayFake{}
	_, err := service.Register(context.Background(), RegisterOperatorDTO{OrgID: 1, UserID: 10001, Name: "test", Roles: []string{"qs:assessment_operator"}, IsActive: true})
	if err == nil || !errors.IsCode(err, code.ErrValidation) {
		t.Fatalf("error=%v", err)
	}
	if len(repo.byUser) != 0 || repo.updates != 0 {
		t.Fatal("registration rejection wrote local records")
	}
}

func TestRegisterValidationAllowsIdentityWithoutRoles(t *testing.T) {
	service := newTestLifecycleService(newFakeOperatorRepo())
	if err := service.validateRegisterDTO(RegisterOperatorDTO{OrgID: 1, UserID: 10001, Name: "test", IsActive: true}); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterMembershipDoesNotReplaceExistingIAMAssignments(t *testing.T) {
	ctx := context.Background()
	repo := newFakeOperatorRepo()
	service := newTestLifecycleService(repo)
	gateway := &operatorAuthzGatewayFake{}
	service.authz = gateway
	service.uow = apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
	dto := RegisterOperatorDTO{OrgID: 1, UserID: 10001, Name: "test", IsActive: true}
	result, err := service.Register(ctx, dto)
	if err != nil {
		t.Fatal(err)
	}
	if gateway.replaceCalls != 0 {
		t.Fatal("registration changed IAM assignments")
	}
	if len(result.Roles) != 0 || !result.AuthzProjectionPending {
		t.Fatalf("new projection=%+v", result)
	}
	op, err := repo.FindByUser(ctx, 1, 10001)
	if err != nil {
		t.Fatal(err)
	}
	op.ReplaceRolesProjection([]domain.Role{domain.RoleResultReviewer}, []domain.Role{domain.RoleResultReviewer}, 42, nil, false)
	result, err = service.Register(ctx, dto)
	if err != nil {
		t.Fatal(err)
	}
	if gateway.replaceCalls != 0 || len(result.Roles) != 1 || result.Roles[0] != string(domain.RoleResultReviewer) || result.AuthzPolicyVersion != 42 {
		t.Fatalf("repeated registration replaced authorization: %+v", result)
	}
}
