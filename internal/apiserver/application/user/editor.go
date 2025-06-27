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
	user.ChangeUsername(req.Username)
	user.ChangeNickname(req.Nickname)
	user.ChangeEmail(req.Email)
	user.ChangePhone(req.Phone)

	// 更新用户信息
	if err := e.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	return nil, nil
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
