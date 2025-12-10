package testee

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
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
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("开始注册受试者",
		"action", "register",
		"resource", "testee",
		"org_id", dto.OrgID,
		"name", dto.Name,
		"has_profile", dto.ProfileID != nil && *dto.ProfileID > 0,
	)

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		l.Debugw("进入事务：验证参数",
			"name", dto.Name,
			"gender", dto.Gender,
		)

		// 1. 验证参数
		if err := s.validator.ValidateName(dto.Name, true); err != nil {
			l.Warnw("姓名验证失败",
				"name", dto.Name,
				"result", "failed",
				"error", err.Error(),
			)
			return err
		}
		gender := domain.Gender(dto.Gender)
		if err := s.validator.ValidateGender(gender); err != nil {
			l.Warnw("性别验证失败",
				"gender", dto.Gender,
				"result", "failed",
				"error", err.Error(),
			)
			return err
		}

		l.Debugw("参数验证通过",
			"name", dto.Name,
			"gender", gender.String(),
			"result", "success",
		)

		// 2. 如果提供了 ProfileID，检查是否已存在
		if dto.ProfileID != nil && *dto.ProfileID > 0 {
			l.Debugw("检查Profile是否已存在",
				"profile_id", *dto.ProfileID,
				"org_id", dto.OrgID,
			)

			_, err := s.repo.FindByProfile(txCtx, dto.OrgID, *dto.ProfileID)
			if err == nil {
				l.Warnw("受试者已存在",
					"profile_id", *dto.ProfileID,
					"org_id", dto.OrgID,
					"result", "failed",
				)
				return errors.WithCode(code.ErrUserAlreadyExists, "testee with this profile already exists")
			}
			if !errors.IsCode(err, code.ErrUserNotFound) {
				l.Errorw("查询Profile失败",
					"profile_id", *dto.ProfileID,
					"org_id", dto.OrgID,
					"error", err.Error(),
				)
				return err
			}
			l.Debugw("Profile未被使用，可以注册",
				"profile_id", *dto.ProfileID,
			)
		}

		// 3. 创建受试者
		result = domain.NewTestee(dto.OrgID, dto.Name, gender, dto.Birthday)
		l.Debugw("创建受试者实体",
			"testee_id", result.ID().String(),
			"name", result.Name(),
		)

		// 4. 设置数据来源
		if dto.Source != "" {
			result.SetSource(dto.Source)
			l.Debugw("设置数据来源",
				"source", dto.Source,
			)
		}

		// 5. 如果提供了 ProfileID，绑定
		if dto.ProfileID != nil && *dto.ProfileID > 0 {
			l.Debugw("绑定Profile",
				"profile_id", *dto.ProfileID,
				"testee_id", result.ID().String(),
			)
			if err := s.binder.Bind(txCtx, result, *dto.ProfileID); err != nil {
				l.Errorw("绑定Profile失败",
					"profile_id", *dto.ProfileID,
					"testee_id", result.ID().String(),
					"error", err.Error(),
				)
				return err
			}
		}

		// 6. 持久化
		l.Debugw("保存受试者",
			"testee_id", result.ID().String(),
			"name", result.Name(),
		)
		if err := s.repo.Save(txCtx, result); err != nil {
			l.Errorw("保存受试者失败",
				"testee_id", result.ID().String(),
				"result", "failed",
				"error", err.Error(),
			)
			return errors.Wrap(err, "failed to save testee")
		}
		l.Debugw("受试者保存成功",
			"testee_id", result.ID().String(),
			"result", "success",
		)

		return nil
	})

	if err != nil {
		duration := time.Since(startTime)
		l.Errorw("注册受试者失败",
			"action", "register",
			"resource", "testee",
			"result", "failed",
			"org_id", dto.OrgID,
			"name", dto.Name,
			"duration_ms", duration.Milliseconds(),
			"error", err.Error(),
		)
		return nil, err
	}

	duration := time.Since(startTime)
	l.Infow("注册受试者成功",
		"action", "register",
		"resource", "testee",
		"result", "success",
		"testee_id", result.ID().String(),
		"name", result.Name(),
		"org_id", dto.OrgID,
		"duration_ms", duration.Milliseconds(),
	)
	return toTesteeResult(result), nil
}

// EnsureByProfile 确保受试者存在（幂等操作）
func (s *registrationService) EnsureByProfile(ctx context.Context, dto EnsureTesteeDTO) (*TesteeResult, error) {
	var result *domain.Testee
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("确保受试者存在",
		"action", "ensure",
		"resource", "testee",
		"org_id", dto.OrgID,
		"profile_id", dto.ProfileID,
	)

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {

		// 如果没有 ProfileID，无法使用幂等创建
		if dto.ProfileID == nil || *dto.ProfileID == 0 {
			l.Warnw("缺少ProfileID",
				"action", "ensure",
				"result", "failed",
			)
			return errors.WithCode(code.ErrInvalidArgument, "profileID is required for ensure operation")
		}

		// 使用工厂的幂等创建方法
		var err error
		result, err = s.factory.GetOrCreateByProfile(txCtx, dto.OrgID, *dto.ProfileID, dto.Name, dto.Gender, dto.Birthday)
		if err != nil {
			l.Errorw("幂等创建失败",
				"profile_id", *dto.ProfileID,
				"org_id", dto.OrgID,
				"error", err.Error(),
			)
		}
		return err
	})

	if err != nil {
		duration := time.Since(startTime)
		l.Errorw("确保受试者失败",
			"action", "ensure",
			"resource", "testee",
			"result", "failed",
			"duration_ms", duration.Milliseconds(),
			"error", err.Error(),
		)
		return nil, err
	}

	duration := time.Since(startTime)
	l.Infow("确保受试者成功",
		"action", "ensure",
		"resource", "testee",
		"result", "success",
		"testee_id", result.ID().String(),
		"org_id", dto.OrgID,
		"duration_ms", duration.Milliseconds(),
	)
	return toTesteeResult(result), nil
}

// GetMyProfile 获取我的受试者档案
func (s *registrationService) GetMyProfile(ctx context.Context, orgID int64, profileID uint64) (*TesteeResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询受试者档案",
		"action", "get_profile",
		"resource", "testee",
		"org_id", orgID,
		"profile_id", profileID,
	)

	testee, err := s.repo.FindByProfile(ctx, orgID, profileID)
	if err != nil {
		duration := time.Since(startTime)
		if errors.IsCode(err, code.ErrUserNotFound) {
			l.Warnw("受试者档案不存在",
				"action", "get_profile",
				"result", "not_found",
				"org_id", orgID,
				"profile_id", profileID,
				"duration_ms", duration.Milliseconds(),
			)
			return nil, errors.WithCode(code.ErrUserNotFound, "testee profile not found")
		}
		l.Errorw("查询受试者档案失败",
			"action", "get_profile",
			"result", "failed",
			"org_id", orgID,
			"profile_id", profileID,
			"duration_ms", duration.Milliseconds(),
			"error", err.Error(),
		)
		return nil, errors.Wrap(err, "failed to find testee by profile")
	}

	duration := time.Since(startTime)
	l.Debugw("查询受试者档案成功",
		"action", "get_profile",
		"result", "success",
		"testee_id", testee.ID().String(),
		"org_id", orgID,
		"profile_id", profileID,
		"duration_ms", duration.Milliseconds(),
	)
	return toTesteeResult(testee), nil
}
