package testee

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/log"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// registrationService 受试者注册服务实现
// 行为者：C端用户(患者/家长)
type registrationService struct {
	repo      domain.Repository
	factory   domain.Factory
	validator domain.Validator
	binder    domain.Binder
	uow       *mysql.UnitOfWork
}

// NewRegistrationService 创建受试者注册服务
func NewRegistrationService(
	repo domain.Repository,
	factory domain.Factory,
	validator domain.Validator,
	binder domain.Binder,
	uow *mysql.UnitOfWork,
) TesteeRegistrationService {
	return &registrationService{
		repo:      repo,
		factory:   factory,
		validator: validator,
		binder:    binder,
		uow:       uow,
	}
}

// Register 注册受试者
func (s *registrationService) Register(ctx context.Context, dto RegisterTesteeDTO) (*TesteeResult, error) {
	var result *domain.Testee

	log := log.WithContext(ctx)
	log.Infof("Starting testee registration: OrgID=%d, Name=%s, ProfileID=%v", dto.OrgID, dto.Name, dto.ProfileID)

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		log.Debugf("Inside transaction: validating parameters")
		// 1. 验证参数
		if err := s.validator.ValidateName(dto.Name, true); err != nil {
			log.Errorf("Name validation failed: %v", err)
			return err
		}
		gender := domain.Gender(dto.Gender)
		if err := s.validator.ValidateGender(gender); err != nil {
			log.Errorf("Gender validation failed: %v", err)
			return err
		}

		// 2. 如果提供了 ProfileID，检查是否已存在
		if dto.ProfileID != nil && *dto.ProfileID > 0 {
			_, err := s.repo.FindByProfile(txCtx, dto.OrgID, *dto.ProfileID)
			if err == nil {
				return errors.WithCode(code.ErrUserAlreadyExists, "testee with this profile already exists")
			}
			if !errors.IsCode(err, code.ErrUserNotFound) {
				return err
			}
		}

		// 3. 创建受试者
		result = domain.NewTestee(dto.OrgID, dto.Name, gender, dto.Birthday)

		// 4. 设置数据来源
		if dto.Source != "" {
			result.SetSource(dto.Source)
		}

		// 5. 如果提供了 ProfileID，绑定
		if dto.ProfileID != nil && *dto.ProfileID > 0 {
			if err := s.binder.Bind(txCtx, result, *dto.ProfileID); err != nil {
				return err
			}
		// 6. 持久化
		log.Debugf("Saving testee to repository: ID=%s, Name=%s", result.ID().String(), result.Name())
		if err := s.repo.Save(txCtx, result); err != nil {
			log.Errorf("Failed to save testee to repository: %v", err)
			return errors.Wrap(err, "failed to save testee")
		}
		log.Infof("Testee saved successfully: ID=%s", result.ID().String())

		return nil
	})

	if err != nil {
		log.Errorf("Transaction failed: %v", err)
		return nil, err
	}

	log.Infof("Testee registration completed successfully: ID=%s, Name=%s", result.ID().String(), result.Name())
	return toTesteeResult(result), nil
}

// EnsureByProfile 确保受试者存在（幂等操作）
func (s *registrationService) EnsureByProfile(ctx context.Context, dto EnsureTesteeDTO) (*TesteeResult, error) {
	var result *domain.Testee

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {

		// 如果没有 ProfileID，无法使用幂等创建
		if dto.ProfileID == nil || *dto.ProfileID == 0 {
			return errors.WithCode(code.ErrInvalidArgument, "profileID is required for ensure operation")
		}

		// 使用工厂的幂等创建方法
		var err error
		result, err = s.factory.GetOrCreateByProfile(txCtx, dto.OrgID, *dto.ProfileID, dto.Name, dto.Gender, dto.Birthday)
		return err
	})

	if err != nil {
		return nil, err
	}

	return toTesteeResult(result), nil
}

// GetMyProfile 获取我的受试者档案
func (s *registrationService) GetMyProfile(ctx context.Context, orgID int64, profileID uint64) (*TesteeResult, error) {
	testee, err := s.repo.FindByProfile(ctx, orgID, profileID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee profile not found")
		}
		return nil, errors.Wrap(err, "failed to find testee by profile")
	}

	return toTesteeResult(testee), nil
}
