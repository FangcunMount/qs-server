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
func (e *UserEditor) UpdateBasicInfo(ctx context.Context, id uint64, username, nickname, email, phone, avatar, introduction string) (*user.User, error) {
	userObj, err := e.userRepo.FindByID(ctx, user.NewUserID(id))
	if err != nil {
		return nil, err
	}

	// 修改用户基本信息
	if username != "" {
		userObj.ChangeUsername(username)
	}
	if nickname != "" {
		userObj.ChangeNickname(nickname)
	}
	if email != "" {
		userObj.ChangeEmail(email)
	}
	if phone != "" {
		userObj.ChangePhone(phone)
	}
	if avatar != "" {
		userObj.ChangeAvatar(avatar)
	}
	if introduction != "" {
		userObj.ChangeIntroduction(introduction)
	}

	// 更新用户信息
	if err := e.userRepo.Update(ctx, userObj); err != nil {
		return nil, err
	}

	return userObj, nil
}

// UpdateAvatar 更新用户头像
func (e *UserEditor) UpdateAvatar(ctx context.Context, id uint64, avatar string) error {
	userObj, err := e.userRepo.FindByID(ctx, user.NewUserID(id))
	if err != nil {
		return err
	}

	// 修改用户头像
	userObj.ChangeAvatar(avatar)

	return e.userRepo.Update(ctx, userObj)
}
