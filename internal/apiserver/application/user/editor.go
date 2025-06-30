package user

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
)

type UserEditor struct {
	userRepo port.UserRepository
}

func NewUserEditor(userRepo port.UserRepository) port.UserEditor {
	return &UserEditor{userRepo: userRepo}
}

// UpdateBasicInfo 更新用户基本信息
func (e *UserEditor) UpdateBasicInfo(ctx context.Context, req port.UserBasicInfoRequest) (*port.UserResponse, error) {
	user, err := e.userRepo.FindByID(ctx, user.NewUserID(req.ID))
	if err != nil {
		return nil, err
	}

	// 修改用户基本信息
	if req.Username != "" {
		user.ChangeUsername(req.Username)
	}
	if req.Nickname != "" {
		user.ChangeNickname(req.Nickname)
	}
	if req.Email != "" {
		user.ChangeEmail(req.Email)
	}
	if req.Phone != "" {
		user.ChangePhone(req.Phone)
	}
	if req.Introduction != "" {
		user.ChangeIntroduction(req.Introduction)
	}

	// 更新用户信息
	if err := e.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	// 返回更新后的用户信息
	return &port.UserResponse{
		ID:           user.ID().Value(),
		Username:     user.Username(),
		Nickname:     user.Nickname(),
		Phone:        user.Phone(),
		Avatar:       user.Avatar(),
		Introduction: user.Introduction(),
		Email:        user.Email(),
		Status:       user.Status().String(),
		CreatedAt:    user.CreatedAt().Format("2006-01-02 15:04:05"),
		UpdatedAt:    user.UpdatedAt().Format("2006-01-02 15:04:05"),
	}, nil
}

// UpdateAvatar 更新用户头像
func (e *UserEditor) UpdateAvatar(ctx context.Context, req port.UserAvatarRequest) error {
	user, err := e.userRepo.FindByID(ctx, user.NewUserID(req.ID))
	if err != nil {
		return err
	}

	// 修改用户头像
	user.ChangeAvatar(req.Avatar)

	if err := e.userRepo.Update(ctx, user); err != nil {
		return err
	}

	return nil
}
