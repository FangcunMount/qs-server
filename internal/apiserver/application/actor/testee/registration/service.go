package registration

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee/shared"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// registrationService 受试者注册服务实现
type registrationService struct {
	repo      testee.Repository
	factory   testee.Factory
	validator testee.Validator
	binder    testee.Binder
	uow       *mysql.UnitOfWork
}

// NewRegistrationService 创建受试者注册服务
func NewRegistrationService(
	repo testee.Repository,
	factory testee.Factory,
	validator testee.Validator,
	binder testee.Binder,
	uow *mysql.UnitOfWork,
) shared.TesteeRegistrationApplicationService {
	return &registrationService{
		repo:      repo,
		factory:   factory,
		validator: validator,
		binder:    binder,
		uow:       uow,
	}
}

// Register 注册受试者
func (s *registrationService) Register(ctx context.Context, dto shared.RegisterTesteeDTO) (*shared.TesteeResult, error) {
	var result *testee.Testee
	var err error

	err = s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 验证参数
		if err := s.validator.ValidateForCreation(txCtx, dto.OrgID, dto.Name, testee.Gender(dto.Gender)); err != nil {
			return err
		}
		if err := s.validator.ValidateBirthday(dto.Birthday); err != nil {
			return err
		}

		// 2. 检查是否已存在（如果提供了 ProfileID）
		if dto.ProfileID != nil {
			_, err := s.repo.FindByProfile(txCtx, dto.OrgID, *dto.ProfileID)
			if err == nil {
				return errors.WithCode(code.ErrUserAlreadyExists, "testee with this profile_id already exists")
			}
			if !errors.IsCode(err, code.ErrUserNotFound) {
				return err
			}
		}

		// 3. 创建受试者
		result = testee.NewTestee(dto.OrgID, dto.Name, testee.Gender(dto.Gender), dto.Birthday)

		// 4. 绑定 ProfileID（如果提供）
		if dto.ProfileID != nil {
			if err := s.binder.Bind(txCtx, result, *dto.ProfileID); err != nil {
				return err
			}
		}

		// 5. 设置来源
		if dto.Source != "" {
			result.SetSource(dto.Source)
		} else {
			result.SetSource("online_form")
		}

		// 6. 持久化
		if err := s.repo.Save(txCtx, result); err != nil {
			return errors.Wrap(err, "failed to save testee")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return toTesteeResult(result), nil
}

// EnsureByProfile 确保受试者存在（幂等）
func (s *registrationService) EnsureByProfile(ctx context.Context, dto shared.EnsureTesteeDTO) (*shared.TesteeResult, error) {
	if dto.ProfileID == nil {
		return nil, errors.WithCode(code.ErrValidation, "profile_id is required")
	}

	var result *testee.Testee
	var err error

	err = s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)
		// 使用工厂的幂等创建方法
		result, err = s.factory.GetOrCreateByProfile(
			txCtx,
			dto.OrgID,
			*dto.ProfileID,
			dto.Name,
			int8(dto.Gender),
			dto.Birthday,
		)
		return err
	})

	if err != nil {
		return nil, err
	}

	return toTesteeResult(result), nil
}

// toTesteeResult 将领域对象转换为 DTO
func toTesteeResult(t *testee.Testee) *shared.TesteeResult {
	if t == nil {
		return nil
	}

	result := &shared.TesteeResult{
		ID:        uint64(t.ID()),
		OrgID:     t.OrgID(),
		ProfileID: t.ProfileID(),
		Name:      t.Name(),
		Gender:    int8(t.Gender()),
		Birthday:  t.Birthday(),
		Source:    t.Source(),
	}

	// 计算年龄
	if t.Birthday() != nil {
		result.Age = calculateAge(*t.Birthday())
	}

	return result
}

// calculateAge 计算年龄
func calculateAge(birthday time.Time) int {
	now := time.Now()
	age := now.Year() - birthday.Year()

	// 如果今年的生日还没到，年龄减1
	if now.Month() < birthday.Month() || (now.Month() == birthday.Month() && now.Day() < birthday.Day()) {
		age--
	}

	return age
}
