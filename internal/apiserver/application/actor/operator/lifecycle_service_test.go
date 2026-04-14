package operator

import (
	"context"
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
		Roles:    []string{"qs:staff"},
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
	if !operator.HasRole(domain.RoleOperator) {
		t.Fatalf("expected operator to have qs:staff role")
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
		Roles:    []string{"qs:evaluator"},
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
	if !operator.HasRole(domain.RoleEvaluatorQS) {
		t.Fatalf("expected operator to have qs:evaluator role")
	}
}

func newTestLifecycleService(repo domain.Repository) *lifecycleService {
	validator := domain.NewValidator()
	roleAllocator := domain.NewRoleAllocator(validator)
	return &lifecycleService{
		repo:          repo,
		validator:     validator,
		editor:        domain.NewEditor(validator),
		lifecycler:    domain.NewLifecycler(roleAllocator),
		roleAllocator: roleAllocator,
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

func (r *fakeOperatorRepo) Save(_ context.Context, staff *domain.Operator) error {
	if _, exists := r.byUser[staff.UserID()]; exists {
		return errors.WithCode(code.ErrUserAlreadyExists, "operator already exists in this organization")
	}
	if staff.ID() == 0 {
		staff.SetID(domain.ID(r.nextID))
		r.nextID++
	}
	r.byUser[staff.UserID()] = staff
	return nil
}

func (r *fakeOperatorRepo) Update(_ context.Context, staff *domain.Operator) error {
	r.byUser[staff.UserID()] = staff
	r.updates++
	return nil
}

func (r *fakeOperatorRepo) FindByID(_ context.Context, id domain.ID) (*domain.Operator, error) {
	for _, staff := range r.byUser {
		if staff.ID() == id {
			return staff, nil
		}
	}
	return nil, errors.WithCode(code.ErrUserNotFound, "operator not found")
}

func (r *fakeOperatorRepo) FindByUser(_ context.Context, _ int64, userID int64) (*domain.Operator, error) {
	if staff, exists := r.byUser[userID]; exists {
		return staff, nil
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
	for userID, staff := range r.byUser {
		if staff.ID() == id {
			delete(r.byUser, userID)
			return nil
		}
	}
	return nil
}

func (r *fakeOperatorRepo) Count(_ context.Context, _ int64) (int64, error) {
	return int64(len(r.byUser)), nil
}
