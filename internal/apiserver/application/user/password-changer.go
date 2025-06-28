package user

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

type PasswordChanger struct {
	userRepo port.UserRepository
}

func NewPasswordChanger(userRepo port.UserRepository) port.PasswordChanger {
	return &PasswordChanger{userRepo: userRepo}
}

// ChangePassword 修改密码
func (p *PasswordChanger) ChangePassword(ctx context.Context, req port.UserPasswordChangeRequest) error {
	user, err := p.userRepo.FindByID(ctx, user.NewUserID(req.ID))
	if err != nil {
		return err
	}

	user.ChangePassword(req.NewPassword)

	if err := p.userRepo.Update(ctx, user); err != nil {
		return err
	}

	return nil
}

// ValidatePassword 验证密码
func (p *PasswordChanger) ValidatePassword(ctx context.Context, username, password string) (*port.UserResponse, error) {
	user, err := p.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	if !user.ValidatePassword(password) {
		return nil, errors.WithCode(code.ErrPasswordIncorrect, "password is incorrect")
	}

	return nil, nil
}
