package user

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
)

type PasswordChanger struct {
	userRepo port.UserRepository
}

func NewPasswordChanger(userRepo port.UserRepository) port.PasswordChanger {
	return &PasswordChanger{userRepo: userRepo}
}

// ChangePassword 修改密码
func (p *PasswordChanger) ChangePassword(ctx context.Context, id uint64, oldPassword, newPassword string) error {
	userObj, err := p.userRepo.FindByID(ctx, user.NewUserID(id))
	if err != nil {
		return err
	}

	// TODO: 验证旧密码
	// if !userObj.CheckPassword(oldPassword) {
	//     return errors.New("old password is incorrect")
	// }

	userObj.ChangePassword(newPassword)

	return p.userRepo.Update(ctx, userObj)
}
